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
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// Incident phases the operator writes. Resolved is the only one that is closed.
const (
	PhaseDetected      = "Detected"
	PhaseInvestigating = "Investigating"
	PhaseResolved      = "Resolved"
	PhaseRecurring     = "Recurring"
)

// Incident severities, from the CRD enum.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// AttributionUnattributed marks an incident no persona claimed. It is a real
// outage Dorgu can see but cannot diagnose or remediate, and the view says so
// rather than folding it into a neighbouring app.
const AttributionUnattributed = "unattributed"

// DefaultIncidentLimit caps the feed. A cluster in a bad hour can produce
// thousands of IncidentMemories, and pushing all of them over SSE on every
// change would make the live update the slowest part of the product. The cap is
// reported in the payload so a truncated feed can never read as a complete one.
const DefaultIncidentLimit = 500

// IncidentsInput is everything the Incidents view is built from.
type IncidentsInput struct {
	Incidents []*dorguv1.IncidentMemory
	// Personas is used only to answer whether an incident's personaRef names a
	// persona that exists, which is what separates an attributed incident from
	// an unattributed one in the UI.
	Personas []*dorguv1.ApplicationPersona

	// Namespace scopes the feed. Empty means every namespace.
	Namespace string
	// Limit caps the rows returned. Zero means DefaultIncidentLimit; a
	// negative value means no cap.
	Limit int

	Readiness Readiness
}

// BuildIncidents projects IncidentMemories into the live incident feed.
//
// Ordering is newest-first by last-seen, because this is a feed and the thing
// that just broke is the thing being looked for. Severity deliberately does not
// override recency: a critical incident from yesterday is not more urgent than a
// warning from ten seconds ago, and the severity column plus the summary counts
// are how a reader filters.
func BuildIncidents(in IncidentsInput) IncidentsPayload {
	personaExists := personaNameSet(in.Personas)

	rows := make([]Incident, 0, len(in.Incidents))
	for _, incident := range in.Incidents {
		if in.Namespace != "" && incident.Namespace != in.Namespace {
			continue
		}
		rows = append(rows, projectIncident(incident, personaExists))
	}

	sortIncidents(rows)
	summary := summariseIncidents(rows)

	limit := in.Limit
	if limit == 0 {
		limit = DefaultIncidentLimit
	}
	var truncation *TruncationNotice
	if limit > 0 && len(rows) > limit {
		truncation = &TruncationNotice{Shown: limit, Total: len(rows), Limit: limit}
		rows = rows[:limit]
	}

	return IncidentsPayload{
		Incidents:  rows,
		Summary:    summary,
		Readiness:  in.Readiness,
		Truncation: truncation,
	}
}

func projectIncident(incident *dorguv1.IncidentMemory, personaExists map[string]bool) Incident {
	spec := incident.Spec
	row := Incident{
		ID:              incident.Namespace + "/" + incident.Name,
		Name:            incident.Name,
		Namespace:       incident.Namespace,
		Severity:        spec.Severity,
		Category:        spec.Category,
		Phase:           incident.Status.Phase,
		Attribution:     attributionOf(spec),
		Signal:          spec.Detection.Signal,
		Source:          spec.Detection.Source,
		FirstSeen:       spec.Detection.FirstSeen,
		LastSeen:        spec.Detection.LastSeen,
		OccurrenceCount: incident.Status.OccurrenceCount,
		LastOccurrence:  incident.Status.LastOccurrence,
		CreatedAt:       incident.CreationTimestamp,
		Persona: IncidentPersona{
			Kind:      spec.PersonaRef.Kind,
			Name:      spec.PersonaRef.Name,
			Namespace: spec.PersonaRef.Namespace,
			Exists:    personaExists[spec.PersonaRef.Namespace+"/"+spec.PersonaRef.Name],
		},
		AffectedResources: spec.Detection.AffectedResources,
	}

	if rc := spec.RootCause; rc != nil {
		row.RootCause = &IncidentRootCause{
			Summary:      rc.Summary,
			Confidence:   rc.Confidence,
			Provider:     rc.Provider,
			Contributing: rc.Contributing,
		}
	}
	if res := spec.Resolution; res != nil {
		row.Resolution = &IncidentResolution{
			Action:    res.Action,
			Outcome:   res.Outcome,
			AppliedAt: res.AppliedAt,
		}
		if ref := res.RemediationRef; ref != nil {
			row.Resolution.RemediationName = ref.Name
			row.Resolution.RemediationNamespace = ref.Namespace
		}
	}
	return row
}

