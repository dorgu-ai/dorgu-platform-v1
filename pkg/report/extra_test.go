package report

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWindowNamesTheCallersFlag(t *testing.T) {
	_, err := ParseWindow("--since", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--since is empty")

	_, err = ParseWindow("since", "7x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid since "7x"`)

	_, err = ParseWindow("since", "-2d")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")

	_, err = ParseWindow("since", "NaNd")
	require.Error(t, err)

	d, err := ParseWindow("since", " 2w ")
	require.NoError(t, err)
	assert.Equal(t, 14*24*time.Hour, d)
}

func TestVersionHelpers(t *testing.T) {
	cases := []struct {
		tag             string
		known, predates bool
	}{
		{"v0.13.0", true, false}, {"v0.12.9", true, true}, {"v0.13.1-rc1", true, false},
		{"v1.0.0", true, false}, {"v0.9.0", true, true}, {"latest", false, false}, {"", false, false},
		{"v0.13", false, false}, {"v0.x.1", false, false}, {"v0.-1.0", false, false},
	}
	for _, tc := range cases {
		tag := tc.tag
		assert.Equal(t, tc.known, OperatorVersionKnown(&tag), tc.tag)
		assert.Equal(t, tc.predates, OperatorPredatesTickets(&tag), tc.tag)
	}
	assert.False(t, OperatorVersionKnown(nil))
	assert.False(t, OperatorPredatesTickets(nil), "an unknown version is never marked")
}

func TestRejectionFlagsAreTheEnum(t *testing.T) {
	assert.Equal(t, []string{"wrong-diagnosis", "too-risky", "not-now", "duplicate", "not-needed", "other"}, RejectionFlags())
	assert.Equal(t, []string{"a", "b"}, SortedKeys(map[string]int{"b": 1, "a": 2}))
}

// Without a version the records decide: a Completed action with no HealCredited
// record means the heals cannot be judged, and the north star is no data.
func TestUnknownOperatorVersionFailsClosedOnTheRecords(t *testing.T) {
	bare := mustRA(t, raOpts{name: "x", phase: "Completed", created: time.Hour, incident: "i"})
	got := Build(Inputs{Remediations: []remActionForTest{bare}}, testNow, fixtureWindow)
	assert.True(t, got.HealAccounting.NoData)
	assert.Nil(t, got.HealAccounting.Clean, "no data is nil, never 0")
	assert.Nil(t, got.HealAccounting.SafeHeals)
	assert.Contains(t, got.HealAccounting.Reason, "could not be determined")
	assert.Equal(t, 1, got.HealAccounting.Completed)

	credited := mustRA(t, raOpts{name: "y", phase: "Completed", created: time.Hour, incident: "i",
		conds: []string{cond("HealCredited", "True", "WorkloadPatchedByTicket", time.Minute)}})
	got = Build(Inputs{Remediations: []remActionForTest{bare, credited}}, testNow, fixtureWindow)
	assert.False(t, got.HealAccounting.NoData, "one HealCredited record proves the operator writes them")

	old := Build(Inputs{Remediations: []remActionForTest{credited}, OperatorVersion: strp("v0.12.0")}, testNow, fixtureWindow)
	assert.True(t, old.HealAccounting.NoData)
	assert.Contains(t, old.HealAccounting.Reason, "predates v0.13.0")
}

func TestUnreadableIncidentsBlankEverythingTheyCarry(t *testing.T) {
	ra := mustRA(t, raOpts{name: "x", phase: "Completed", created: time.Hour, incident: "i"})
	got := Build(Inputs{
		Remediations: []remActionForTest{ra},
		Errors:       []InputError{{Input: "incidents", Error: "forbidden"}},
	}, testNow, fixtureWindow)

	assert.True(t, got.HealAccounting.NoData)
	assert.Nil(t, got.HealAccounting.SafeHeals)
	assert.True(t, got.TierQ.Q6Refusals.NoData)
	assert.True(t, got.TierQ.Q1TimeToHeal.DetectToProposal.NoData)
	assert.Contains(t, got.TierQ.Q1TimeToHeal.DetectToProposal.Reason, "IncidentMemories could not be read")
	assert.Equal(t, []InputError{{Input: "incidents", Error: "forbidden"}}, got.InputErrors)
}

// An action approved with --no-heal is never patched; the rollback that follows
// is not a heal that was undone.
func TestRollbackOfAnApprovalThatNeverHealedIsNotARollback(t *testing.T) {
	notHealed := mustRA(t, raOpts{name: "a", phase: "RolledBack", created: time.Hour, conds: []string{
		cond("WorkloadDiverged", "False", "WorkloadRestored", time.Minute)}})
	patched := mustRA(t, raOpts{name: "b", phase: "RolledBack", created: time.Hour, conds: []string{
		cond("WorkloadDiverged", "False", "WorkloadRestored", time.Minute),
		cond("WorkloadPatched", "True", "Patched", time.Minute)}})
	got := Build(Inputs{Remediations: []remActionForTest{notHealed, patched}, OperatorVersion: strp("v0.13.1")}, testNow, fixtureWindow)

	assert.Equal(t, 1, got.HealAccounting.Excluded[ExcludedApprovedNotHealed])
	assert.Equal(t, 1, got.HealAccounting.Excluded[ExcludedRolledBack])
}

// A standing Guardrails/Concurrent refusal is the incident's own proposal while
// that proposal is in flight, or once the incident resolved; otherwise another
// incident holds the persona and it stays a refusal.
func TestConcurrentRefusalIsADeferralOnlyForTheIncidentsOwnFix(t *testing.T) {
	h := time.Hour
	refused := func(name string) string {
		return incidentJSON(name, 5*h, 5*h, "", cond("ProposalRefused", "True", "Guardrails_Concurrent", h))
	}
	inFlight := Build(Inputs{
		Incidents:    []incidentForTest{mustIncident(t, refused("i1"))},
		Remediations: []remActionForTest{mustRA(t, raOpts{name: "r", phase: "Pending", created: 2 * h, incident: "i1"})},
	}, testNow, fixtureWindow).TierQ.Q6Refusals
	assert.Empty(t, inFlight.Rows)
	require.NotNil(t, inFlight.Deferred)
	assert.Equal(t, 1, *inFlight.Deferred)

	finished := Build(Inputs{
		Incidents:    []incidentForTest{mustIncident(t, refused("i1"))},
		Remediations: []remActionForTest{mustRA(t, raOpts{name: "r", phase: "Completed", created: 2 * h, incident: "i1"})},
	}, testNow, fixtureWindow).TierQ.Q6Refusals
	require.Len(t, finished.Rows, 1, "a finished action of its own does not make a real refusal a deferral")
	assert.Equal(t, "Concurrent", finished.Rows[0].Reason)

	otherHolds := Build(Inputs{Incidents: []incidentForTest{mustIncident(t, refused("i1"))}}, testNow, fixtureWindow).TierQ.Q6Refusals
	require.Len(t, otherHolds.Rows, 1)
}

// The record and the live condition can differ in date on an operator before
// v0.13.1; the later one is when the incident was last refused.
func TestRefusalIsDatedByTheLaterOfRecordAndLiveCondition(t *testing.T) {
	d := 24 * time.Hour
	inc := mustIncident(t, incidentJSON("i", 20*d, 20*d, "",
		cond("ProposalRefused", "True", "ModePolicy_SelfHealingMode", 2*d),
		cond("ProposalRefusalRecorded", "True", "ModePolicy_SelfHealingMode", 10*d)))
	got := Build(Inputs{Incidents: []incidentForTest{inc}}, testNow, fixtureWindow).TierQ.Q6Refusals
	require.Len(t, got.Rows, 1, "the live condition is inside the window even though the record is not")
}

func TestRefusalWithNoConditionTimeFallsBackToTheIncidentCreation(t *testing.T) {
	raw := `{"metadata":{"name":"i","namespace":"shop","creationTimestamp":"` + ago(time.Hour) + `"},` +
		`"spec":{"detection":{}},"status":{"conditions":[{"type":"ProposalRefused","status":"True","reason":"Ground_WorkloadGone"}]}}`
	got := Build(Inputs{Incidents: []incidentForTest{mustIncident(t, raw)}}, testNow, fixtureWindow).TierQ.Q6Refusals
	require.Len(t, got.Rows, 1)
}

func TestPlanSourcesDefaultToUnspecified(t *testing.T) {
	ra := mustRA(t, raOpts{name: "x", phase: "Pending", created: time.Hour})
	ra.Spec.PlanSource = ""
	got := Build(Inputs{Remediations: []remActionForTest{ra}}, testNow, fixtureWindow).TierQ.Q3LLM
	assert.Equal(t, map[string]int{"unspecified": 1}, got.PlanSources)
	assert.True(t, got.NoData)
}

func TestEventsReadButEmptyIsAMeasuredZero(t *testing.T) {
	got := Build(Inputs{EventsRead: true}, testNow, fixtureWindow).CLIDeclines
	assert.False(t, got.NoData)
	assert.Empty(t, got.ByReason)
}

func TestAFoldedEventStartedBeforeTheWindowCountsOnce(t *testing.T) {
	ev := mustEvent(t, `{"reason":"DorguCLINoTicket","firstTimestamp":"`+ago(30*24*time.Hour)+`","lastTimestamp":"`+ago(time.Hour)+`","count":50,"involvedObject":{"kind":"RemediationAction"}}`)
	got := Build(Inputs{EventsRead: true, Events: []eventForTest{ev}}, testNow, fixtureWindow).CLIDeclines
	assert.Equal(t, 1, got.ByReason["DorguCLINoTicket"])
}

func TestAnIncidentOutsideTheRAsNamespaceIsFoundByItsRef(t *testing.T) {
	ra := mustRA(t, raOpts{name: "x", phase: "Completed", created: time.Hour, incident: "i",
		conds: []string{cond("HealCredited", "True", "WorkloadPatchedByTicket", time.Minute)}})
	ra.Spec.IncidentRef.Namespace = ""
	inc := mustIncident(t, incidentJSON("i", 2*time.Hour, 2*time.Hour, "resolved"))
	got := Build(Inputs{Remediations: []remActionForTest{ra}, Incidents: []incidentForTest{inc}, OperatorVersion: strp("v0.13.1")}, testNow, fixtureWindow)
	assert.Equal(t, 1, *got.HealAccounting.Clean, "an empty incidentRef namespace means the action's own")
}
