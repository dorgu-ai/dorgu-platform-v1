package report

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

func TestReportClassifiesEveryTerminalPhase(t *testing.T) {
	cases := map[string]string{
		"Expired": ExcludedExpired, "Acknowledged": ExcludedAcknowledged, "RolledBack": ExcludedRolledBack,
		"Failed": ExcludedFailed, "Applying": ExcludedInFlight, "Verifying": ExcludedInFlight,
		"Ticketed": ExcludedInFlight, "": ExcludedInFlight,
	}
	for phase, want := range cases {
		ra := mustRA(t, raOpts{name: "x", phase: phase, created: time.Hour})
		got := Build(Inputs{Remediations: []dorguv1.RemediationAction{ra}}, testNow, fixtureWindow)
		assert.Equal(t, 1, got.HealAccounting.Excluded[want], "phase %q", phase)
		assert.Equal(t, 0, *got.HealAccounting.Clean)
	}
}

func TestReportTellsApprovalRefusalsFromApplyRefusals(t *testing.T) {
	approval := mustRA(t, raOpts{name: "a", phase: "Failed", created: time.Hour, gate: approvalGateRefused, conds: []string{
		cond("Applied", "False", "GateRefusedAtApproval", time.Hour), cond("GateRefused", "True", "BlastRadius", time.Hour)}})
	apply := mustRA(t, raOpts{name: "b", phase: "Failed", created: time.Hour, gate: approvalGatePass, conds: []string{
		cond("Applied", "False", "PreconditionRejected", time.Hour), cond("GateRefused", "True", "ObserveMode", time.Hour)}})
	noVerdict := mustRA(t, raOpts{name: "c", phase: "Failed", created: time.Hour, conds: []string{
		cond("Applied", "False", "GateRefusedAtApproval", time.Hour), cond("GateRefused", "True", "NoPlan", time.Hour)}})

	rows := Build(Inputs{Remediations: []dorguv1.RemediationAction{approval, apply, noVerdict}}, testNow, fixtureWindow).
		TierQ.Q6Refusals.Rows

	got := map[string]bool{}
	for _, r := range rows {
		got[r.Phase+"/"+r.Stage+"/"+r.Reason] = true
	}
	assert.Equal(t, map[string]bool{
		"approval/Guardrails/BlastRadius": true,
		"apply/Ticket/ObserveMode":        true,
		"approval/Unknown/NoPlan":         true,
	}, got, "the stage is the refusing verdict's; with none recorded it is Unknown, never invented")
}

func TestSplitProposalRefusal(t *testing.T) {
	stage, reason := splitProposalRefusal("DedupCooldown_RejectionCooldown")
	assert.Equal(t, "DedupCooldown", stage)
	assert.Equal(t, "RejectionCooldown", reason)

	stage, reason = splitProposalRefusal("Ground_Workload_Gone")
	assert.Equal(t, "Ground", stage, "split on the first underscore only")
	assert.Equal(t, "Workload_Gone", reason)

	for _, bad := range []string{"NoStage", "_Reason", "Stage_", ""} {
		stage, reason = splitProposalRefusal(bad)
		assert.Equal(t, StageUnknown, stage, bad)
		assert.Equal(t, bad, reason, bad)
	}
}

func TestReportKeepsARejectionReasonItDoesNotKnow(t *testing.T) {
	ra := mustRA(t, raOpts{name: "x", phase: "Rejected", created: time.Hour, conds: []string{
		cond("RejectionReason", "True", "FromANewerCLI", time.Hour)}})
	got := Build(Inputs{Remediations: []dorguv1.RemediationAction{ra}}, testNow, fixtureWindow)

	assert.Equal(t, 1, got.TierQ.Q2Precision.RejectionsByReason["unrecognised:FromANewerCLI"])
	assert.Equal(t, 0, got.TierQ.Q2Precision.Wrong, "an unknown reason is not assumed to mean wrong")
}

// An approved-then-rejected RemediationAction is one rejection, not an approval too.
func TestReportCountsARejectionAfterApprovalOnce(t *testing.T) {
	ra := mustRA(t, raOpts{name: "x", phase: "Rejected", created: 2 * time.Hour, approvedAt: time.Hour})
	got := Build(Inputs{Remediations: []dorguv1.RemediationAction{ra}}, testNow, fixtureWindow).TierA.A6Approval

	assert.Equal(t, 0, got.Approved)
	assert.Equal(t, 1, got.Rejected)
	assert.True(t, got.TooFewDecisions, "one decision is a count, not a band")
	assert.Empty(t, got.Band)
	assert.Nil(t, got.Rate)
}

