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

package fixtures

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

func cond(now time.Time, typ string, status metav1.ConditionStatus, reason, message string, age time.Duration) metav1.Condition {
	return metav1.Condition{
		Type: typ, Status: status, Reason: reason, Message: message, LastTransitionTime: ago(now, age),
	}
}

func personaRef(name string) dorguv1.PersonaReference {
	return dorguv1.PersonaReference{Kind: "ApplicationPersona", Name: name, Namespace: "apps"}
}

func affected(pod string) []dorguv1.ResourceReference {
	return []dorguv1.ResourceReference{{Kind: "Pod", Name: pod, Namespace: "apps", Role: "affected"}}
}

// incidents are the shapes the Incidents and Memory views tell apart: a diagnosed
// OOM with a proposal waiting, a recurrence that escalated and was refused at the
// gate, a deferral, an unattributed outage, and two closed ones, one of which
// keeps a durable refusal record after its live condition was cleared.
func incidents(now time.Time) []*dorguv1.IncidentMemory {
	return []*dorguv1.IncidentMemory{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout-oomkilled", CreationTimestamp: ago(now, 3*time.Hour)},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: personaRef("checkout"), Attribution: "persona", Category: "resource", Severity: "critical",
				Detection: dorguv1.DetectionInfo{
					Signal: "OOMKilled", Source: "pod-failure-detector",
					FirstSeen: ago(now, 3*time.Hour), LastSeen: ago(now, 5*time.Minute),
					AffectedResources: affected("checkout-podinfo-0"),
				},
				RootCause: &dorguv1.RootCauseInfo{
					Summary:    "The podinfo container is OOMKilled at a 128Mi limit while its working set grows under checkout traffic.",
					Confidence: "0.88", Provider: "ai-enhanced",
					Contributing: []dorguv1.ContributingSignal{{Signal: "OOMKilled", Detail: "3 kills in 10 minutes"}},
				},
			},
			Status: dorguv1.IncidentMemoryStatus{
				Phase: "Investigating", OccurrenceCount: 3,
				Conditions: []metav1.Condition{cond(now, "ProposalRefused", metav1.ConditionFalse, "ProposalMade", "", 3*time.Hour)},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "apps", Name: "frontend-crashloop-r2", CreationTimestamp: ago(now, 40*time.Minute),
				Annotations: map[string]string{
					"dorgu.io/recurrence-of":    "frontend-crashloop",
					"dorgu.io/recurrence-count": "1",
					"dorgu.io/escalated-from":   "ReadinessNotReady,RestartLoop",
					"dorgu.io/escalated-at":     now.Add(-12 * time.Minute).UTC().Format(time.RFC3339),
					"dorgu.io/containers":       "web",
					"dorgu.io/last-resolution":  "failed",
				},
			},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: personaRef("frontend"), Attribution: "persona", Category: "deployment", Severity: "critical",
				Detection: dorguv1.DetectionInfo{
					Signal: "CrashLoopBackOff", Source: "pod-failure-detector",
					FirstSeen: ago(now, 40*time.Minute), LastSeen: ago(now, time.Minute),
					AffectedResources: affected("frontend-web-1"),
				},
				RootCause: &dorguv1.RootCauseInfo{
					Summary:    "Image example.internal/frontend:1.9.2 cannot be pulled; three of four replicas never started.",
					Confidence: "0.74", Provider: "ai-enhanced",
				},
			},
			Status: dorguv1.IncidentMemoryStatus{
				Phase: "Recurring", OccurrenceCount: 6,
				Conditions: []metav1.Condition{
					cond(now, "ProposalRefused", metav1.ConditionTrue, "Validate_StepCommand",
						"step 2 runs a command that is not read-only", 30*time.Minute),
					cond(now, "ProposalRefusalRecorded", metav1.ConditionTrue, "Validate_StepCommand",
						"step 2 runs a command that is not read-only", 30*time.Minute),
					cond(now, "Recurring", metav1.ConditionTrue, "RecurrenceOf", "", 40*time.Minute),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "reports-restarts", CreationTimestamp: ago(now, 5*time.Hour)},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: personaRef("reports"), Attribution: "persona", Category: "resource", Severity: "warning",
				Detection: dorguv1.DetectionInfo{
					Signal: "RestartLoop", Source: "pod-failure-detector",
					FirstSeen: ago(now, 5*time.Hour), LastSeen: ago(now, 4*time.Minute),
					AffectedResources: affected("reports-0"),
				},
			},
			Status: dorguv1.IncidentMemoryStatus{
				Phase: "Detected", OccurrenceCount: 4,
				Conditions: []metav1.Condition{cond(now, "ProposalRefused", metav1.ConditionTrue,
					"DedupCooldown_ActiveRemediation", "an earlier proposal for this incident is waiting for a decision", time.Hour)},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "reports-cpu", CreationTimestamp: ago(now, 2*time.Hour)},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: personaRef("reports"), Attribution: "persona", Category: "resource", Severity: "warning",
				Detection: dorguv1.DetectionInfo{
					Signal: "ReadinessNotReady", Source: "pod-failure-detector",
					FirstSeen: ago(now, 2*time.Hour), LastSeen: ago(now, 10*time.Minute),
					AffectedResources: affected("reports-1"),
				},
				RootCause: &dorguv1.RootCauseInfo{Summary: "CPU saturation delays readiness probes.", Confidence: "0.71", Provider: "rule-engine"},
			},
			Status: dorguv1.IncidentMemoryStatus{Phase: "Investigating", OccurrenceCount: 2},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "legacy", Name: "legacy-billing-oom", CreationTimestamp: ago(now, 26*time.Hour)},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef:  dorguv1.PersonaReference{Kind: "ApplicationPersona", Name: "legacy-billing", Namespace: "legacy"},
				Attribution: "unattributed", Category: "resource", Severity: "warning",
				Detection: dorguv1.DetectionInfo{
					Signal: "OOMKilled", Source: "pod-failure-detector",
					FirstSeen: ago(now, 26*time.Hour), LastSeen: ago(now, 3*time.Hour),
					AffectedResources: []dorguv1.ResourceReference{{Kind: "Pod", Name: "legacy-billing-0", Namespace: "legacy", Role: "affected"}},
				},
				RootCause: &dorguv1.RootCauseInfo{Summary: "Container exceeded its memory limit.", Confidence: "0.62", Provider: "rule-engine"},
			},
			Status: dorguv1.IncidentMemoryStatus{Phase: "Detected", OccurrenceCount: 12},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout-cpu-throttle", CreationTimestamp: ago(now, 50*time.Hour)},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: personaRef("checkout"), Attribution: "persona", Category: "resource", Severity: "warning",
				Detection: dorguv1.DetectionInfo{
					Signal: "LivenessRestarts", Source: "pod-failure-detector",
					FirstSeen: ago(now, 50*time.Hour), LastSeen: ago(now, 48*time.Hour),
					AffectedResources: affected("checkout-podinfo-2"),
				},
				RootCause: &dorguv1.RootCauseInfo{Summary: "CPU requests too low for peak traffic.", Confidence: "0.9", Provider: "ai-enhanced"},
				Resolution: &dorguv1.ResolutionInfo{
					Action: "Raised the CPU request", Outcome: "resolved", AppliedAt: ptr(ago(now, 48*time.Hour)),
					RemediationRef: &dorguv1.RemediationReference{Name: "completed-checkout-cpu", Namespace: "apps"},
					Duration:       &metav1.Duration{Duration: 4 * time.Minute},
				},
			},
			Status: dorguv1.IncidentMemoryStatus{
				Phase: "Resolved", OccurrenceCount: 2,
				Conditions: []metav1.Condition{
					cond(now, "ProposalRefused", metav1.ConditionFalse, "IncidentResolved", "", 48*time.Hour),
					cond(now, "ProposalRefusalRecorded", metav1.ConditionTrue, "Validate_StepCommand",
						"the first plan ran a command that is not read-only; the rest of the plan was proposed", 50*time.Hour),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "reports-oom-old", CreationTimestamp: ago(now, 80*time.Hour)},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: personaRef("reports"), Attribution: "persona", Category: "resource", Severity: "info",
				Detection: dorguv1.DetectionInfo{
					Signal: "OOMKilled", Source: "pod-failure-detector",
					FirstSeen: ago(now, 80*time.Hour), LastSeen: ago(now, 79*time.Hour),
					AffectedResources: affected("reports-1"),
				},
				Resolution: &dorguv1.ResolutionInfo{Action: "none", Outcome: "self-resolved"},
			},
			Status: dorguv1.IncidentMemoryStatus{Phase: "Resolved", OccurrenceCount: 1},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "frontend-crashloop", CreationTimestamp: ago(now, 30*time.Hour)},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: personaRef("frontend"), Attribution: "persona", Category: "deployment", Severity: "critical",
				Detection: dorguv1.DetectionInfo{
					Signal: "CrashLoopBackOff", Source: "pod-failure-detector",
					FirstSeen: ago(now, 30*time.Hour), LastSeen: ago(now, 28*time.Hour),
					AffectedResources: affected("frontend-web-0"),
				},
				Resolution: &dorguv1.ResolutionInfo{Action: "Plan refused at approval", Outcome: "failed"},
			},
			Status: dorguv1.IncidentMemoryStatus{Phase: "Resolved", OccurrenceCount: 3},
		},
	}
}

