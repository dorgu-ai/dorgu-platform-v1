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
	"regexp"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// Annotations and conditions the operator writes on an IncidentMemory since
// v0.13.0. The operator keeps most of these constants in its controller
// package, which is not importable, so the strings are restated here and the
// tests lock them to the documented values.
const (
	annotationRecurrenceOf      = "dorgu.io/recurrence-of"
	annotationRecurrenceCount   = "dorgu.io/recurrence-count"
	annotationEscalatedFrom     = "dorgu.io/escalated-from"
	annotationEscalatedAt       = "dorgu.io/escalated-at"
	annotationContainers        = "dorgu.io/containers"
	annotationLastResolution    = "dorgu.io/last-resolution"
	annotationRecurrencesFolded = "dorgu.io/recurrences-folded"

	conditionRecurring = "Recurring"
	conditionFlapping  = "Flapping"
)

// Proposal states an incident can be in, read from its ProposalRefused
// condition.
const (
	// ProposalRefused is a diagnosis the gate refused to turn into a plan.
	// Nothing was proposed, and the reason says which guardrail refused.
	ProposalRefused = "refused"
	// ProposalDeferred is a cycle in which the gate stood down because a
	// proposal for this incident is outstanding or cooling down after a
	// decision. It is not a refusal and says nothing against the diagnosis.
	ProposalDeferred = "deferred"
	// ProposalMade is a real RemediationAction having been proposed.
	ProposalMade = "proposed"
	// ProposalCleared is a refusal that no longer applies because the incident
	// resolved.
	ProposalCleared = "cleared"
)

// faultFamily is the progression of pod signals the operator treats as one
// fault, mildest first. A worse signal escalates the open incident instead of
// opening another one. It mirrors the operator's own ranking (B9); a signal
// outside it is a different kind of incident and has no family.
var faultFamily = []string{
	"ReadinessNotReady",
	"RestartLoop",
	"LivenessRestarts",
	"CrashLoopBackOff",
	"OOMKilled",
}

var generationSuffix = regexp.MustCompile(`-r([0-9]+)$`)

// IncidentProposal is what the gate did with this incident's diagnosis.
type IncidentProposal struct {
	// State is ProposalRefused, ProposalDeferred, ProposalMade or
	// ProposalCleared.
	State string `json:"state"`
	// Reason is the condition's CamelCase enum. For a refusal it is
	// "<Stage>_<Reason>", split below so a client never parses it.
	Reason string `json:"reason"`
	// Stage and Guardrail are the parts of a refusal's reason, e.g.
	// ModePolicy and SelfHealingMode. Empty unless the state is refused.
	Stage     string      `json:"stage,omitempty"`
	Guardrail string      `json:"guardrail,omitempty"`
	Message   string      `json:"message,omitempty"`
	Since     metav1.Time `json:"since"`
}

// IncidentRefusalRecord is the durable record that the gate refused a proposal
// for this incident. ProposalRefused is the live state and is rewritten when the
// incident resolves; this one is never cleared, so a refusal that happened can
// still be counted afterwards.
type IncidentRefusalRecord struct {
	Reason    string      `json:"reason"`
	Stage     string      `json:"stage,omitempty"`
	Guardrail string      `json:"guardrail,omitempty"`
	Message   string      `json:"message,omitempty"`
	At        metav1.Time `json:"at"`
}

// IncidentLineage places an incident in the history of its fault.
//
// A fault that comes back after its incident resolved is a new IncidentMemory
// named "<first-name>-r<N>". Every record of one fault shares a lineage, which
// is the name without the suffix.
type IncidentLineage struct {
	// Key is the incident name without its -rN suffix.
	Key string `json:"key"`
	// Generation is 0 for the first occurrence and N for the Nth recurrence.
	Generation int `json:"generation"`
	// RecurrenceOf names the earlier incident this one recurs. It may name one
	// the operator has since pruned.
	RecurrenceOf string `json:"recurrenceOf,omitempty"`
	// PriorIncidents is how many earlier incidents of this fault were recorded.
	PriorIncidents int `json:"priorIncidents,omitempty"`
	// Flapping is true once the operator's recurrence cap was reached and
	// recurrences are being folded into this incident instead of opening new
	// ones.
	Flapping          bool `json:"flapping"`
	RecurrencesFolded int  `json:"recurrencesFolded,omitempty"`
}

// IncidentSignals is the signal family an incident moves through.
type IncidentSignals struct {
	// Family is the progression this incident's signal belongs to, mildest
	// first, and empty when the signal is not part of one.
	Family []string `json:"family,omitempty"`
	// Rank is the 1-based position of the current signal in Family, 0 when it
	// is not in it.
	Rank int `json:"rank,omitempty"`
	// EscalatedFrom is the shapes the incident had before its current one, in
	// order, oldest first. The operator keeps the last 16.
	EscalatedFrom []string     `json:"escalatedFrom,omitempty"`
	EscalatedAt   *metav1.Time `json:"escalatedAt,omitempty"`
	// Containers are the containers the incident is about.
	Containers []string `json:"containers,omitempty"`
	// LastResolution is the resolution a reopened incident replaced, in the
	// operator's own "<outcome>: <action>" form.
	LastResolution string `json:"lastResolution,omitempty"`
}

