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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

var overviewNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func readable() InputRead { return InputRead{Present: true, Synced: true} }

func allReadable() OverviewReads {
	return OverviewReads{
		Remediations: readable(), Incidents: readable(), Personas: readable(),
		ClusterPersonas: readable(), Events: readable(),
	}
}

func overviewInput(mutate ...func(*OverviewInput)) OverviewInput {
	in := OverviewInput{
		Now: overviewNow, Since: DefaultOverviewWindow, Context: "kind-demo",
		Reads: allReadable(), Readiness: Readiness{Synced: true, CRDsInstalled: true},
	}
	for _, m := range mutate {
		m(&in)
	}
	return in
}

func completedAction(name string, age time.Duration) *dorguv1.RemediationAction {
	approved := metav1.NewTime(overviewNow.Add(-age + time.Minute))
	return &dorguv1.RemediationAction{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "shop", Name: name, CreationTimestamp: metav1.NewTime(overviewNow.Add(-age)),
		},
		Spec: dorguv1.RemediationActionSpec{IncidentRef: dorguv1.IncidentReference{Name: "inc", Namespace: "shop"}},
		Status: dorguv1.RemediationActionStatus{
			Phase: "Completed", ApprovedAt: &approved,
			Conditions: []metav1.Condition{{
				Type: "HealCredited", Status: metav1.ConditionTrue, Reason: "WorkloadPatchedByTicket",
				LastTransitionTime: approved,
			}},
			Gate: &dorguv1.GateStatus{Phase: "ApprovalRecheck", Result: "Pass"},
		},
	}
}

func operatorDeployment(image string, ready int32) *appsv1.Deployment {
	one := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "dorgu-system", Name: "dorgu-operator",
			Labels: map[string]string{OperatorLabel: OperatorLabelValue},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: image}}}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: ready},
	}
}

func TestOverviewComputesTheReportFromTheCache(t *testing.T) {
	inc := &dorguv1.IncidentMemory{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "inc", CreationTimestamp: metav1.NewTime(overviewNow.Add(-3 * time.Hour))},
		Spec: dorguv1.IncidentMemorySpec{
			Detection:  dorguv1.DetectionInfo{FirstSeen: metav1.NewTime(overviewNow.Add(-3 * time.Hour))},
			Resolution: &dorguv1.ResolutionInfo{Outcome: "resolved"},
		},
	}
	cp := &dorguv1.ClusterPersona{Spec: dorguv1.ClusterPersonaSpec{Policies: &dorguv1.ClusterPolicies{
		SelfHealing: &dorguv1.SelfHealingPolicy{Mode: "observe"},
	}}}
	bare := &dorguv1.ClusterPersona{}

	got := BuildOverview(overviewInput(func(in *OverviewInput) {
		in.Remediations = []*dorguv1.RemediationAction{completedAction("a", 2*time.Hour)}
		in.Incidents = []*dorguv1.IncidentMemory{inc}
		in.Deployments = []*appsv1.Deployment{operatorDeployment("ghcr.io/dorgu-ai/dorgu-operator:v0.13.1", 1)}
		in.AppPersonas = []*dorguv1.ApplicationPersona{{}, {}}
		in.ClusterPersonas = []*dorguv1.ClusterPersona{cp, bare}
	}))

	require.NotNil(t, got.Report)
	d := got.Report
	assert.Equal(t, "7d", got.Window)
	assert.Equal(t, 1, d.HealAccounting.Total)
	require.NotNil(t, d.HealAccounting.SafeHeals)
	assert.Equal(t, 1, *d.HealAccounting.SafeHeals)
	assert.Equal(t, "kind-demo", d.Cluster.Context)
	assert.Equal(t, "all", d.Cluster.Namespace)
	require.NotNil(t, d.Cluster.OperatorVersion)
	assert.Equal(t, "v0.13.1", *d.Cluster.OperatorVersion)
	require.NotNil(t, d.Cluster.Personas)
	assert.Equal(t, 2, *d.Cluster.Personas)
	assert.Equal(t, []string{"observe", "propose"}, d.Cluster.SelfHealing, "an unset mode is the default, propose")
	assert.True(t, d.Sources.RemediationActions.OK)
	assert.Equal(t, 1, d.Sources.Incidents.Count)
	assert.Empty(t, d.InputErrors)
	assert.False(t, d.CLIDeclines.NoData, "the Events were read, so an empty list is a measured zero")
	assert.True(t, got.Operator.Running())
}

