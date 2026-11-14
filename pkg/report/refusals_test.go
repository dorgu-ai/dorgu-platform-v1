package report

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// B10 (f), D1.1 finding 4: `dorgu report` showed deferred_proposals: 0 and an empty refusal
// mix while the operator's counters held 25 deferrals and 3 Validate/step-command refusals.
//
// Deferrals are deliberately not stored on cluster objects (B6), so 0 was a statement the
// report could not make: it now says they are not recorded, and where to read them.
//
// The 3 refusals were stored and then erased: a ProposalRefused condition is rewritten to
// False when the incident resolves, or when a later proposal answers it (the fallback that
// proposes the rest of a refused plan does that at once), and the report reads only a True
// one. The operator (v0.13.1) now also keeps ProposalRefusalRecorded, which is never
// cleared, and the report counts a refusal from it.

const refusalRecorded = "ProposalRefusalRecorded"

func TestReportCountsARefusalThatResolvedAfterwards(t *testing.T) {
	h := time.Hour
	in := Inputs{Incidents: []dorguv1.IncidentMemory{
		// The incident resolved: the live condition was cleared, the record stands.
		mustIncident(t, incidentJSON("inc-resolved", 5*h, 5*h, "self-resolved",
			cond("ProposalRefused", "False", "IncidentResolved", h),
			cond(refusalRecorded, "True", "Validate_StepCommand", 3*h))),
	}}

	q6 := Build(in, testNow, fixtureWindow).TierQ.Q6Refusals

	require.Len(t, q6.Rows, 1)
	assert.Equal(t, "proposal", q6.Rows[0].Phase)
	assert.Equal(t, "Validate", q6.Rows[0].Stage)
	assert.Equal(t, "StepCommand", q6.Rows[0].Reason)
	assert.Equal(t, 1, q6.Rows[0].Count)
	assert.Equal(t, 1, q6.Proposals, "a proposal that was refused and never written is one proposal")
}

// A refusal followed by a proposal for the same incident (the fallback that proposes the rest
// of a refused plan): the refusal is counted, the proposal is not counted twice.
func TestReportCountsARefusalAnAnswerClearedWithoutDoublingTheProposal(t *testing.T) {
	h := time.Hour
	in := Inputs{
		Incidents: []dorguv1.IncidentMemory{mustIncident(t, incidentJSON("inc-fallback", 5*h, 5*h, "",
			cond("ProposalRefused", "False", "ProposalMade", 3*h),
			cond(refusalRecorded, "True", "Validate_StepCommand", 3*h)))},
		Remediations: []dorguv1.RemediationAction{mustRA(t, raOpts{name: "ra-fallback", phase: "Pending", created: 3 * h, incident: "inc-fallback"})},
	}

	q6 := Build(in, testNow, fixtureWindow).TierQ.Q6Refusals

	require.Len(t, q6.Rows, 1)
	assert.Equal(t, 1, q6.Rows[0].Count)
	assert.Equal(t, 1, q6.Proposals, "the RemediationAction is the one proposal")
}

// A refusal that is still standing is on the incident twice (the live condition and the
// record) and is one refusal.
func TestReportCountsAStandingRefusalOnce(t *testing.T) {
	h := time.Hour
	in := Inputs{Incidents: []dorguv1.IncidentMemory{
		mustIncident(t, incidentJSON("inc-standing", 5*h, 5*h, "",
			cond("ProposalRefused", "True", "ModePolicy_SelfHealingMode", 3*h),
			cond(refusalRecorded, "True", "ModePolicy_SelfHealingMode", 3*h))),
	}}

	q6 := Build(in, testNow, fixtureWindow).TierQ.Q6Refusals

	require.Len(t, q6.Rows, 1)
	assert.Equal(t, 1, q6.Rows[0].Count)
	assert.Equal(t, 1, q6.Proposals)
}

// A refusal recorded before the window is not in it.
func TestReportLeavesOutARecordedRefusalFromBeforeTheWindow(t *testing.T) {
	in := Inputs{Incidents: []dorguv1.IncidentMemory{
		mustIncident(t, incidentJSON("inc-old", 20*24*time.Hour, 20*24*time.Hour, "resolved",
			cond(refusalRecorded, "True", "Validate_StepCommand", 10*24*time.Hour))),
	}}

	q6 := Build(in, testNow, fixtureWindow).TierQ.Q6Refusals

	assert.Empty(t, q6.Rows)
}

// An operator before v0.13.1 writes no record, and a refusal it cleared is gone: that is
// as before, and the live condition is still read.
func TestReportStillReadsTheLiveConditionOfAnOlderOperator(t *testing.T) {
	h := time.Hour
	in := Inputs{Incidents: []dorguv1.IncidentMemory{
		mustIncident(t, incidentJSON("inc-live", 5*h, 5*h, "", cond("ProposalRefused", "True", "Ground_WorkloadGone", 2*h))),
		mustIncident(t, incidentJSON("inc-cleared", 5*h, 5*h, "", cond("ProposalRefused", "False", "ProposalMade", 2*h))),
	}}

	q6 := Build(in, testNow, fixtureWindow).TierQ.Q6Refusals

	require.Len(t, q6.Rows, 1)
	assert.Equal(t, "WorkloadGone", q6.Rows[0].Reason)
}

// --- deferrals are not recorded ---------------------------------------------------------

const deferralsNote = "not recorded on cluster objects (see the dorgu_gate_decisions_total metric, result=deferred)"

func TestReportDoesNotClaimZeroDeferrals(t *testing.T) {
	d := Build(Inputs{Incidents: []dorguv1.IncidentMemory{
		mustIncident(t, incidentJSON("i", 5*time.Hour, 5*time.Hour, "", cond("ProposalRefused", "True", "ModePolicy_SelfHealingMode", time.Hour))),
	}}, testNow, fixtureWindow)

	q6 := d.TierQ.Q6Refusals
	assert.Nil(t, q6.Deferred, "no number: the operator does not record deferrals on objects")
	assert.Equal(t, deferralsNote, q6.DeferredNote)

	raw, err := json.Marshal(d)
	require.NoError(t, err)
	var generic map[string]any
	require.NoError(t, json.Unmarshal(raw, &generic))
	refusals := generic["tier_q"].(map[string]any)["q6_refusal_mix"].(map[string]any)
	assert.Contains(t, refusals, "deferred_proposals")
	assert.Nil(t, refusals["deferred_proposals"], "null, not 0")
	assert.Equal(t, deferralsNote, refusals["deferred_note"])
}

// Deferrals an operator before the fix wrote onto incidents are still counted, and the note says
// the count is partial.
func TestReportCountsTheDeferralsAnOlderOperatorWrote(t *testing.T) {
	d := Build(Inputs{Incidents: []dorguv1.IncidentMemory{
		mustIncident(t, incidentJSON("i", 3*time.Hour, 3*time.Hour, "", cond("ProposalRefused", "True", "DedupCooldown_ActiveRemediation", 2*time.Hour))),
	}}, testNow, fixtureWindow)

	require.NotNil(t, d.TierQ.Q6Refusals.Deferred)
	assert.Equal(t, 1, *d.TierQ.Q6Refusals.Deferred)
	assert.Equal(t, deferralsNote, d.TierQ.Q6Refusals.DeferredNote)
}

// --- B10 (g): `dorgu health` counts Pending proposals only -------------------------------
