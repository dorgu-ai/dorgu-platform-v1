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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// What the operator's gate established about a plan, read-only.
//
// Everything here is written by the operator from the live cluster and never
// taken from whoever authored the spec, which is why it is projected as its own
// block instead of being folded into the step fields. Where the gate and the
// spec disagree (workloadRef.managedBy above all), the gate wins: any principal
// that can write a RemediationAction can write spec.workloadRef.

// Apply ticket states. The ticket is what the gate cleared at approval and the
// only thing that may be applied.
const (
	// TicketIssued is a ticket that exists, has not been used and has not run
	// out.
	TicketIssued = "issued"
	// TicketExpired is a ticket that was never used and is past expiresAt.
	// Nothing is applied under one.
	TicketExpired = "expired"
	// TicketConsumed is a ticket the plan was applied under: the apply has
	// started or finished.
	TicketConsumed = "consumed"
)

// Condition types the operator writes on a RemediationAction that this view
// reads. They are the operator's own constants where it exports one; Expired
// lives in the operator's controller package, which is not importable.
const (
	conditionExpired = "Expired"
)

// Why an Expired plan expired, the four reasons the operator writes.
const (
	ExpiredApprovalTimedOut = "ApprovalTimedOut"
	ExpiredSuperseded       = "Superseded"
	ExpiredPredatesGate     = "PredatesGate"
	ExpiredIncidentResolved = "IncidentResolved"
)

// RemediationGate is the operator's gate record for a plan.
type RemediationGate struct {
	// Phase is Proposal or ApprovalRecheck: which run produced this record.
	Phase string `json:"phase"`
	// Result is Pass, Clamp or Refuse.
	Result      string      `json:"result"`
	EvaluatedAt metav1.Time `json:"evaluatedAt"`
	// Verdicts is every guardrail the gate ran, in the order it ran them. The
	// stage is on each one so the UI can group per stage. Reason is a sentence
	// the operator guarantees contains no model text, and Numbers carries the
	// inputs as data, so nothing here is parsed out of prose.
	Verdicts []dorguv1.GateVerdict `json:"verdicts,omitempty"`
}

// RemediationTicket is the apply ticket, summarised.
type RemediationTicket struct {
	// State is TicketIssued, TicketExpired or TicketConsumed.
	State     string       `json:"state"`
	IssuedAt  *metav1.Time `json:"issuedAt,omitempty"`
	ExpiresAt metav1.Time  `json:"expiresAt"`
	Container string       `json:"container,omitempty"`
	// Verdicts is what the approval re-check decided, kept apart from the
	// proposal-time verdicts in the gate block.
	Verdicts []dorguv1.GateVerdict `json:"verdicts,omitempty"`
}

// RemediationReason is a condition's machine reason and its sentence.
type RemediationReason struct {
	// Reason is the CamelCase enum, e.g. Superseded or BlastRadius.
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

// projectGate returns the gate block, or nil on a plan the gate never wrote.
func projectGate(status dorguv1.RemediationActionStatus) *RemediationGate {
	gate := status.Gate
	if gate == nil {
		return nil
	}
	return &RemediationGate{
		Phase:       gate.Phase,
		Result:      gate.Result,
		EvaluatedAt: gate.EvaluatedAt,
		Verdicts:    gate.Verdicts,
	}
}

// projectTicket summarises the apply ticket, or nil when none was issued.
func projectTicket(status dorguv1.RemediationActionStatus, now time.Time) *RemediationTicket {
	ticket := status.ApplyTicket
	if ticket == nil {
		return nil
	}
	return &RemediationTicket{
		State:     ticketState(status, now),
		IssuedAt:  ticket.IssuedAt,
		ExpiresAt: ticket.ExpiresAt,
		Container: ticket.Container,
		Verdicts:  ticket.Verdicts,
	}
}

// ticketState decides issued, expired or consumed.
//
// Consumed wins over expired: a ticket the apply started under is spent, and
// reporting it as expired because the clock moved on afterwards would say the
// change never happened. Consumption is read from what the apply left behind
// (appliedAt, or a phase past Ticketed), since the operator does not write a
// "used" flag.
func ticketState(status dorguv1.RemediationActionStatus, now time.Time) string {
	switch status.Phase {
	case RemediationApplying, RemediationVerifying, RemediationCompleted, RemediationRolledBack:
		return TicketConsumed
	}
	if status.AppliedAt != nil {
		return TicketConsumed
	}
	if !status.ApplyTicket.ExpiresAt.IsZero() && !now.Before(status.ApplyTicket.ExpiresAt.Time) {
		return TicketExpired
	}
	return TicketIssued
}

// conditionReason returns the reason of the first condition of the given type
// that is True, and ok=false when there is none.
func conditionReason(conditions []metav1.Condition, conditionType string) (RemediationReason, bool) {
	for _, c := range conditions {
		if c.Type == conditionType && c.Status == metav1.ConditionTrue {
			return RemediationReason{Reason: c.Reason, Message: c.Message}, true
		}
	}
	return RemediationReason{}, false
}

// expiredReason says why a plan expired, only while it actually is Expired.
func expiredReason(action *dorguv1.RemediationAction) *RemediationReason {
	if action.Status.Phase != RemediationExpired {
		return nil
	}
	reason, ok := conditionReason(action.Status.Conditions, conditionExpired)
	if !ok {
		return nil
	}
	return &reason
}

// gateRefusal reports the guardrail that refused a plan at approval.
func gateRefusal(action *dorguv1.RemediationAction) *RemediationReason {
	reason, ok := conditionReason(action.Status.Conditions, dorguv1.ConditionGateRefused)
	if !ok {
		return nil
	}
	return &reason
}

// authoritativeWorkloadRef returns the workload ref with its ownership taken
// from the operator's gate when the gate recorded one.
//
// spec.workloadRef.managedBy is writable by anyone who can create or update a
// RemediationAction; status.gate.ownership is written by the operator, from the
// live Deployment. The second is the one to show. The input is not mutated.
// ownershipSource is "gate" when the override applied, "spec" otherwise.
func authoritativeWorkloadRef(
	action *dorguv1.RemediationAction,
) (ref *dorguv1.WorkloadRef, ownershipSource string) {
	spec := action.Spec.WorkloadRef
	gate := action.Status.Gate
	if spec == nil || gate == nil || gate.Ownership == nil || strings.TrimSpace(gate.Ownership.ManagedBy) == "" {
		return spec, "spec"
	}
	merged := *spec
	merged.ManagedBy = gate.Ownership.ManagedBy
	merged.ManagedByDetail = gate.Ownership.Detail
	return &merged, "gate"
}
