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
	"sort"
	"strings"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// The guardrail verdict, kept as Dorgu's own.
//
// The verdict on spec.steps[].safety is structured data whose every string
// Dorgu wrote from its own arithmetic against the workload it observed. This
// file does the two things a client has to do with it, and nothing else:
//
//  1. take the messages back out of the step description the operator spliced
//     them into, so the verdict is stated once rather than twice
//  2. rank the verdicts, so a list can point at the plan worth opening
//
// It does not reword a verdict, summarise one, or decide what one means. Those
// are the operator's, and the reason the field exists at all is that a verdict
// presented in somebody else's words stopped being a measurement.

// verdictRank orders verdicts by how much a reader needs to know about them: a
// field Dorgu refused outright, then one it substituted a value for, then one it
// sized itself because the plan carried nothing appliable.
var verdictRank = []string{
	dorguv1.SafetyVerdictRejected,
	dorguv1.SafetyVerdictClamped,
	dorguv1.SafetyVerdictDerived,
}

// strongestVerdict returns the most consequential verdict anywhere in the plan,
// or "" when no guardrail ruled.
//
// The recognised verdicts are not treated as a closed set. An operator newer
// than this build may add one, and an unranked verdict is still Dorgu's, so it
// is reported rather than dropped: the first in sorted order, so repeated
// renders agree.
func strongestVerdict(steps []Step) string {
	seen := map[string]bool{}
	for _, step := range steps {
		for _, entry := range step.Safety {
			if verdict := strings.TrimSpace(entry.Verdict); verdict != "" {
				seen[verdict] = true
			}
		}
	}
	if len(seen) == 0 {
		return ""
	}

	for _, verdict := range verdictRank {
		if seen[verdict] {
			return verdict
		}
	}

	unranked := make([]string, 0, len(seen))
	for verdict := range seen {
		unranked = append(unranked, verdict)
	}
	sort.Strings(unranked)
	return unranked[0]
}

// countGuardrails is how many fields a guardrail ruled on across the plan.
func countGuardrails(steps []Step) int {
	total := 0
	for _, step := range steps {
		total += len(step.Safety)
	}
	return total
}

// descriptionWithoutSafetyMessages returns a step's description with the
// guardrail messages taken back out of it.
//
// The operator writes a guarded step's description as the step's own sentence
// followed by every safety message verbatim. A client that renders the
// description and then the verdicts prints the same forty words twice, three
// lines apart, which is what the structured field was introduced to stop.
//
// A description that turns out to be nothing but the message is left alone.
// Printing it twice is worse than printing it once; printing an empty step is
// worse than both.
func descriptionWithoutSafetyMessages(description string, entries []dorguv1.StepSafety) string {
	if len(entries) == 0 {
		return description
	}

	stripped := description
	for _, entry := range entries {
		if message := strings.TrimSpace(entry.Message); message != "" {
			stripped = strings.ReplaceAll(stripped, message, " ")
		}
	}

	stripped = strings.Join(strings.Fields(stripped), " ")
	if stripped == "" {
		return description
	}
	return stripped
}

// nonRedundantExplanation returns the explanation to render, or "" when the plan
// summary already says the same thing.
//
// Older operators wrote the root cause into planSummary and the same sentence
// with a prefix into explanation. The operator no longer does, but objects
// written by older ones are still in clusters, so the renderer copes with them
// rather than assuming the fix reached every object.
func nonRedundantExplanation(explanation, planSummary string) string {
	trimmed := strings.TrimSpace(explanation)
	summary := strings.TrimSpace(planSummary)
	if trimmed == "" || summary == "" {
		return trimmed
	}

	normalisedSummary := normaliseProse(summary)
	normalisedBody := normaliseProse(trimmed)
	if strings.Contains(normalisedBody, normalisedSummary) ||
		strings.Contains(normalisedSummary, normalisedBody) {
		return ""
	}
	return trimmed
}

// normaliseProse collapses case and whitespace so two renderings of the same
// sentence compare equal.
func normaliseProse(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}
