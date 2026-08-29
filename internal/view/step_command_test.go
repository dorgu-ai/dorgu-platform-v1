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
	"testing"

	"github.com/stretchr/testify/assert"
)

// This file exists because a bypass in here shipped once.
//
// The verb used to be found by scanning for the first token that matched a
// known verb name, so a flag value that happened to be a verb name decided the
// answer: `kubectl -n logs delete deployment/foo` classified as read-only,
// because "logs" was reached before "delete". A `delete` was then offered as a
// safe copy-paste command on a workload Dorgu does not own. Every case below is
// pinned so that cannot come back.

func TestReadsOnlyClassifiesTheVerbPositionally(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		// The bypass, in the forms that reached it.
		{"a namespace named after a read-only verb cannot hide a delete",
			"kubectl -n logs delete deployment/foo", false},
		{"a namespace named get cannot hide a patch",
			"kubectl --namespace get patch deploy/checkout", false},
		{"an output value named describe cannot hide a delete",
			"kubectl -o describe delete ns/prod", false},
		{"a context named top cannot hide a drain",
			"kubectl --context top drain node-a", false},

		// The same shapes, honestly read-only.
		{"a namespace named after a verb still allows a real read",
			"kubectl -n logs logs deploy/checkout", true},
		{"a global flag before a read verb", "kubectl -n apps get pods", true},
		{"a global flag before a write verb", "kubectl -n apps patch deploy/x", false},

		// Ordinary forms.
		{"verb first, read", "kubectl get pods -n apps", true},
		{"verb first, write", "kubectl delete pod/x -n apps", false},
		{"logs", "kubectl logs deploy/checkout -n apps", true},
		{"events", "kubectl events -n apps --for pod/x", true},
		{"top", "kubectl top nodes", true},
		{"describe", "kubectl describe deploy/checkout -n apps", true},

		// Flags that carry their own value.
		{"equals form on a value-taking flag", "kubectl --namespace=logs delete deploy/x", false},
		{"equals form before a read verb", "kubectl --namespace=apps get pods", true},
		{"boolean global flag", "kubectl --insecure-skip-tls-verify get pods", true},
		{"boolean global flag before a write", "kubectl --insecure-skip-tls-verify delete pod/x", false},

		// Refused because they cannot be read with certainty.
		{"an unrecognised flag is refused rather than guessed at",
			"kubectl --made-up-flag get pods", false},
		{"an unrecognised equals flag is refused",
			"kubectl --made-up-flag=x get pods", false},
		{"a trailing flag after the verb does not change the verb", "kubectl get pods -n", true},
		{"a value-taking flag with no value and no verb is refused", "kubectl -n", false},
		{"an unknown verb is refused", "kubectl frobnicate deploy/x", false},
		{"no verb at all", "kubectl --insecure-skip-tls-verify", false},
		{"not kubectl", "helm upgrade checkout ./chart", false},
		{"bare kubectl", "kubectl", false},
		{"empty", "", false},

		// rollout splits on its subcommand, and a bare rollout names none.
		{"rollout status reads", "kubectl rollout status deploy/checkout -n apps", true},
		{"rollout history reads", "kubectl rollout history deploy/checkout", true},
		{"rollout restart writes", "kubectl rollout restart deploy/checkout", false},
		{"rollout undo writes", "kubectl rollout undo deploy/checkout", false},
		{"rollout pause writes", "kubectl rollout pause deploy/checkout", false},
		{"a bare rollout cannot be classified", "kubectl rollout", false},
		{"rollout with only flags cannot be classified", "kubectl rollout --revision=2", false},
		{"a global flag before rollout status", "kubectl -n apps rollout status deploy/x", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, readsOnly(tt.command))
		})
	}
}

// The two filters are independent, and each has to hold on its own. Shape is
// checked because the command may have been authored by a model; ownership is
// checked because a write to a workload somebody else owns breaks their next
// deploy.
func TestRunnableStepCommandAppliesBothFilters(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		owned        bool
		wantOffered  string
		wantWithheld string
	}{
		{
			name:        "a write is offered on a workload Dorgu owns",
			command:     "kubectl scale deploy/reports --replicas=3",
			owned:       false,
			wantOffered: "kubectl scale deploy/reports --replicas=3",
		},
		{
			name:         "a write is withheld on a workload somebody else owns",
			command:      "kubectl scale deploy/checkout --replicas=3",
			owned:        true,
			wantWithheld: commandWithheldOwnedWorkload,
		},
		{
			name:        "a read is offered on a workload somebody else owns",
			command:     "kubectl logs deploy/checkout -n apps",
			owned:       true,
			wantOffered: "kubectl logs deploy/checkout -n apps",
		},
		{
			name:         "the bypass shape is withheld on an owned workload",
			command:      "kubectl -n logs delete deployment/checkout",
			owned:        true,
			wantWithheld: commandWithheldOwnedWorkload,
		},
		{
			// Shape is checked first and does not depend on ownership: a shell
			// metacharacter is refused even where Dorgu could write.
			name:         "a shell metacharacter is refused regardless of ownership",
			command:      "kubectl get pods; rm -rf /",
			owned:        false,
			wantWithheld: commandWithheldUnsafeShape,
		},
		{
			name:         "a non-kubectl command is refused regardless of ownership",
			command:      "helm upgrade checkout ./chart",
			owned:        false,
			wantWithheld: commandWithheldUnsafeShape,
		},
		{
			name:    "no command means nothing to offer and nothing to explain",
			command: "   ",
			owned:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offered, withheld := runnableStepCommand(tt.command, tt.owned)
			assert.Equal(t, tt.wantOffered, offered)
			assert.Equal(t, tt.wantWithheld, withheld)
		})
	}
}

// A command that is dropped has to say why. A reader who saw it in
// `kubectl get -o yaml` would otherwise think this screen lost it.
func TestEveryWithheldCommandCarriesAReason(t *testing.T) {
	for _, command := range []string{
		"kubectl delete deploy/checkout",
		"kubectl -n logs delete deploy/checkout",
		"kubectl get pods && rm -rf /",
		"not-kubectl at all",
		"kubectl --made-up-flag get pods",
	} {
		offered, withheld := runnableStepCommand(command, true)
		assert.Empty(t, offered, command)
		assert.NotEmpty(t, withheld, "%q was dropped without saying why", command)
	}
}
