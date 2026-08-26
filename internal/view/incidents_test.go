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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

var baseTime = time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC)

func incident(
	namespace, name, personaName, severity, phase string,
	mutate ...func(*dorguv1.IncidentMemory),
) *dorguv1.IncidentMemory {
	i := &dorguv1.IncidentMemory{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         namespace,
			Name:              name,
			CreationTimestamp: metav1.NewTime(baseTime),
		},
		Spec: dorguv1.IncidentMemorySpec{
			PersonaRef: dorguv1.PersonaReference{
				Kind: "ApplicationPersona", Name: personaName, Namespace: namespace,
			},
			Category: "resource",
			Severity: severity,
			Detection: dorguv1.DetectionInfo{
				Signal:    "OOMKilled",
				Source:    "pod-failure-detector",
				FirstSeen: metav1.NewTime(baseTime),
				LastSeen:  metav1.NewTime(baseTime),
				AffectedResources: []dorguv1.ResourceReference{
					{Kind: "Pod", Name: personaName + "-abc", Namespace: namespace, Role: "affected"},
				},
			},
		},
		Status: dorguv1.IncidentMemoryStatus{Phase: phase},
	}
	for _, m := range mutate {
		m(i)
	}
	return i
}

func seenAt(offset time.Duration) func(*dorguv1.IncidentMemory) {
	return func(i *dorguv1.IncidentMemory) {
		i.Spec.Detection.LastSeen = metav1.NewTime(baseTime.Add(offset))
	}
}

func TestIncidentProjectionCarriesEveryColumnTheViewShows(t *testing.T) {
	i := incident("apps", "checkout-oom", "checkout", SeverityCritical, PhaseDetected,
		func(i *dorguv1.IncidentMemory) {
			i.Spec.RootCause = &dorguv1.RootCauseInfo{
				Summary:    "Memory limit of 128Mi is below the working set",
				Confidence: "0.85",
				Provider:   "ai-enhanced",
				Contributing: []dorguv1.ContributingSignal{
					{Signal: "OOMKilled", Detail: "3 restarts in 10 minutes"},
				},
			}
			i.Status.OccurrenceCount = 3
		})

	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{i},
		Personas:  []*dorguv1.ApplicationPersona{persona("apps", "checkout", "checkout")},
	})

	require.Len(t, payload.Incidents, 1)
	row := payload.Incidents[0]
	assert.Equal(t, "apps/checkout-oom", row.ID)
	assert.Equal(t, SeverityCritical, row.Severity)
	assert.Equal(t, "OOMKilled", row.Signal)
	assert.Equal(t, "pod-failure-detector", row.Source)
	assert.Equal(t, "resource", row.Category)
	assert.Equal(t, PhaseDetected, row.Phase)
	assert.Equal(t, int32(3), row.OccurrenceCount)
	assert.Equal(t, "checkout", row.Persona.Name)
	assert.True(t, row.Persona.Exists)
	require.Len(t, row.AffectedResources, 1)
	assert.Equal(t, "checkout-abc", row.AffectedResources[0].Name)

	require.NotNil(t, row.RootCause)
	assert.Equal(t, "Memory limit of 128Mi is below the working set", row.RootCause.Summary)
	assert.Equal(t, "ai-enhanced", row.RootCause.Provider)
	assert.Equal(t, "0.85", row.RootCause.Confidence,
		"confidence is passed through as the string the diagnosis wrote, not reformatted")
	require.Len(t, row.RootCause.Contributing, 1)
}

// Detection runs for free and AI diagnosis is opt-in, so an undiagnosed
// incident is a common state rather than an error.
func TestAnUndiagnosedIncidentHasNoRootCause(t *testing.T) {
	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{
			incident("apps", "checkout-oom", "checkout", SeverityWarning, PhaseDetected),
		},
	})
	assert.Nil(t, payload.Incidents[0].RootCause)
	assert.Equal(t, 0, payload.Summary.Diagnosed)
}