func TestOverviewHasNoReportWithoutRemediationActions(t *testing.T) {
	cases := map[string]InputRead{
		"not served":   {Present: false, Synced: true},
		"still warm":   {Present: true, Synced: false},
		"read refused": {Present: true, Synced: true, Failure: "forbidden"},
	}
	for name, read := range cases {
		t.Run(name, func(t *testing.T) {
			got := BuildOverview(overviewInput(func(in *OverviewInput) { in.Reads.Remediations = read }))
			assert.Nil(t, got.Report, "no report is a different statement from a report of zero")
			assert.Equal(t, "7d", got.Window)
		})
	}
}

// An input that could not be read degrades its own section and names itself;
// it never becomes a zero.
func TestOverviewNamesWhatItCouldNotRead(t *testing.T) {
	got := BuildOverview(overviewInput(func(in *OverviewInput) {
		in.Remediations = []*dorguv1.RemediationAction{completedAction("a", 2*time.Hour)}
		in.Reads.Events = InputRead{Present: true, Synced: true, Failure: "the list timed out"}
		in.Reads.Incidents = InputRead{Present: false, Synced: true}
		in.Reads.Personas = InputRead{Present: true, Synced: true, Failure: "no"}
		in.Reads.ClusterPersonas = InputRead{Present: false, Synced: true}
		in.Namespace = "shop"
	}))

	require.NotNil(t, got.Report)
	d := got.Report
	assert.True(t, d.CLIDeclines.NoData, "unread Events are no data, not 0 declines")
	assert.True(t, d.HealAccounting.NoData)
	assert.Nil(t, d.HealAccounting.SafeHeals)
	assert.Nil(t, d.Cluster.Personas, "unknown, not 0")
	assert.Empty(t, d.Cluster.SelfHealing)
	assert.Equal(t, "shop", d.Cluster.Namespace)
	names := []string{}
	for _, e := range d.InputErrors {
		names = append(names, e.Input+":"+e.Error)
	}
	assert.Equal(t, []string{"incidents:not_found", "events:timeout", "personas:timeout", "cluster_personas:not_found"}, names)
	assert.False(t, d.Sources.Events.OK)
	assert.NotEmpty(t, d.Sources.Events.Error)
}

func TestOperatorStatus(t *testing.T) {
	assert.Equal(t, OperatorStatus{}, OperatorStatusOf(nil))
	assert.False(t, OperatorStatusOf(nil).Running())

	idle := OperatorStatusOf([]*appsv1.Deployment{operatorDeployment("dorgu-operator:v0.13.1", 0)})
	assert.True(t, idle.Found)
	assert.False(t, idle.Running(), "installed but no ready replica is not running")

	unlabelled := operatorDeployment("x:v1", 1)
	unlabelled.Labels = nil
	assert.False(t, OperatorStatusOf([]*appsv1.Deployment{unlabelled}).Found)

	digest := OperatorStatusOf([]*appsv1.Deployment{operatorDeployment("dorgu-operator@sha256:abc", 1)})
	assert.Nil(t, digest.Version, "a digest has no tag to report")
	noTag := OperatorStatusOf([]*appsv1.Deployment{operatorDeployment("registry:5000/dorgu-operator", 1)})
	assert.Nil(t, noTag.Version, "a registry port is not a tag")

	noReplicas := operatorDeployment("x:v1", 1)
	noReplicas.Spec.Replicas = nil
	assert.Equal(t, int32(1), OperatorStatusOf([]*appsv1.Deployment{noReplicas}).Desired)
}