// attributionOf defaults an empty attribution to "persona".
//
// The field is optional in the CRD and older incidents predate it. Defaulting
// to persona matches the operator's own reading and, unlike defaulting to
// unattributed, cannot invent a blind spot that does not exist.
func attributionOf(spec dorguv1.IncidentMemorySpec) string {
	if spec.Attribution == "" {
		return "persona"
	}
	return spec.Attribution
}

// IsOpen reports whether an incident still needs attention.
//
// Resolved is the only closed phase. An empty phase counts as open: an incident
// the operator has not yet reconciled is a detected problem, and treating a
// missing field as "fine" is the failure mode the clean-room run named.
func (i Incident) IsOpen() bool {
	return i.Phase != PhaseResolved
}

func sortIncidents(rows []Incident) {
	sort.SliceStable(rows, func(a, b int) bool {
		x, y := rows[a], rows[b]
		if !x.LastSeen.Equal(&y.LastSeen) {
			return x.LastSeen.After(y.LastSeen.Time)
		}
		if x.Namespace != y.Namespace {
			return x.Namespace < y.Namespace
		}
		return x.Name < y.Name
	})
}

func summariseIncidents(rows []Incident) IncidentsSummary {
	summary := IncidentsSummary{Total: len(rows)}
	for _, row := range rows {
		if row.IsOpen() {
			summary.Open++
		}
		switch row.Severity {
		case SeverityCritical:
			summary.Critical++
		case SeverityWarning:
			summary.Warning++
		case SeverityInfo:
			summary.Info++
		}
		if row.Attribution == AttributionUnattributed {
			summary.Unattributed++
		}
		if row.RootCause != nil {
			summary.Diagnosed++
		}
	}
	return summary
}

func personaNameSet(personas []*dorguv1.ApplicationPersona) map[string]bool {
	out := make(map[string]bool, len(personas))
	for _, p := range personas {
		out[p.Namespace+"/"+p.Name] = true
	}
	return out
}

// ---------------------------------------------------------------------------
// Per-app incident counts, used by the Apps view
// ---------------------------------------------------------------------------

// incidentIndexByPersona counts open incidents per persona, keyed by the
// namespace and metadata.name that IncidentMemory.spec.personaRef carries.
type incidentIndexByPersona map[string]AppIncidents

func indexIncidents(incidents []*dorguv1.IncidentMemory) incidentIndexByPersona {
	index := incidentIndexByPersona{}
	for _, incident := range incidents {
		if incident.Status.Phase == PhaseResolved {
			continue
		}
		ref := incident.Spec.PersonaRef
		key := ref.Namespace + "/" + ref.Name
		entry := index[key]
		entry.Open++
		if incident.Spec.Severity == SeverityCritical {
			entry.Critical++
		}
		if last := latestSeen(incident); last != nil {
			if entry.LastIncidentTime == nil || last.After(entry.LastIncidentTime.Time) {
				entry.LastIncidentTime = last
			}
		}
		index[key] = entry
	}
	return index
}

// forPersona returns the counts for one persona, folding in the operator's own
// activeIncidents figure without reconciling the two. A disagreement is a fact
// about the operator worth showing, not a rounding error to hide.
func (index incidentIndexByPersona) forPersona(persona *dorguv1.ApplicationPersona) AppIncidents {
	entry := index[persona.Namespace+"/"+persona.Name]
	entry.PersonaReported = persona.Status.ActiveIncidents
	if entry.LastIncidentTime == nil {
		entry.LastIncidentTime = persona.Status.LastIncidentTime
	}
	return entry
}

func latestSeen(incident *dorguv1.IncidentMemory) *metav1.Time {
	last := incident.Spec.Detection.LastSeen
	if occurrence := incident.Status.LastOccurrence; occurrence != nil && occurrence.After(last.Time) {
		return occurrence
	}
	if last.IsZero() {
		return nil
	}
	return &last
}
