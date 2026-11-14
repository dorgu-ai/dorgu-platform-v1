package fixtures

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/view"
)

var now = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func populated(s Scenario) *store.Store {
	st := store.New(nil)
	Populate(st, s, now)
	return st
}

func TestParse(t *testing.T) {
	for _, s := range Scenarios {
		got, err := Parse(string(s))
		require.NoError(t, err)
		assert.Equal(t, s, got)
	}
	_, err := Parse("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "valid scenarios")
}

func TestFullHasEveryShapeTheViewsTellApart(t *testing.T) {
	st := populated(Full)

	assert.True(t, st.SyncedAll())
	assert.Len(t, st.Remediations(), 9)
	assert.Len(t, st.Incidents(), 8)
	assert.Len(t, st.AppPersonas(), 3)
	assert.Len(t, st.ActionEvents(), 2)
	assert.Len(t, st.Nodes(), 2)

	phases := map[string]int{}
	for _, r := range st.Remediations() {
		phases[r.Status.Phase]++
	}
	for _, phase := range []string{"Pending", "Ticketed", "Completed", "Failed", "Expired", "Rejected"} {
		assert.Positive(t, phases[phase], phase)
	}

	payload := view.BuildRemediations(view.RemediationsInput{
		Remediations: st.Remediations(), Personas: st.AppPersonas(), Incidents: st.Incidents(),
		Readiness: view.Readiness{Synced: true, CRDsInstalled: true},
	})
	states := map[string]bool{}
	for _, r := range payload.Remediations {
		if r.Ticket != nil {
			states[r.Ticket.State] = true
		}
	}
	assert.True(t, states[view.TicketIssued] || states[view.TicketExpired], "a ticket that has not been consumed")
	assert.True(t, states[view.TicketConsumed])
}

func TestEverySettledScenarioHasWorkloadsAndNodes(t *testing.T) {
	for _, s := range []Scenario{Full, CRDsMissing, OperatorIdle, None, Refused} {
		st := populated(s)
		assert.NotEmpty(t, st.Deployments(), s)
		assert.NotEmpty(t, st.Nodes(), s)
	}
}

func TestWarmingHasNothingSyncedAndNoObjects(t *testing.T) {
	st := populated(Warming)
	assert.False(t, st.SyncedAll())
	assert.Empty(t, st.Deployments())
}

func TestCRDsMissingServesNoDorguResource(t *testing.T) {
	st := populated(CRDsMissing)
	for _, r := range kube.DorguResources {
		assert.False(t, st.CRDPresent(r), r)
	}
	assert.Empty(t, st.Incidents())
}

func TestRefusedNamesTheResourcesItCouldNotRead(t *testing.T) {
	st := populated(Refused)
	assert.NotEmpty(t, st.SyncFailure(kube.ResourceRemediationActions))
	assert.NotEmpty(t, st.SyncFailure(kube.ResourceIncidentMemories))
	assert.Empty(t, st.SyncFailure(kube.ResourceApplicationPersonas))
}

func TestOperatorIdleHasAnOperatorWithNoReadyReplica(t *testing.T) {
	st := populated(OperatorIdle)
	op := view.OperatorStatusOf(st.Deployments())
	assert.True(t, op.Found)
	assert.False(t, op.Running())
	assert.Empty(t, st.Remediations())
}

func TestNoneHasARunningOperatorThatRecordedNothing(t *testing.T) {
	st := populated(None)
	assert.True(t, view.OperatorStatusOf(st.Deployments()).Running())
	assert.Empty(t, st.Incidents())
	assert.Empty(t, st.Remediations())
	assert.NotEmpty(t, st.AppPersonas())
}

func TestTimesAreRelativeToTheInstantGiven(t *testing.T) {
	later := store.New(nil)
	Populate(later, Full, now.Add(240*time.Hour))
	for _, r := range later.Remediations() {
		assert.True(t, r.CreationTimestamp.Time.After(now.Add(100*time.Hour)), r.Name)
	}
}
