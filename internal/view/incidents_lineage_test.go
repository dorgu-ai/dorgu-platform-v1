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

func cond(t, status, reason, msg string) metav1.Condition {
	return metav1.Condition{
		Type: t, Status: metav1.ConditionStatus(status), Reason: reason, Message: msg,
		LastTransitionTime: metav1.NewTime(baseTime.Add(time.Minute)),
	}
}

func withConditions(c ...metav1.Condition) func(*dorguv1.IncidentMemory) {
	return func(i *dorguv1.IncidentMemory) { i.Status.Conditions = c }
}

func withAnnotations(a map[string]string) func(*dorguv1.IncidentMemory) {
	return func(i *dorguv1.IncidentMemory) { i.Annotations = a }
}

func buildIncident(i *dorguv1.IncidentMemory) Incident {
	return BuildIncidents(IncidentsInput{Incidents: []*dorguv1.IncidentMemory{i}}).Incidents[0]
}

func TestProposalStates(t *testing.T) {
	tests := []struct {
		name          string
		condition     metav1.Condition
		wantState     string
		wantStage     string
		wantGuardrail string
	}{
		{
			"refused splits stage and guardrail",
			cond(dorguv1.ConditionProposalRefused, "True", "ModePolicy_SelfHealingMode", "mode is off"),
			ProposalRefused, "ModePolicy", "SelfHealingMode",
		},
		{
			"a refusal keeps everything after the first underscore as the guardrail",
			cond(dorguv1.ConditionProposalRefused, "True", "Ground_Workload_Gone", ""),
			ProposalRefused, "Ground", "Workload_Gone",
		},
		{
			"a refusal reason with no underscore is not guessed at",
			cond(dorguv1.ConditionProposalRefused, "True", "Weird", ""),
			ProposalRefused, "", "Weird",
		},
		{
			"deferred is not a refusal",
			cond(dorguv1.ConditionProposalRefused, "False", dorguv1.ReasonProposalDeferred, "outstanding"),
			ProposalDeferred, "", "",
		},
		{
			"an older operator wrote the stand-down for an outstanding proposal as a refusal: still a deferral",
			cond(dorguv1.ConditionProposalRefused, "True", "DedupCooldown_ActiveRemediation", "waiting"),
			ProposalDeferred, "DedupCooldown", "ActiveRemediation",
		},
		{
			"a cooldown after a rejection is a real refusal",
			cond(dorguv1.ConditionProposalRefused, "True", "DedupCooldown_RejectionCooldown", ""),
			ProposalRefused, "DedupCooldown", "RejectionCooldown",
		},
		{
			"made",
			cond(dorguv1.ConditionProposalRefused, "False", dorguv1.ReasonProposalMade, ""),
			ProposalMade, "", "",
		},
		{
			"resolved incident clears the refusal",
			cond(dorguv1.ConditionProposalRefused, "False", dorguv1.ReasonIncidentResolved, ""),
			ProposalCleared, "", "",
		},
		{
			"an unknown false reason reads as cleared, not refused",
			cond(dorguv1.ConditionProposalRefused, "False", "SomethingNew", ""),
			ProposalCleared, "", "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := buildIncident(incident("apps", "im-a", "checkout", SeverityWarning, PhaseDetected,
				withConditions(tc.condition)))
			require.NotNil(t, row.Proposal)
			assert.Equal(t, tc.wantState, row.Proposal.State)
			assert.Equal(t, tc.wantStage, row.Proposal.Stage)
			assert.Equal(t, tc.wantGuardrail, row.Proposal.Guardrail)
			assert.Equal(t, tc.condition.Reason, row.Proposal.Reason)
			assert.Equal(t, tc.condition.Message, row.Proposal.Message)
			assert.True(t, row.Proposal.Since.Equal(&tc.condition.LastTransitionTime))
		})
	}
}

func TestNoProposalConditionMeansNoProposalBlock(t *testing.T) {
	row := buildIncident(incident("apps", "im-a", "checkout", SeverityWarning, PhaseDetected))
	assert.Nil(t, row.Proposal)
	assert.Nil(t, row.RefusalRecord)
}

func TestTheRefusalRecordSurvivesAResolvedIncident(t *testing.T) {
	row := buildIncident(incident("apps", "im-a", "checkout", SeverityWarning, PhaseResolved,
		withConditions(
			cond(dorguv1.ConditionProposalRefused, "False", dorguv1.ReasonIncidentResolved, ""),
			cond(dorguv1.ConditionProposalRefusalRecorded, "True", "Guardrails_BlastRadius", "ratio too high"),
		)))

	require.NotNil(t, row.Proposal)
	assert.Equal(t, ProposalCleared, row.Proposal.State)
	require.NotNil(t, row.RefusalRecord)
	assert.Equal(t, "Guardrails", row.RefusalRecord.Stage)
	assert.Equal(t, "BlastRadius", row.RefusalRecord.Guardrail)
	assert.Equal(t, "ratio too high", row.RefusalRecord.Message)
}

func TestAFalseRefusalRecordedConditionIsIgnored(t *testing.T) {
	row := buildIncident(incident("apps", "im-a", "checkout", SeverityWarning, PhaseDetected,
		withConditions(cond(dorguv1.ConditionProposalRefusalRecorded, "False", "Guardrails_BlastRadius", ""))))
	assert.Nil(t, row.RefusalRecord)
}

