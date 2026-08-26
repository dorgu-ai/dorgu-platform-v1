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

// Package workload resolves an ApplicationPersona to the Deployment it
// describes, and reports which system owns that Deployment's desired state.
//
// # Why this is a copy
//
// Both algorithms are ports of dorgu-operator's internal/workload. The CRD
// types in dorgu-operator/api/v1 are importable and are imported; the match
// chain and the ownership detection are not, because Go forbids importing
// another module's internal packages. The dashboard has to agree with the
// operator on both questions or it will show a different answer than
// `dorgu health` for the same cluster, so the logic is ported verbatim and
// pinned by tests that mirror the operator's own.
//
// The right fix is for the operator to promote internal/workload to
// pkg/workload and for this package to become a thin re-export. Until then,
// treat divergence between the two as a bug in this file.
package workload

import (
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// LabelAppName is the Kubernetes recommended app label.
	LabelAppName = "app.kubernetes.io/name"
	// LabelApp is the common short-form app label.
	LabelApp = "app"
)

// Rung names, reported to the UI so a reader can see how a persona was tied to
// a Deployment rather than having to trust that it was.
const (
	RungLabelAppName = "label " + LabelAppName
	RungLabelApp     = "label " + LabelApp
	RungName         = "metadata.name"
	RungSelector     = "spec.selector.matchLabels"
)

// matcher is one rung of the fallback chain: a human-readable name and the
// predicate that decides whether a Deployment belongs to the named persona.
type matcher struct {
	rung  string
	match func(*appsv1.Deployment, string) bool
}

// chain is the ordered fallback chain. Earlier rungs are more explicit
// statements of intent, so they win over later ones.
var chain = []matcher{
	{RungLabelAppName, func(d *appsv1.Deployment, name string) bool {
		return d.Labels[LabelAppName] == name
	}},
	{RungLabelApp, func(d *appsv1.Deployment, name string) bool {
		return d.Labels[LabelApp] == name
	}},
	{RungName, func(d *appsv1.Deployment, name string) bool {
		return d.Name == name
	}},
	{RungSelector, func(d *appsv1.Deployment, name string) bool {
		if d.Spec.Selector == nil {
			return false
		}
		ml := d.Spec.Selector.MatchLabels
		return ml[LabelAppName] == name || ml[LabelApp] == name
	}},
}

// AmbiguousError reports that a single rung matched more than one Deployment.
// The dashboard surfaces this as its own state rather than picking a candidate:
// showing one arbitrary Deployment's resources under a persona would be a
// confident wrong answer, which is the failure mode this product exists to
// avoid.
type AmbiguousError struct {
	// PersonaName is the persona spec.name that was being resolved.
	PersonaName string
	// Rung is the chain rung that produced the tie.
	Rung string
	// Candidates holds the matching Deployment names, sorted.
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%d Deployments match persona %q by %s (%s); set %s=%s on exactly one",
		len(e.Candidates), e.PersonaName, e.Rung, strings.Join(e.Candidates, ", "),
		LabelAppName, e.PersonaName)
}

// ChainDescription lists the rungs that were tried, for "we found nothing"
// messages. Naming the rungs is the difference between an actionable error and
// a dead end.
func ChainDescription() string {
	names := make([]string, 0, len(chain))
	for _, m := range chain {
		names = append(names, m.rung)
	}
	return strings.Join(names, ", ")
}

// Matches reports whether a Deployment belongs to the named persona by any rung
// of the chain. Used for the "is this workload monitored?" question, where
// ambiguity does not matter because either answer means monitored.
func Matches(deploy *appsv1.Deployment, personaName string) bool {
	if deploy == nil || personaName == "" {
		return false
	}
	for _, m := range chain {
		if m.match(deploy, personaName) {
			return true
		}
	}
	return false
}

// Resolve picks the Deployment described by personaName from the candidate set,
// walking the fallback chain in order. It returns the match and the rung that
// found it. No match at any rung returns (nil, "", nil): the caller decides how
// to report an unfound workload. A rung matching several Deployments returns an
// *AmbiguousError.
func Resolve(deployments []*appsv1.Deployment, personaName string) (*appsv1.Deployment, string, error) {
	if personaName == "" {
		return nil, "", nil
	}

	for _, m := range chain {
		var matched []*appsv1.Deployment
		for _, d := range deployments {
			if d != nil && m.match(d, personaName) {
				matched = append(matched, d)
			}
		}

		switch len(matched) {
		case 0:
			continue
		case 1:
			return matched[0], m.rung, nil
		default:
			candidates := make([]string, 0, len(matched))
			for _, d := range matched {
				candidates = append(candidates, d.Name)
			}
			sort.Strings(candidates)
			return nil, m.rung, &AmbiguousError{
				PersonaName: personaName,
				Rung:        m.rung,
				Candidates:  candidates,
			}
		}
	}

	return nil, "", nil
}

// PickContainer chooses the container a persona describes.
//
// A persona names an application, not a container, and the two rarely match in
// brownfield clusters (persona "frontend" over Deployment "frontend-podinfo"
// whose container is "podinfo"). The order is: exact name match, then the first
// container. Callers record the chosen name so a reader can see what was
// actually inspected.
func PickContainer(deploy *appsv1.Deployment, personaName string) *corev1.Container {
	if deploy == nil {
		return nil
	}
	containers := deploy.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		return nil
	}
	for i := range containers {
		if containers[i].Name == personaName {
			return &containers[i]
		}
	}
	return &containers[0]
}
