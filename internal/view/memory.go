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

// DefaultMemoryLimit caps the Memory list, for the same reason the incident feed
// is capped. The cap is reported in the payload so a truncated list never reads
// as a complete one.
const DefaultMemoryLimit = 1000

// MemoryCondition is one status condition on an IncidentMemory.
//
// The conditions are the part of the record the Incidents feed folds into a
// verdict: ProposalRefused is the live state and is rewritten when the incident
// resolves, while ProposalRefusalRecorded is the operator's durable record of a
// refusal and is never cleared. The Memory view shows both side by side, because
// that difference is the reason the second one exists.
type MemoryCondition struct {
	Type               string      `json:"type"`
	Status             string      `json:"status"`
	Reason             string      `json:"reason,omitempty"`
	Message            string      `json:"message,omitempty"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitzero"`
}

// MemoryRecord is one IncidentMemory with everything the detail pane shows.
// The embedded Incident is the same projection the Incidents feed uses, so the
// two views cannot describe one incident differently.
type MemoryRecord struct {
	Incident
	Conditions []MemoryCondition `json:"conditions"`
	// ResolutionDuration is how long the incident took to resolve, as the
	// operator recorded it. Empty when it has not resolved.
	ResolutionDuration string `json:"resolutionDuration,omitempty"`
}

// MemorySummary is the count line above the Memory list.
type MemorySummary struct {
	Total    int `json:"total"`
	Resolved int `json:"resolved"`
	Open     int `json:"open"`
	// Refused is the number of incidents with a durable proposal-refusal record.
	Refused int `json:"refused"`
	// WithRootCause is the number that carry a diagnosis.
	WithRootCause int `json:"withRootCause"`
}

// MemoryPayload is the whole Memory view.
type MemoryPayload struct {
	Memories   []MemoryRecord    `json:"memories"`
	Summary    MemorySummary     `json:"summary"`
	Readiness  Readiness         `json:"readiness"`
	Truncation *TruncationNotice `json:"truncation,omitempty"`
}

// BuildMemory projects every IncidentMemory, open or closed, newest first.
func BuildMemory(in IncidentsInput) MemoryPayload {
	personaExists := personaNameSet(in.Personas)

	rows := make([]MemoryRecord, 0, len(in.Incidents))
	for _, incident := range in.Incidents {
		if in.Namespace != "" && incident.Namespace != in.Namespace {
			continue
		}
		rows = append(rows, projectMemory(incident, personaExists))
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].LastSeen.Equal(&rows[j].LastSeen) {
			return rows[i].LastSeen.After(rows[j].LastSeen.Time)
		}
		return rows[i].ID < rows[j].ID
	})

	summary := MemorySummary{Total: len(rows)}
	for _, r := range rows {
		if r.IsOpen() {
			summary.Open++
		} else {
			summary.Resolved++
		}
		if r.RefusalRecord != nil {
			summary.Refused++
		}
		if r.RootCause != nil {
			summary.WithRootCause++
		}
	}

	limit := in.Limit
	if limit == 0 {
		limit = DefaultMemoryLimit
	}
	var truncation *TruncationNotice
	if limit > 0 && len(rows) > limit {
		truncation = &TruncationNotice{Shown: limit, Total: len(rows), Limit: limit}
		rows = rows[:limit]
	}
	return MemoryPayload{Memories: rows, Summary: summary, Readiness: in.Readiness, Truncation: truncation}
}

func projectMemory(incident *dorguv1.IncidentMemory, personaExists map[string]bool) MemoryRecord {
	rec := MemoryRecord{
		Incident:   projectIncident(incident, personaExists),
		Conditions: make([]MemoryCondition, 0, len(incident.Status.Conditions)),
	}
	for _, c := range incident.Status.Conditions {
		mc := MemoryCondition{
			Type: c.Type, Status: string(c.Status), Reason: c.Reason, Message: c.Message,
			LastTransitionTime: c.LastTransitionTime,
		}
		rec.Conditions = append(rec.Conditions, mc)
	}
	if r := incident.Spec.Resolution; r != nil && r.Duration != nil {
		rec.ResolutionDuration = r.Duration.Duration.String()
	}
	return rec
}