func TestReportApprovalBands(t *testing.T) {
	mk := func(approved, rejected int) string {
		var ras []dorguv1.RemediationAction
		for i := 0; i < approved; i++ {
			ras = append(ras, mustRA(t, raOpts{name: "a", phase: "Pending", created: time.Hour, approvedAt: time.Minute}))
		}
		for i := 0; i < rejected; i++ {
			ras = append(ras, mustRA(t, raOpts{name: "r", phase: "Rejected", created: time.Hour}))
		}
		return Build(Inputs{Remediations: ras}, testNow, fixtureWindow).TierA.A6Approval.Band
	}
	assert.Equal(t, "below", mk(2, 8))
	assert.Equal(t, "in", mk(5, 5))
	assert.Equal(t, "above", mk(9, 1))
	assert.Equal(t, "in", mk(8, 2), "80% is the edge of the band, not over it")
	assert.Equal(t, "", mk(4, 0), "four decisions are too few to call a band")
	assert.Equal(t, "above", mk(5, 0), "five is the floor")
}

func TestReportEventOccurrencesAndTimes(t *testing.T) {
	series := mustEvent(t, `{"reason":"DorguCLINoTicket","eventTime":"`+micro(time.Hour)+`","series":{"count":4},"involvedObject":{"kind":"RemediationAction"}}`)
	counted := mustEvent(t, `{"reason":"DorguCLINoTicket","firstTimestamp":"`+ago(time.Hour)+`","count":2,"involvedObject":{"kind":"RemediationAction"}}`)
	bare := mustEvent(t, `{"reason":"DorguCLINoTicket","lastTimestamp":"`+ago(time.Hour)+`","involvedObject":{"kind":"RemediationAction"}}`)
	undated := mustEvent(t, `{"reason":"DorguCLINoTicket","involvedObject":{"kind":"RemediationAction"}}`)
	otherKind := mustEvent(t, `{"reason":"DorguCLINoTicket","lastTimestamp":"`+ago(time.Hour)+`","involvedObject":{"kind":"Pod"}}`)

	got := Build(Inputs{EventsRead: true, Events: []corev1.Event{series, counted, bare, undated, otherKind}},
		testNow, fixtureWindow).CLIDeclines

	assert.Equal(t, 4+2+1, got.ByReason["DorguCLINoTicket"], "series count, event count, a lone event; an undated event and another kind are not counted")
}

func TestReportFlagsRefusalsOnlyWithEnoughProposals(t *testing.T) {
	rows := map[refusalKey]int{{PhaseProposal, "ModePolicy", "SelfHealingMode"}: 4}
	assert.False(t, refusalMix(9, rows, 0).Rows[0].Flagged, "9 proposals is under the sample floor")
	assert.True(t, refusalMix(10, rows, 0).Rows[0].Flagged, "4 of 10 is over 30%")
	assert.False(t, refusalMix(20, rows, 0).Rows[0].Flagged, "4 of 20 is 20%")
}

func TestReportGateClampsAreReadFromStatusGate(t *testing.T) {
	gate := `{"phase":"ApprovalRecheck","result":"Clamp","verdicts":[{"stage":"Guardrails","guardrail":"blast-radius","verdict":"Clamp"}]}`
	ra := mustRA(t, raOpts{name: "x", phase: "Completed", created: time.Hour, gate: gate})
	got := Build(Inputs{Remediations: []dorguv1.RemediationAction{ra}}, testNow, fixtureWindow)

	assert.Equal(t, map[string]int{"blast-radius": 1}, got.FirstRunClamps.GateClamps)
	assert.Equal(t, 0, got.FirstRunClamps.ProposalsWithClamp, "no step recorded a clamp at proposal")
}

func TestReportSkipsARemediationWithNoCreationTime(t *testing.T) {
	ra := mustRA(t, raOpts{name: "x", phase: "Completed", created: time.Hour})
	ra.CreationTimestamp = metav1.Time{}
	got := Build(Inputs{Remediations: []dorguv1.RemediationAction{ra}}, testNow, fixtureWindow)
	assert.Equal(t, 0, got.HealAccounting.Total, "it cannot be placed in the window")
}