func TestLineageFromTheName(t *testing.T) {
	tests := []struct {
		name    string
		wantKey string
		wantGen int
	}{
		{"im-apps-checkout-oomkilled-abc123def456", "im-apps-checkout-oomkilled-abc123def456", 0},
		{"im-apps-checkout-oomkilled-abc123def456-r1", "im-apps-checkout-oomkilled-abc123def456", 1},
		{"im-apps-checkout-oomkilled-abc123def456-r12", "im-apps-checkout-oomkilled-abc123def456", 12},
		{"im-apps-checkout-r2-abc123def456", "im-apps-checkout-r2-abc123def456", 0},
		{"checkout-r", "checkout-r", 0},
		{"checkout-r99999999999999999999", "checkout-r99999999999999999999", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, gen := incidentGeneration(tc.name)
			assert.Equal(t, tc.wantKey, key)
			assert.Equal(t, tc.wantGen, gen)
		})
	}
}

func TestARecurrenceCarriesItsLineageAndFlappingState(t *testing.T) {
	row := buildIncident(incident("apps", "im-x-abc123def456-r2", "checkout", SeverityWarning, PhaseDetected,
		withAnnotations(map[string]string{
			"dorgu.io/recurrence-of":      "im-x-abc123def456-r1",
			"dorgu.io/recurrence-count":   "2",
			"dorgu.io/recurrences-folded": "4",
		}),
		withConditions(cond("Flapping", "True", "RecurrenceCapReached", ""))))

	assert.Equal(t, "im-x-abc123def456", row.Lineage.Key)
	assert.Equal(t, 2, row.Lineage.Generation)
	assert.Equal(t, "im-x-abc123def456-r1", row.Lineage.RecurrenceOf)
	assert.Equal(t, 2, row.Lineage.PriorIncidents)
	assert.Equal(t, 4, row.Lineage.RecurrencesFolded)
	assert.True(t, row.Lineage.Flapping)
}

func TestAFirstOccurrenceIsGenerationZeroAndNotFlapping(t *testing.T) {
	row := buildIncident(incident("apps", "im-x-abc123def456", "checkout", SeverityWarning, PhaseDetected,
		withConditions(cond("Flapping", "False", "RecurrenceCapReached", ""))))
	assert.Equal(t, 0, row.Lineage.Generation)
	assert.Empty(t, row.Lineage.RecurrenceOf)
	assert.False(t, row.Lineage.Flapping)
}

func TestMalformedCountAnnotationsReadAsZero(t *testing.T) {
	row := buildIncident(incident("apps", "im-x-abc123def456-r1", "checkout", SeverityWarning, PhaseDetected,
		withAnnotations(map[string]string{
			"dorgu.io/recurrence-count":   "many",
			"dorgu.io/recurrences-folded": "-3",
		})))
	assert.Equal(t, 0, row.Lineage.PriorIncidents)
	assert.Equal(t, 0, row.Lineage.RecurrencesFolded)
}

func TestSignalFamilyAndEscalationTrail(t *testing.T) {
	row := buildIncident(incident("apps", "im-x-abc123def456", "checkout", SeverityCritical, PhaseDetected,
		withAnnotations(map[string]string{
			"dorgu.io/escalated-from":  "ReadinessNotReady, RestartLoop,,CrashLoopBackOff",
			"dorgu.io/escalated-at":    "2026-08-27T03:05:00Z",
			"dorgu.io/containers":      "app,sidecar",
			"dorgu.io/last-resolution": "resolved: raise memory",
		})))

	assert.Equal(t, "OOMKilled", row.Signal)
	assert.Equal(t, 5, row.Signals.Rank, "OOMKilled is the worst of five")
	assert.Equal(t, []string{"ReadinessNotReady", "RestartLoop", "LivenessRestarts", "CrashLoopBackOff", "OOMKilled"},
		row.Signals.Family)
	assert.Equal(t, []string{"ReadinessNotReady", "RestartLoop", "CrashLoopBackOff"}, row.Signals.EscalatedFrom)
	assert.Equal(t, []string{"app", "sidecar"}, row.Signals.Containers)
	require.NotNil(t, row.Signals.EscalatedAt)
	assert.Equal(t, time.Date(2026, 8, 27, 3, 5, 0, 0, time.UTC), row.Signals.EscalatedAt.UTC())
	assert.Equal(t, "resolved: raise memory", row.Signals.LastResolution)
}

func TestASignalOutsideTheFamilyHasNoFamily(t *testing.T) {
	row := buildIncident(incident("apps", "im-x", "checkout", SeverityWarning, PhaseDetected,
		func(i *dorguv1.IncidentMemory) { i.Spec.Detection.Signal = "ImagePullBackOff" }))
	assert.Empty(t, row.Signals.Family)
	assert.Zero(t, row.Signals.Rank)
	assert.Nil(t, row.Signals.EscalatedAt)
}

func TestAnUnparseableEscalationTimeIsDropped(t *testing.T) {
	row := buildIncident(incident("apps", "im-x", "checkout", SeverityWarning, PhaseDetected,
		withAnnotations(map[string]string{"dorgu.io/escalated-at": "yesterday"})))
	assert.Nil(t, row.Signals.EscalatedAt)
}

func TestTheFamilyListIsACopy(t *testing.T) {
	row := buildIncident(incident("apps", "im-x", "checkout", SeverityWarning, PhaseDetected))
	row.Signals.Family[0] = "mutated"
	again := buildIncident(incident("apps", "im-x", "checkout", SeverityWarning, PhaseDetected))
	assert.Equal(t, "ReadinessNotReady", again.Signals.Family[0])
}