// ---------------------------------------------------------------------------
// RemediationActions
// ---------------------------------------------------------------------------

func verdict(stage, guardrail, kind, reason string, numbers map[string]string) dorguv1.GateVerdict {
	return dorguv1.GateVerdict{Stage: stage, Guardrail: guardrail, Verdict: kind, Reason: reason, Numbers: numbers}
}

// proposalGate is a Proposal-phase gate run. Verdicts after a refusal are not
// written by the operator, which is what lets the UI draw them as "not run".
func proposalGate(now time.Time, age time.Duration, result string, verdicts ...dorguv1.GateVerdict) *dorguv1.GateStatus {
	return &dorguv1.GateStatus{Phase: "Proposal", Result: result, EvaluatedAt: ago(now, age), Verdicts: verdicts}
}

func passAll(extra ...dorguv1.GateVerdict) []dorguv1.GateVerdict {
	out := []dorguv1.GateVerdict{
		verdict("Validate", "step-type", "pass", "", nil),
		verdict("Ground", "live-workload", "pass", "", nil),
		verdict("Ownership", "ownership", "pass", "", nil),
	}
	out = append(out, extra...)
	return append(out,
		verdict("ModePolicy", "self-healing-mode", "pass", "", nil),
		verdict("DedupCooldown", "active-remediation", "pass", "", nil),
	)
}

