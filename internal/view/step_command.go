/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package view

import (
	"strings"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// Whether a step's suggested command may be offered for copying.
//
// The command is model-authored and this payload feeds a copy button, so it is
// untrusted input on its way to somebody's shell. Two independent filters apply:
//
//  1. shape, via the operator's own SanitizeStepCommand: one line, a kubectl
//     invocation, no shell metacharacters, within the CRD's length bound. It is
//     imported rather than re-implemented so the two cannot drift.
//  2. ownership: on a workload something else owns, only a command that reads is
//     offered. A write there takes field ownership from Helm or ArgoCD and
//     breaks their next apply, which is the failure the whole ownership model
//     exists to prevent.
//
// The operator already strips writing commands from an owned plan. This does it
// again because the dashboard reads RemediationActions straight out of the
// cluster: an older operator, a hand-written object, or anything with permission
// to create the CRD can put a `kubectl patch` in there. A guard the user's shell
// depends on belongs on the side that does the printing.
//
// Nothing here ever executes a command.

// mutatingKubectlVerbs, readOnlyKubectlVerbs and readOnlyRolloutSubcommands
// mirror the CLI's, which mirror the operator's ownership shaping. They are
// duplicated because neither module imports the other's internals; keep the
// three in step.
//
// Read-only verbs are listed explicitly rather than inferred from the absence of
// a mutating one, so a command whose verb matches nothing at all is refused
// rather than assumed harmless.
var (
	mutatingKubectlVerbs = map[string]bool{
		"annotate":  true,
		"apply":     true,
		"autoscale": true,
		"cordon":    true,
		"create":    true,
		"delete":    true,
		"drain":     true,
		"edit":      true,
		"expose":    true,
		"label":     true,
		"patch":     true,
		"replace":   true,
		"run":       true,
		"scale":     true,
		"set":       true,
		"taint":     true,
		"uncordon":  true,
	}

	readOnlyKubectlVerbs = map[string]bool{
		"api-resources": true,
		"api-versions":  true,
		"auth":          true,
		"cluster-info":  true,
		"describe":      true,
		"diff":          true,
		"events":        true,
		"explain":       true,
		"get":           true,
		"logs":          true,
		"top":           true,
		"version":       true,
	}

	// readOnlyRolloutSubcommands are the `kubectl rollout` forms that only read.
	// undo, restart, pause and resume all write.
	readOnlyRolloutSubcommands = map[string]bool{
		"history": true,
		"status":  true,
	}
)

// Reasons a command the object carried is not offered. They are rendered, so a
// reader who saw the command in `kubectl get -o yaml` learns why this screen is
// not repeating it rather than assuming the screen lost it.
const (
	commandWithheldUnsafeShape = "This step carried a command that is not a plain single-line kubectl " +
		"invocation, so Dorgu will not offer it for copying."
	commandWithheldOwnedWorkload = "This step carried a command that writes to a workload Dorgu does not " +
		"own. Applying it would take the fields it sets away from the owner and break their next deploy, " +
		"so it is not offered. The owner instructions below are the change to make instead."
)

// runnableStepCommand returns the command to offer, and the reason when there is
// none to offer but the step carried one.
func runnableStepCommand(command string, owned bool) (offered, withheld string) {
	if strings.TrimSpace(command) == "" {
		return "", ""
	}

	safe := dorguv1.SanitizeStepCommand(command)
	if safe == "" {
		return "", commandWithheldUnsafeShape
	}
	if owned && !readsOnly(safe) {
		return "", commandWithheldOwnedWorkload
	}
	return safe, ""
}

// Global kubectl flags, split by whether they consume the token after them.
//
// This table is the whole of the fix for a real bypass. The verb used to be
// found by scanning for the first token that matched a known verb name, which
// meant a flag VALUE that happened to be a verb name decided the answer:
//
//	kubectl -n logs delete deployment/foo
//
// classified as read-only, because "logs" was reached before "delete". A
// namespace called logs, get, top or describe is not exotic, and the
// consequence was a delete offered as a safe copy-paste command on a workload
// Dorgu does not own, which is the exact failure the ownership filter exists to
// prevent.
//
// So the verb is now found positionally: flags are skipped, a value-taking flag
// takes its value with it, and the first token that is left is the verb. A flag
// this table does not know is not guessed at, because guessing is what the
// scan was doing.
var (
	// valueTakingGlobalFlags consume the next token.
	valueTakingGlobalFlags = map[string]bool{
		"-n": true, "--namespace": true,
		"-o": true, "--output": true,
		"-s": true, "--server": true,
		"--context":               true,
		"--cluster":               true,
		"--user":                  true,
		"--kubeconfig":            true,
		"--token":                 true,
		"--as":                    true,
		"--as-group":              true,
		"--as-uid":                true,
		"--cache-dir":             true,
		"--certificate-authority": true,
		"--client-certificate":    true,
		"--client-key":            true,
		"--request-timeout":       true,
		"--tls-server-name":       true,
		"--username":              true,
		"--password":              true,
		"--profile":               true,
		"--profile-output":        true,
		"--log-flush-frequency":   true,
		"-v":                      true,
		"--v":                     true,
		"--vmodule":               true,
	}

	// booleanGlobalFlags stand alone.
	booleanGlobalFlags = map[string]bool{
		"--insecure-skip-tls-verify": true,
		"--match-server-version":     true,
		"--warnings-as-errors":       true,
		"--disable-compression":      true,
	}
)

// readsOnly reports whether a kubectl command only reads cluster state.
//
// Anything not positively recognised as read-only is refused: an unknown verb,
// an unknown flag, a flag whose value is missing, or a command that names no
// verb at all. That direction is deliberate. Refusing a safe command costs the
// reader a copy button; offering an unsafe one costs them a Deployment.
func readsOnly(command string) bool {
	verb, arguments, found := kubectlVerb(command)
	if !found {
		return false
	}
	if verb == "rollout" {
		return rolloutReadsOnly(arguments)
	}
	if mutatingKubectlVerbs[verb] {
		return false
	}
	return readOnlyKubectlVerbs[verb]
}

// kubectlVerb returns the subcommand a kubectl invocation names, and the tokens
// after it.
//
// It reports false rather than guessing whenever the command cannot be parsed
// with certainty: not a kubectl invocation, a flag this build does not know, a
// value-taking flag with nothing after it, or no verb at all.
func kubectlVerb(command string) (verb string, arguments []string, found bool) {
	fields := strings.Fields(command)
	if len(fields) < 2 || fields[0] != "kubectl" {
		return "", nil, false
	}

	for i := 1; i < len(fields); i++ {
		token := fields[i]

		if !strings.HasPrefix(token, "-") {
			// The first token that is not a flag is the verb.
			return token, fields[i+1:], true
		}

		// --flag=value carries its value, so nothing extra is skipped.
		if name, _, hasValue := strings.Cut(token, "="); hasValue {
			if !valueTakingGlobalFlags[name] && !booleanGlobalFlags[name] {
				return "", nil, false
			}
			continue
		}

		switch {
		case booleanGlobalFlags[token]:
			continue
		case valueTakingGlobalFlags[token]:
			// Its value is the next token and is not a verb. A flag with
			// nothing after it is a command this build cannot read.
			if i+1 >= len(fields) {
				return "", nil, false
			}
			i++
		default:
			// An unrecognised flag. Skipping it might skip a verb; consuming a
			// token after it might consume one. Neither guess is safe.
			return "", nil, false
		}
	}

	return "", nil, false
}

// rolloutReadsOnly classifies a `kubectl rollout` invocation from its remaining
// arguments. A bare `kubectl rollout` names no subcommand, so it is refused
// along with everything else that cannot be classified.
func rolloutReadsOnly(arguments []string) bool {
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "-") {
			continue
		}
		return readOnlyRolloutSubcommands[argument]
	}
	return false
}
