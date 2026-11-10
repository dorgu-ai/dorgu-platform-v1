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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

func withStatus(mutate func(*dorguv1.RemediationActionStatus)) func(*dorguv1.RemediationAction) {
	return func(r *dorguv1.RemediationAction) { mutate(&r.Status) }
}

func buildAt(now time.Time, action *dorguv1.RemediationAction) Remediation {
	payload := BuildRemediations(RemediationsInput{
		Remediations: []*dorguv1.RemediationAction{action},
		Now:          now,
	})
	return payload.Remediations[0]
}

func TestTheGateRecordIsProjectedWithEveryVerdictAndItsNumbers(t *testing.T) {
	action := remediation("apps", "fix", withStatus(func(s *dorguv1.RemediationActionStatus) {
		s.Gate = &dorguv1.GateStatus{
			Phase:       "Proposal",
			Result:      "Clamp",
			EvaluatedAt: metav1.NewTime(baseTime),
			Verdicts: []dorguv1.GateVerdict{
				{Stage: "Ground", Guardrail: "workload-exists", Verdict: dorguv1.GateVerdictPass},
				{
					Stage: "Guardrails", Guardrail: "blast-radius", Verdict: dorguv1.GateVerdictClamp,
					Reason:  "memory limit clamped to 2x",
					Numbers: map[string]string{"baseline": "256Mi", "ratio": "16"},
				},
			},
		}
	}))

	row := buildOne(action)

	require.NotNil(t, row.Gate)
	assert.Equal(t, "Proposal", row.Gate.Phase)
	assert.Equal(t, "Clamp", row.Gate.Result)
	require.Len(t, row.Gate.Verdicts, 2)
	assert.Equal(t, "Ground", row.Gate.Verdicts[0].Stage)
	assert.Equal(t, "16", row.Gate.Verdicts[1].Numbers["ratio"], "numbers travel as data")
}

func TestAPlanTheGateNeverWroteHasNoGateBlock(t *testing.T) {
	row := buildOne(remediation("apps", "old"))
	assert.Nil(t, row.Gate)
	assert.Nil(t, row.Ticket)
	assert.Nil(t, row.Expired)
	assert.Nil(t, row.GateRefusal)
}

func TestTicketState(t *testing.T) {
	now := baseTime.Add(time.Hour)
	issued := metav1.NewTime(baseTime.Add(50 * time.Minute))
	appliedAt := metav1.NewTime(baseTime.Add(55 * time.Minute))

	tests := []struct {
		name      string
		phase     string
		expiresAt time.Time
		appliedAt *metav1.Time
		want      string
	}{
		{"ticketed and live", RemediationTicketed, now.Add(10 * time.Minute), nil, TicketIssued},
		{"ticketed and past expiry", RemediationTicketed, now.Add(-time.Minute), nil, TicketExpired},
		{"expiry exactly now is expired", RemediationTicketed, now, nil, TicketExpired},
		{"applying is consumed", RemediationApplying, now.Add(10 * time.Minute), nil, TicketConsumed},
		{"verifying is consumed", RemediationVerifying, now.Add(10 * time.Minute), nil, TicketConsumed},
		{"completed is consumed even past expiry", RemediationCompleted, now.Add(-time.Hour), nil, TicketConsumed},
		{"rolled back is consumed", RemediationRolledBack, now.Add(-time.Hour), nil, TicketConsumed},
		{"appliedAt marks a failed apply consumed", RemediationFailed, now.Add(-time.Hour), &appliedAt, TicketConsumed},
		{"a failed plan that never applied keeps its expiry", RemediationFailed, now.Add(-time.Hour), nil, TicketExpired},
		{"zero expiry is not treated as expired", RemediationTicketed, time.Time{}, nil, TicketIssued},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action := remediation("apps", "fix", withStatus(func(s *dorguv1.RemediationActionStatus) {
				s.Phase = tc.phase
				s.AppliedAt = tc.appliedAt
				s.ApplyTicket = &dorguv1.ApplyTicket{
					Container: "podinfo",
					IssuedAt:  &issued,
					ExpiresAt: metav1.NewTime(tc.expiresAt),
					Verdicts: []dorguv1.GateVerdict{
						{Stage: "Guardrails", Guardrail: "blast-radius", Verdict: dorguv1.GateVerdictPass},
					},
				}
			}))
			row := buildAt(now, action)
			require.NotNil(t, row.Ticket)
			assert.Equal(t, tc.want, row.Ticket.State)
			assert.Equal(t, "podinfo", row.Ticket.Container)
			assert.Len(t, row.Ticket.Verdicts, 1)
		})
	}
}

func TestExpiredReasons(t *testing.T) {
	for _, reason := range []string{
		ExpiredApprovalTimedOut, ExpiredSuperseded, ExpiredPredatesGate, ExpiredIncidentResolved,
	} {
		t.Run(reason, func(t *testing.T) {
			action := remediation("apps", "fix", withStatus(func(s *dorguv1.RemediationActionStatus) {
				s.Phase = RemediationExpired
				s.Conditions = []metav1.Condition{
					{Type: "Expired", Status: metav1.ConditionTrue, Reason: reason, Message: "because"},
				}
			}))
			row := buildOne(action)
			require.NotNil(t, row.Expired)
			assert.Equal(t, reason, row.Expired.Reason)
			assert.Equal(t, "because", row.Expired.Message)
		})
	}
}

func TestAnExpiredConditionOnALivePlanIsNotReportedAsExpiry(t *testing.T) {
	action := remediation("apps", "fix", withStatus(func(s *dorguv1.RemediationActionStatus) {
		s.Phase = RemediationApproved
		s.Conditions = []metav1.Condition{
			{Type: "Expired", Status: metav1.ConditionTrue, Reason: ExpiredSuperseded},
		}
	}))
	assert.Nil(t, buildOne(action).Expired)
}