// An unattributed incident names the workload the signals came from, and no
// persona of that name need exist. Marking it as existing would claim Dorgu can
// act on it, which it cannot.
func TestUnattributedIncidentReportsThatNoPersonaExists(t *testing.T) {
	i := incident("apps", "orphan-oom", "legacy-billing", SeverityCritical, PhaseDetected,
		func(i *dorguv1.IncidentMemory) {
			i.Spec.Attribution = AttributionUnattributed
		})

	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{i},
		Personas:  []*dorguv1.ApplicationPersona{persona("apps", "checkout", "checkout")},
	})

	row := payload.Incidents[0]
	assert.Equal(t, AttributionUnattributed, row.Attribution)
	assert.False(t, row.Persona.Exists)
	assert.Equal(t, 1, payload.Summary.Unattributed)
}

// The field is optional and older incidents predate it. Defaulting to persona
// matches the operator's reading and cannot invent a blind spot.
func TestEmptyAttributionDefaultsToPersona(t *testing.T) {
	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{
			incident("apps", "checkout-oom", "checkout", SeverityWarning, PhaseDetected),
		},
	})
	assert.Equal(t, "persona", payload.Incidents[0].Attribution)
	assert.Equal(t, 0, payload.Summary.Unattributed)
}

func TestFeedIsNewestFirstByLastSeen(t *testing.T) {
	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{
			incident("apps", "old", "checkout", SeverityCritical, PhaseDetected, seenAt(-time.Hour)),
			incident("apps", "new", "checkout", SeverityInfo, PhaseDetected, seenAt(time.Minute)),
			incident("apps", "middle", "checkout", SeverityWarning, PhaseDetected, seenAt(0)),
		},
	})

	got := []string{payload.Incidents[0].Name, payload.Incidents[1].Name, payload.Incidents[2].Name}
	assert.Equal(t, []string{"new", "middle", "old"}, got,
		"severity must not override recency: this is a feed")
}

func TestOrderingIsStableForIdenticalTimestamps(t *testing.T) {
	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{
			incident("staging", "b", "checkout", SeverityInfo, PhaseDetected),
			incident("apps", "b", "checkout", SeverityInfo, PhaseDetected),
			incident("apps", "a", "checkout", SeverityInfo, PhaseDetected),
		},
	})

	got := []string{
		payload.Incidents[0].ID, payload.Incidents[1].ID, payload.Incidents[2].ID,
	}
	assert.Equal(t, []string{"apps/a", "apps/b", "staging/b"}, got)
}

func TestSummaryCountsSeverityAndOpenState(t *testing.T) {
	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{
			incident("apps", "a", "checkout", SeverityCritical, PhaseDetected),
			incident("apps", "b", "checkout", SeverityWarning, PhaseInvestigating),
			incident("apps", "c", "checkout", SeverityInfo, PhaseResolved),
			incident("apps", "d", "checkout", SeverityCritical, PhaseRecurring),
		},
	})

	assert.Equal(t, 4, payload.Summary.Total)
	assert.Equal(t, 3, payload.Summary.Open)
	assert.Equal(t, 2, payload.Summary.Critical)
	assert.Equal(t, 1, payload.Summary.Warning)
	assert.Equal(t, 1, payload.Summary.Info)
}

// An incident the operator has not reconciled has no phase. Treating a missing
// field as "fine" is the failure mode the clean-room run named.
func TestAnIncidentWithNoPhaseCountsAsOpen(t *testing.T) {
	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{incident("apps", "a", "checkout", SeverityWarning, "")},
	})
	assert.Equal(t, 1, payload.Summary.Open)
	assert.True(t, payload.Incidents[0].IsOpen())
}

func TestResolutionIsProjectedIncludingTheAcknowledgedOutcome(t *testing.T) {
	applied := metav1.NewTime(baseTime)
	i := incident("apps", "a", "checkout", SeverityCritical, PhaseResolved, func(i *dorguv1.IncidentMemory) {
		i.Spec.Resolution = &dorguv1.ResolutionInfo{
			Action:         "Increase memory limit to 256Mi",
			Outcome:        "acknowledged",
			AppliedAt:      &applied,
			RemediationRef: &dorguv1.RemediationReference{Name: "checkout-oom-fix", Namespace: "apps"},
		}
	})

	payload := BuildIncidents(IncidentsInput{Incidents: []*dorguv1.IncidentMemory{i}})

	res := payload.Incidents[0].Resolution
	require.NotNil(t, res)
	assert.Equal(t, "acknowledged", res.Outcome,
		"acknowledged means a human approved an advisory plan and nothing was applied")
	assert.Equal(t, "checkout-oom-fix", res.RemediationName)
	assert.Equal(t, "apps", res.RemediationNamespace)
}