// splitStageReason splits the operator's "<Stage>_<Reason>" enum. A reason with
// no underscore is returned whole as the guardrail with no stage, rather than
// guessed at.
func splitStageReason(reason string) (stage, guardrail string) {
	stage, guardrail, found := strings.Cut(reason, "_")
	if !found {
		return "", reason
	}
	return stage, guardrail
}

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// projectProposal reads the live ProposalRefused condition.
func projectProposal(conditions []metav1.Condition) *IncidentProposal {
	c := findCondition(conditions, dorguv1.ConditionProposalRefused)
	if c == nil {
		return nil
	}
	out := &IncidentProposal{
		Reason:  c.Reason,
		Message: c.Message,
		Since:   c.LastTransitionTime,
	}
	switch {
	case c.Status == metav1.ConditionTrue && isLegacyDeferral(c.Reason):
		// An operator before the fix wrote the stand-down of an outstanding proposal as
		// a refusal, on every open incident one cycle after proposing. It is waiting for
		// a human, not refused, and `dorgu report` counts it the same way.
		out.State = ProposalDeferred
		out.Stage, out.Guardrail = splitStageReason(c.Reason)
	case c.Status == metav1.ConditionTrue:
		out.State = ProposalRefused
		out.Stage, out.Guardrail = splitStageReason(c.Reason)
	case c.Reason == dorguv1.ReasonProposalDeferred:
		out.State = ProposalDeferred
	case c.Reason == dorguv1.ReasonProposalMade:
		out.State = ProposalMade
	default:
		out.State = ProposalCleared
	}
	return out
}

// isLegacyDeferral reports whether a True ProposalRefused reason is really an
// outstanding proposal: DedupCooldown_ActiveRemediation.
func isLegacyDeferral(reason string) bool {
	stage, guardrail := splitStageReason(reason)
	return stage == "DedupCooldown" && guardrail == "ActiveRemediation"
}

// projectRefusalRecord reads the durable ProposalRefusalRecorded condition.
func projectRefusalRecord(conditions []metav1.Condition) *IncidentRefusalRecord {
	c := findCondition(conditions, dorguv1.ConditionProposalRefusalRecorded)
	if c == nil || c.Status != metav1.ConditionTrue {
		return nil
	}
	stage, guardrail := splitStageReason(c.Reason)
	return &IncidentRefusalRecord{
		Reason:    c.Reason,
		Stage:     stage,
		Guardrail: guardrail,
		Message:   c.Message,
		At:        c.LastTransitionTime,
	}
}

// incidentGeneration splits a name into its lineage key and recurrence number.
//
// The operator additionally shortens very long lineage keys in the middle; the
// dashboard shows the key unshortened, which only differs for names near the
// 253 character limit and is only a label.
func incidentGeneration(name string) (key string, generation int) {
	m := generationSuffix.FindStringSubmatch(name)
	if m == nil {
		return name, 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return name, 0
	}
	return strings.TrimSuffix(name, m[0]), n
}

func projectLineage(incident *dorguv1.IncidentMemory) IncidentLineage {
	key, generation := incidentGeneration(incident.Name)
	out := IncidentLineage{
		Key:          key,
		Generation:   generation,
		RecurrenceOf: incident.Annotations[annotationRecurrenceOf],
	}
	out.PriorIncidents = atoiOrZero(incident.Annotations[annotationRecurrenceCount])
	out.RecurrencesFolded = atoiOrZero(incident.Annotations[annotationRecurrencesFolded])
	if c := findCondition(incident.Status.Conditions, conditionFlapping); c != nil && c.Status == metav1.ConditionTrue {
		out.Flapping = true
	}
	return out
}

func projectSignals(incident *dorguv1.IncidentMemory) IncidentSignals {
	out := IncidentSignals{
		EscalatedFrom:  splitCSV(incident.Annotations[annotationEscalatedFrom]),
		Containers:     splitCSV(incident.Annotations[annotationContainers]),
		LastResolution: incident.Annotations[annotationLastResolution],
	}
	if at, err := time.Parse(time.RFC3339, incident.Annotations[annotationEscalatedAt]); err == nil {
		t := metav1.NewTime(at)
		out.EscalatedAt = &t
	}
	signal := incident.Spec.Detection.Signal
	for i, member := range faultFamily {
		if member == signal {
			out.Family = append([]string(nil), faultFamily...)
			out.Rank = i + 1
		}
	}
	return out
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoiOrZero(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