func TestAnExpiredPhaseWithNoConditionHasNoReason(t *testing.T) {
	action := remediation("apps", "fix", withStatus(func(s *dorguv1.RemediationActionStatus) {
		s.Phase = RemediationExpired
	}))
	assert.Nil(t, buildOne(action).Expired)
}

func TestAGateRefusalAtApprovalIsReported(t *testing.T) {
	action := remediation("apps", "fix", withStatus(func(s *dorguv1.RemediationActionStatus) {
		s.Conditions = []metav1.Condition{
			{Type: dorguv1.ConditionGateRefused, Status: metav1.ConditionTrue, Reason: "BlastRadius", Message: "too big"},
			{Type: dorguv1.ConditionGateRefused + "Other", Status: metav1.ConditionTrue, Reason: "Nope"},
		}
	}))
	row := buildOne(action)
	require.NotNil(t, row.GateRefusal)
	assert.Equal(t, "BlastRadius", row.GateRefusal.Reason)
}

func TestAFalseGateRefusedConditionIsNotARefusal(t *testing.T) {
	action := remediation("apps", "fix", withStatus(func(s *dorguv1.RemediationActionStatus) {
		s.Conditions = []metav1.Condition{
			{Type: dorguv1.ConditionGateRefused, Status: metav1.ConditionFalse, Reason: "BlastRadius"},
		}
	}))
	assert.Nil(t, buildOne(action).GateRefusal)
}

// ---------------------------------------------------------------------------
// Ownership: the gate's record outranks what the spec's author wrote
// ---------------------------------------------------------------------------

func gateOwnership(managedBy, detail string, resolved bool) func(*dorguv1.RemediationAction) {
	return withStatus(func(s *dorguv1.RemediationActionStatus) {
		s.Gate = &dorguv1.GateStatus{
			Phase: "Proposal", Result: "Pass",
			Ownership: &dorguv1.GateOwnership{ManagedBy: managedBy, Detail: detail, Resolved: resolved},
		}
	})
}

func TestTheGatesOwnershipOverridesASpecThatClaimsUnmanaged(t *testing.T) {
	// Anyone who can write the object can write managedBy: unmanaged. The gate
	// read the live Deployment.
	action := remediation("apps", "fix", unmanaged,
		gateOwnership(dorguv1.ManagedByArgoCD, `Argo CD application "shop"`, true))

	row := buildOne(action)

	require.NotNil(t, row.Workload)
	assert.Equal(t, dorguv1.ManagedByArgoCD, row.Workload.ManagedBy)
	assert.Equal(t, `Argo CD application "shop"`, row.Workload.ManagedByDetail)
	assert.Equal(t, "gate", row.Workload.OwnershipSource)
	assert.True(t, row.Workload.Owned)
	assert.True(t, row.AdvisoryOnly)
	assert.NotEmpty(t, row.Workload.WhyNotPatched)
	assert.Equal(t, "reports", row.Workload.Name, "the rest of the ref is kept")
}

func TestTheGatesOwnershipCanClearAStaleOwnedSpec(t *testing.T) {
	action := remediation("apps", "fix", helmOwned, gateOwnership(dorguv1.ManagedByUnmanaged, "", true))
	row := buildOne(action)
	assert.False(t, row.Workload.Owned)
	assert.False(t, row.AdvisoryOnly)
	assert.Equal(t, "gate", row.Workload.OwnershipSource)
}

func TestAnUnresolvedGateOwnerIsTreatedAsOwned(t *testing.T) {
	action := remediation("apps", "fix", unmanaged, gateOwnership(dorguv1.ManagedByUnknown, "", false))
	row := buildOne(action)
	assert.True(t, row.Workload.Owned)
	assert.True(t, row.AdvisoryOnly)
}

func TestWithoutAGateOwnerTheSpecIsUsedAndSaysSo(t *testing.T) {
	row := buildOne(remediation("apps", "fix", helmOwned))
	assert.Equal(t, "spec", row.Workload.OwnershipSource)
	assert.True(t, row.AdvisoryOnly)

	row = buildOne(remediation("apps", "fix", unmanaged))
	assert.False(t, row.AdvisoryOnly)
}

func TestAGateOwnerWithAnEmptyClassIsIgnored(t *testing.T) {
	action := remediation("apps", "fix", helmOwned, gateOwnership("  ", "", true))
	row := buildOne(action)
	assert.Equal(t, dorguv1.ManagedByHelm, row.Workload.ManagedBy)
	assert.Equal(t, "spec", row.Workload.OwnershipSource)
}

func TestAuthoritativeOwnershipDoesNotMutateTheCachedObject(t *testing.T) {
	action := remediation("apps", "fix", unmanaged,
		gateOwnership(dorguv1.ManagedByFlux, "", true))
	buildOne(action)
	assert.Equal(t, dorguv1.ManagedByUnmanaged, action.Spec.WorkloadRef.ManagedBy,
		"the informer cache must not be rewritten by a projection")
}

func TestAnAcknowledgedOwnedPlanIsAdvisoryOnly(t *testing.T) {
	action := remediation("apps", "fix", helmOwned, withStatus(func(s *dorguv1.RemediationActionStatus) {
		s.Phase = RemediationAcknowledged
	}))
	row := buildOne(action)
	assert.True(t, row.AdvisoryOnly)
	assert.Equal(t, RemediationAcknowledged, row.Phase)
	assert.Nil(t, row.Ticket, "an acknowledged plan issued no ticket")
}