func blast(field, baseline, requested, ratio, max string) map[string]string {
	return map[string]string{"field": field, "baseline": baseline, "requested": requested, "ratio": ratio, "max": max}
}

func raBase(now time.Time, ns, name, incident string, age time.Duration, persona string) *dorguv1.RemediationAction {
	return &dorguv1.RemediationAction{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, CreationTimestamp: ago(now, age)},
		Spec: dorguv1.RemediationActionSpec{
			IncidentRef: dorguv1.IncidentReference{Name: incident, Namespace: ns},
			PersonaRef:  personaRef(persona),
			TrustLevel:  2, Confidence: "0.85", PlanSource: dorguv1.PlanSourceRuleBased,
			Explanation: "The container was OOMKilled three times in ten minutes.",
			Action:      dorguv1.RemediationActionDetail{Type: dorguv1.ActionTypePersonaUpdate},
		},
		Status: dorguv1.RemediationActionStatus{Phase: "Pending"},
	}
}

func memoryPatch(v string) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(`{"spec":{"resources":{"limits":{"memory":"` + v + `"}}}}`)}
}

func remediations(now time.Time) []*dorguv1.RemediationAction {
	observed := ago(now, 6*time.Minute)
	helm := &dorguv1.WorkloadRef{
		Kind: "Deployment", Name: "checkout-podinfo", Namespace: "apps", Container: "podinfo",
		ManagedBy: dorguv1.ManagedByHelm, ManagedByDetail: `Helm release "checkout" in namespace apps`,
		ObservedResources: &dorguv1.ObservedResources{Limits: &dorguv1.ResourceValues{Memory: "128Mi"}},
		ObservedImage:     "ghcr.io/stefanprodan/podinfo:6.7.1", ObservedAt: &observed,
	}
	unmanaged := &dorguv1.WorkloadRef{
		Kind: "Deployment", Name: "reports", Namespace: "apps", Container: "reports",
		ManagedBy:         dorguv1.ManagedByUnmanaged,
		ObservedResources: &dorguv1.ObservedResources{Limits: &dorguv1.ResourceValues{Memory: "128Mi"}},
		ObservedImage:     "example.internal/reports:2.4.0", ObservedAt: &observed,
	}

	// 1. The clean-room shape: an AI plan against a Helm-owned workload, clamped.
	clamped := raBase(now, "apps", "fix-oom-checkout", "checkout-oomkilled", 3*time.Hour, "checkout")
	clamped.Spec.PlanSource = dorguv1.PlanSourceAIAnthropic
	clamped.Spec.Confidence = "0.88"
	clamped.Spec.PlanSummary = "Raise the memory limit on the podinfo container and watch the rollout."
	clamped.Spec.WorkloadRef = helm
	clamped.Spec.Action.Patch = memoryPatch("256Mi")
	clamped.Spec.Steps = []dorguv1.RemediationStep{{
		Order: 1, ID: "raise-memory", Type: dorguv1.StepTypePersonaUpdate, Risk: "low", AutoExecutable: true,
		Description: "Set spec.resources.limits.memory to 256Mi on the ApplicationPersona.",
		Rationale:   "Raising the memory limit to 4Gi is well within a 2x ceiling and gives the container ample headroom.",
		Patch:       memoryPatch("256Mi"),
		Safety: []dorguv1.StepSafety{
			{
				Rule: dorguv1.SafetyRuleBlastRadius, Verdict: dorguv1.SafetyVerdictClamped,
				Field: "spec.resources.limits.memory", Baseline: "128Mi", Requested: "4Gi", Permitted: "256Mi",
				Ratio: "32.00", MaxRatio: "2.0", Message: "Requested 4Gi against a 128Mi baseline; clamped to 256Mi.",
			},
			{
				Rule: dorguv1.SafetyRuleAbsentField, Verdict: dorguv1.SafetyVerdictRejected,
				Field: "spec.resources.limits.cpu", Message: "The container has no CPU limit, so one is not introduced.",
			},
		},
	}}
	clamped.Status.Gate = proposalGate(now, 3*time.Hour, "Clamp",
		verdict("Validate", "step-type", "pass", "", nil),
		verdict("Ground", "live-workload", "pass", "", nil),
		verdict("Ownership", "ownership", "pass", "Owned by a Helm release: advisory only.", nil),
		verdict("Guardrails", "blast-radius", "clamp", "Memory change exceeds the ceiling and was clamped.",
			blast("spec.resources.limits.memory", "128Mi", "4Gi", "32.00", "2.0")),
		verdict("ModePolicy", "self-healing-mode", "pass", "", nil),
		verdict("DedupCooldown", "active-remediation", "pass", "", nil),
	)
	clamped.Status.Gate.Ownership = &dorguv1.GateOwnership{ManagedBy: dorguv1.ManagedByHelm, Detail: helm.ManagedByDetail, Resolved: true}

	// 2. Rule-based plan against an unmanaged workload: the only writable case.
	reports := raBase(now, "apps", "fix-oom-reports", "reports-restarts", 90*time.Minute, "reports")
	reports.Spec.WorkloadRef = unmanaged
	reports.Spec.Action.Patch = memoryPatch("192Mi")
	reports.Spec.Steps = []dorguv1.RemediationStep{{
		Order: 1, ID: "raise-memory", Type: dorguv1.StepTypePersonaUpdate, Risk: "low", AutoExecutable: true,
		Description: "Set spec.resources.limits.memory to 192Mi.", Patch: memoryPatch("192Mi"),
	}}
	reports.Status.Gate = proposalGate(now, 90*time.Minute, "Pass", passAll(
		verdict("Guardrails", "blast-radius", "pass", "", blast("spec.resources.limits.memory", "128Mi", "192Mi", "1.50", "2.0")))...)
	reports.Status.Gate.Ownership = &dorguv1.GateOwnership{ManagedBy: dorguv1.ManagedByUnmanaged, Resolved: true}

	// 3. Advisory only: nothing in it can be applied.
	advisory := raBase(now, "apps", "restart-frontend", "frontend-crashloop-r2", 30*time.Minute, "frontend")
	advisory.Spec.PlanSource = dorguv1.PlanSourceAIAnthropic
	advisory.Spec.WorkloadRef = &dorguv1.WorkloadRef{
		Kind: "Deployment", Name: "frontend-web", Namespace: "apps", Container: "web",
		ManagedBy: dorguv1.ManagedByArgoCD, ManagedByDetail: `Argo CD application "storefront"`,
	}
	advisory.Spec.Action = dorguv1.RemediationActionDetail{Type: dorguv1.ActionTypeNotification}
	advisory.Spec.Steps = []dorguv1.RemediationStep{{
		Order: 1, ID: "check-registry", Type: dorguv1.StepTypeManual, Risk: "low",
		Description: "Check that the registry credentials for example.internal are still valid.",
		Rationale:   "Three replicas never pulled the image, which usually means an expired pull secret.",
	}}
	advisory.Status.Gate = proposalGate(now, 30*time.Minute, "Pass", passAll()...)
	advisory.Status.Gate.Ownership = &dorguv1.GateOwnership{ManagedBy: dorguv1.ManagedByArgoCD, Detail: `Argo CD application "storefront"`, Resolved: true}

	// 4. Approved and ticketed: the countdown.
	approvedAt := ago(now, 3*time.Minute)
	ticketed := raBase(now, "apps", "ticketed-reports-cpu", "reports-cpu", 2*time.Hour, "reports")
	ticketed.Spec.WorkloadRef = unmanaged
	ticketed.Spec.Action.Patch = &apiextensionsv1.JSON{Raw: []byte(`{"spec":{"resources":{"requests":{"cpu":"400m"}}}}`)}
	ticketed.Spec.Steps = []dorguv1.RemediationStep{{
		Order: 1, ID: "raise-cpu", Type: dorguv1.StepTypePersonaUpdate, Risk: "low", AutoExecutable: true,
		Description: "Set spec.resources.requests.cpu to 400m.",
		Patch:       &apiextensionsv1.JSON{Raw: []byte(`{"spec":{"resources":{"requests":{"cpu":"400m"}}}}`)},
	}}
	ticketed.Status.Phase = "Ticketed"
	ticketed.Status.ApprovedBy = "alex@example.com"
	ticketed.Status.ApprovedAt = &approvedAt
	ticketed.Status.Gate = &dorguv1.GateStatus{
		Phase: "ApprovalRecheck", Result: "Pass", EvaluatedAt: approvedAt,
		Verdicts:  passAll(verdict("Guardrails", "blast-radius", "pass", "", blast("spec.resources.requests.cpu", "250m", "400m", "1.60", "2.0"))),
		Ownership: &dorguv1.GateOwnership{ManagedBy: dorguv1.ManagedByUnmanaged, Resolved: true},
	}
	ticketed.Status.ApplyTicket = &dorguv1.ApplyTicket{
		Container: "reports", IssuedAt: &approvedAt,
		ExpiresAt: metav1.NewTime(now.Add(12*time.Minute + 41*time.Second)),
		Patch:     ticketed.Spec.Action.Patch,
	}

	// 5. Completed and verified: the ticket is consumed and the heal credited.
	appliedAt := ago(now, 48*time.Hour+2*time.Minute)
	done := raBase(now, "apps", "completed-checkout-cpu", "checkout-cpu-throttle", 50*time.Hour, "checkout")
	done.Spec.WorkloadRef = &dorguv1.WorkloadRef{
		Kind: "Deployment", Name: "checkout-podinfo", Namespace: "apps", Container: "podinfo", ManagedBy: dorguv1.ManagedByUnmanaged,
	}
	done.Spec.Steps = []dorguv1.RemediationStep{{
		Order: 1, ID: "raise-cpu", Type: dorguv1.StepTypePersonaUpdate, Risk: "low", AutoExecutable: true,
		Description: "Set spec.resources.requests.cpu to 200m.",
		Patch:       &apiextensionsv1.JSON{Raw: []byte(`{"spec":{"resources":{"requests":{"cpu":"200m"}}}}`)},
	}}
	done.Status.StepStatuses = []dorguv1.StepStatus{{Order: 1, Phase: "Completed", AppliedAt: &appliedAt, VerificationResult: "Healthy"}}
	done.Status.Phase = "Completed"
	done.Status.ApprovedBy = "alex@example.com"
	done.Status.ApprovedAt = ptr(ago(now, 49*time.Hour+50*time.Minute))
	done.Status.AppliedAt = &appliedAt
	done.Status.VerificationResult = "Healthy"
	done.Status.Gate = &dorguv1.GateStatus{
		Phase: "ApprovalRecheck", Result: "Pass", EvaluatedAt: ago(now, 49*time.Hour+50*time.Minute),
		Verdicts: passAll(verdict("Guardrails", "blast-radius", "pass", "", blast("spec.resources.requests.cpu", "100m", "200m", "2.00", "2.0"))),
	}
	done.Status.ApplyTicket = &dorguv1.ApplyTicket{
		Container: "podinfo", IssuedAt: ptr(ago(now, 49*time.Hour+50*time.Minute)), ExpiresAt: ago(now, 49*time.Hour+30*time.Minute),
	}
	done.Status.Conditions = []metav1.Condition{
		cond(now, "HealCredited", metav1.ConditionTrue, "WorkloadPatchedByTicket", "", 48*time.Hour),
		cond(now, "Verified", metav1.ConditionTrue, "HealthRestored", "", 48*time.Hour),
	}

	// 6. Refused at approval: the strip stops at Guardrails and the rest is not run.
	refused := raBase(now, "apps", "refused-frontend-limits", "frontend-crashloop", 30*time.Hour, "frontend")
	refused.Spec.WorkloadRef = &dorguv1.WorkloadRef{
		Kind: "Deployment", Name: "frontend-web", Namespace: "apps", Container: "web", ManagedBy: dorguv1.ManagedByUnmanaged,
	}
	refused.Spec.Action.Patch = memoryPatch("2Gi")
	refused.Spec.Steps = []dorguv1.RemediationStep{{
		Order: 1, ID: "raise-memory", Type: dorguv1.StepTypePersonaUpdate, Risk: "medium", AutoExecutable: true,
		Description: "Set spec.resources.limits.memory to 2Gi.", Patch: memoryPatch("2Gi"),
	}}
	refused.Status.Phase = "Failed"
	refused.Status.ApprovedBy = "alex@example.com"
	refused.Status.ApprovedAt = ptr(ago(now, 29*time.Hour))
	refused.Status.Gate = &dorguv1.GateStatus{
		Phase: "ApprovalRecheck", Result: "Refuse", EvaluatedAt: ago(now, 29*time.Hour),
		Verdicts: []dorguv1.GateVerdict{
			verdict("Validate", "step-type", "pass", "", nil),
			verdict("Ground", "live-workload", "pass", "", nil),
			verdict("Ownership", "ownership", "pass", "", nil),
			verdict("Guardrails", "blast-radius", "refuse", "The live limit changed since the proposal: the change now exceeds 100% of the limit.",
				blast("spec.resources.limits.memory", "512Mi", "2Gi", "4.00", "2.0")),
		},
	}
	refused.Status.Conditions = []metav1.Condition{
		cond(now, "Applied", metav1.ConditionFalse, "GateRefusedAtApproval", "", 29*time.Hour),
		cond(now, "GateRefused", metav1.ConditionTrue, "BlastRadius", "The change exceeds 100% of the live limit.", 29*time.Hour),
	}

	// 7. Expired: nobody decided in time.
	expired := raBase(now, "legacy", "expired-legacy-memory", "legacy-billing-oom", 26*time.Hour, "legacy-billing")
	expired.Spec.PersonaRef = dorguv1.PersonaReference{Kind: "ApplicationPersona", Name: "legacy-billing", Namespace: "legacy"}
	expired.Spec.Action.Patch = memoryPatch("256Mi")
	expired.Spec.Steps = []dorguv1.RemediationStep{{
		Order: 1, ID: "raise-memory", Type: dorguv1.StepTypePersonaUpdate, Risk: "low", AutoExecutable: true,
		Description: "Set spec.resources.limits.memory to 256Mi.", Patch: memoryPatch("256Mi"),
	}}
	expired.Spec.Approval = &dorguv1.ApprovalSpec{Required: true, Deadline: ptr(ago(now, 2*time.Hour))}
	expired.Status.Phase = "Expired"
	expired.Status.Conditions = []metav1.Condition{
		cond(now, "Expired", metav1.ConditionTrue, "ApprovalTimedOut", "No decision before the approval deadline.", 2*time.Hour),
	}

	// 8 and 9. Rejected, with reasons.
	rejected := raBase(now, "apps", "rejected-reports-oom", "reports-oom-old", 80*time.Hour, "reports")
	rejected.Status.Phase = "Rejected"
	rejected.Status.Conditions = []metav1.Condition{
		cond(now, "Rejected", metav1.ConditionTrue, "UserRejected", "", 79*time.Hour),
		cond(now, "RejectionReason", metav1.ConditionTrue, "NotNeeded", "it fixed itself", 79*time.Hour),
	}
	rejectedRisky := raBase(now, "apps", "rejected-checkout-dup", "checkout-oomkilled", 100*time.Hour, "checkout")
	rejectedRisky.Status.Phase = "Rejected"
	rejectedRisky.Status.Conditions = []metav1.Condition{
		cond(now, "Rejected", metav1.ConditionTrue, "UserRejected", "", 99*time.Hour),
		cond(now, "RejectionReason", metav1.ConditionTrue, "TooRisky", "", 99*time.Hour),
	}

	return []*dorguv1.RemediationAction{clamped, reports, advisory, ticketed, done, refused, expired, rejected, rejectedRisky}
}

// actionEvents are the declines the CLI recorded as Events.
func actionEvents(now time.Time) []*corev1.Event {
	mk := func(name, reason string, age time.Duration, count int32) *corev1.Event {
		return &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: name},
			Reason:     reason, Type: corev1.EventTypeWarning, Count: count,
			FirstTimestamp: ago(now, age), LastTimestamp: ago(now, age),
			InvolvedObject: corev1.ObjectReference{Kind: "RemediationAction", Name: "fix-oom-checkout", Namespace: "apps"},
		}
	}
	return []*corev1.Event{
		mk("ev-noticket", "DorguCLINoTicket", 20*time.Minute, 2),
		mk("ev-declined", "DorguCLIDeclined", 40*time.Minute, 1),
	}
}