func memoryIncident(name, phase string, last time.Time, conds ...metav1.Condition) *dorguv1.IncidentMemory {
	return &dorguv1.IncidentMemory{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: name},
		Spec: dorguv1.IncidentMemorySpec{
			Severity: "critical", Detection: dorguv1.DetectionInfo{Signal: "OOMKilled", LastSeen: metav1.NewTime(last)},
		},
		Status: dorguv1.IncidentMemoryStatus{Phase: phase, Conditions: conds},
	}
}

func TestMemoryKeepsTheLiveRefusalAndTheDurableRecordApart(t *testing.T) {
	at := metav1.NewTime(overviewNow)
	resolved := memoryIncident("old", PhaseResolved, overviewNow.Add(-time.Hour),
		metav1.Condition{Type: dorguv1.ConditionProposalRefused, Status: metav1.ConditionFalse, Reason: "IncidentResolved", LastTransitionTime: at},
		metav1.Condition{Type: dorguv1.ConditionProposalRefusalRecorded, Status: metav1.ConditionTrue, Reason: "Validate_StepCommand", Message: "step 2 command is not read-only", LastTransitionTime: at})
	resolved.Spec.RootCause = &dorguv1.RootCauseInfo{Summary: "limit too low", Confidence: "0.8", Provider: "rule-based"}
	resolved.Spec.Resolution = &dorguv1.ResolutionInfo{Action: "patched", Outcome: "resolved", Duration: &metav1.Duration{Duration: 90 * time.Second}}
	open := memoryIncident("new", PhaseDetected, overviewNow)

	got := BuildMemory(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{resolved, open},
		Readiness: Readiness{Synced: true, CRDsInstalled: true},
	})

	require.Len(t, got.Memories, 2)
	assert.Equal(t, "shop/new", got.Memories[0].ID, "newest first")
	assert.Equal(t, MemorySummary{Total: 2, Resolved: 1, Open: 1, Refused: 1, WithRootCause: 1}, got.Summary)

	rec := got.Memories[1]
	require.Len(t, rec.Conditions, 2)
	assert.Equal(t, "ProposalRefused", rec.Conditions[0].Type)
	assert.Equal(t, "False", rec.Conditions[0].Status)
	assert.Equal(t, "ProposalRefusalRecorded", rec.Conditions[1].Type)
	assert.Equal(t, "step 2 command is not read-only", rec.Conditions[1].Message)
	require.NotNil(t, rec.RefusalRecord)
	assert.Equal(t, "Validate", rec.RefusalRecord.Stage)
	assert.Equal(t, "1m30s", rec.ResolutionDuration)
	assert.Empty(t, got.Memories[0].Conditions)
	assert.NotNil(t, got.Memories[0].Conditions, "an empty list, not null")
}

func TestMemoryScopesToANamespaceAndReportsTruncation(t *testing.T) {
	other := memoryIncident("x", PhaseDetected, overviewNow)
	other.Namespace = "elsewhere"
	got := BuildMemory(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{
			memoryIncident("a", PhaseDetected, overviewNow), memoryIncident("b", PhaseDetected, overviewNow.Add(-time.Hour)), other,
		},
		Namespace: "shop", Limit: 1,
	})
	require.Len(t, got.Memories, 1)
	assert.Equal(t, "shop/a", got.Memories[0].ID)
	require.NotNil(t, got.Truncation)
	assert.Equal(t, TruncationNotice{Shown: 1, Total: 2, Limit: 1}, *got.Truncation)

	tie := BuildMemory(IncidentsInput{Incidents: []*dorguv1.IncidentMemory{
		memoryIncident("b", PhaseDetected, overviewNow), memoryIncident("a", PhaseDetected, overviewNow),
	}})
	assert.Equal(t, "shop/a", tie.Memories[0].ID, "equal times break by id so the order is stable")
}