// A capped feed must never read as a complete one.
func TestTruncationIsReportedNotSwallowed(t *testing.T) {
	var incidents []*dorguv1.IncidentMemory
	for n := range 10 {
		incidents = append(incidents, incident("apps", fmt.Sprintf("i%02d", n), "checkout",
			SeverityWarning, PhaseDetected, seenAt(time.Duration(n)*time.Minute)))
	}

	payload := BuildIncidents(IncidentsInput{Incidents: incidents, Limit: 3})

	require.Len(t, payload.Incidents, 3)
	require.NotNil(t, payload.Truncation)
	assert.Equal(t, 3, payload.Truncation.Shown)
	assert.Equal(t, 10, payload.Truncation.Total)
	assert.Equal(t, 3, payload.Truncation.Limit)
	assert.Equal(t, 10, payload.Summary.Total,
		"the summary counts everything, so the header does not lie about the cap")
	assert.Equal(t, "i09", payload.Incidents[0].Name, "the cap keeps the newest")
}

func TestNoTruncationNoticeWhenUnderTheLimit(t *testing.T) {
	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{incident("apps", "a", "checkout", SeverityInfo, PhaseDetected)},
		Limit:     10,
	})
	assert.Nil(t, payload.Truncation)
}

func TestNegativeLimitMeansNoCap(t *testing.T) {
	var incidents []*dorguv1.IncidentMemory
	for n := range DefaultIncidentLimit + 5 {
		incidents = append(incidents, incident("apps", fmt.Sprintf("i%04d", n), "checkout",
			SeverityInfo, PhaseDetected))
	}

	payload := BuildIncidents(IncidentsInput{Incidents: incidents, Limit: -1})
	assert.Len(t, payload.Incidents, DefaultIncidentLimit+5)
	assert.Nil(t, payload.Truncation)
}

func TestZeroLimitUsesTheDefault(t *testing.T) {
	var incidents []*dorguv1.IncidentMemory
	for n := range DefaultIncidentLimit + 1 {
		incidents = append(incidents, incident("apps", fmt.Sprintf("i%04d", n), "checkout",
			SeverityInfo, PhaseDetected))
	}

	payload := BuildIncidents(IncidentsInput{Incidents: incidents})
	assert.Len(t, payload.Incidents, DefaultIncidentLimit)
	require.NotNil(t, payload.Truncation)
	assert.Equal(t, DefaultIncidentLimit, payload.Truncation.Limit)
}

func TestNamespaceScopeFiltersTheFeed(t *testing.T) {
	payload := BuildIncidents(IncidentsInput{
		Incidents: []*dorguv1.IncidentMemory{
			incident("apps", "a", "checkout", SeverityInfo, PhaseDetected),
			incident("staging", "b", "checkout", SeverityInfo, PhaseDetected),
		},
		Namespace: "apps",
	})
	require.Len(t, payload.Incidents, 1)
	assert.Equal(t, "apps", payload.Incidents[0].Namespace)
}

func TestEmptyFeedEncodesAsAListNotNull(t *testing.T) {
	payload := BuildIncidents(IncidentsInput{})
	assert.NotNil(t, payload.Incidents)
	assert.Empty(t, payload.Incidents)
}

// A status LastOccurrence newer than detection LastSeen is the fresher fact and
// drives the app row's last-incident time.
func TestLastOccurrenceWinsOverLastSeen(t *testing.T) {
	later := metav1.NewTime(baseTime.Add(time.Hour))
	i := incident("apps", "a", "checkout", SeverityWarning, PhaseDetected, func(i *dorguv1.IncidentMemory) {
		i.Status.LastOccurrence = &later
	})

	index := indexIncidents([]*dorguv1.IncidentMemory{i})
	counts := index.forPersona(persona("apps", "checkout", "checkout"))
	require.NotNil(t, counts.LastIncidentTime)
	assert.True(t, counts.LastIncidentTime.Equal(&later))
}

func TestIndexFallsBackToThePersonaLastIncidentTime(t *testing.T) {
	reported := metav1.NewTime(baseTime)
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Status.LastIncidentTime = &reported
	})

	counts := indexIncidents(nil).forPersona(p)
	require.NotNil(t, counts.LastIncidentTime)
	assert.True(t, counts.LastIncidentTime.Equal(&reported))
	assert.Equal(t, 0, counts.Open)
}
