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
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/pkg/report"
)

// DefaultOverviewWindow is the window the live Overview is computed for, the same
// default as `dorgu report`.
const DefaultOverviewWindow = 7 * 24 * time.Hour

// OperatorLabel marks the operator's own Deployment.
const OperatorLabel = "app.kubernetes.io/name"

// OperatorLabelValue is the value of OperatorLabel on the operator Deployment.
const OperatorLabelValue = "dorgu-operator"

// InputRead says whether one input to the Overview could be read, and why not.
type InputRead struct {
	// Present is false when the API server does not serve the resource.
	Present bool
	// Synced is true once the informer completed its initial list.
	Synced bool
	// Failure is why the informer gave up, empty when it did not.
	Failure string
}

// readable is true when the input can be trusted as a complete list.
func (r InputRead) readable() bool { return r.Present && r.Synced && r.Failure == "" }

// OverviewReads is what the Overview could read, per input.
type OverviewReads struct {
	Remediations    InputRead
	Incidents       InputRead
	Personas        InputRead
	ClusterPersonas InputRead
	Events          InputRead
}

// OverviewInput is everything the Overview is computed from.
type OverviewInput struct {
	Remediations    []*dorguv1.RemediationAction
	Incidents       []*dorguv1.IncidentMemory
	Events          []*corev1.Event
	Deployments     []*appsv1.Deployment
	AppPersonas     []*dorguv1.ApplicationPersona
	ClusterPersonas []*dorguv1.ClusterPersona

	// Context and Namespace describe the connection, shown in the report header.
	Context   string
	Namespace string

	Now   time.Time
	Since time.Duration

	Reads     OverviewReads
	Readiness Readiness
}

// OperatorStatus is what the Deployments say about the operator itself.
type OperatorStatus struct {
	// Found is true when a Deployment labelled as the operator is in the cache.
	Found bool `json:"found"`
	// Desired and Ready are the replica counts of that Deployment.
	Desired int32 `json:"desired"`
	Ready   int32 `json:"ready"`
	// Version is the image tag, nil when it cannot be read or is a digest.
	Version *string `json:"version"`
}

// Running reports whether at least one operator replica is ready.
func (o OperatorStatus) Running() bool { return o.Found && o.Ready > 0 }

// OverviewPayload is the whole Overview in one object.
type OverviewPayload struct {
	// Report is the KPI summary, byte for byte what `dorgu report --json` would
	// compute from the same records. It is null when it cannot be computed: the
	// RemediationActions are the report's core, and without them there is no
	// report, which is a different statement from a report of zero.
	Report    *report.Doc    `json:"report"`
	Window    string         `json:"window"`
	Readiness Readiness      `json:"readiness"`
	Operator  OperatorStatus `json:"operator"`
}

// BuildOverview computes the report from the cache.
//
// It reads nothing itself. Every list arrives as an argument, and the clock is an
// argument, so the same cache and the same instant give the same payload.
func BuildOverview(in OverviewInput) OverviewPayload {
	op := OperatorStatusOf(in.Deployments)
	out := OverviewPayload{
		Window:    report.FormatWindow(in.Since),
		Readiness: in.Readiness,
		Operator:  op,
	}
	// The core of the report is the RemediationActions. Not served, not yet
	// listed, or refused: there is nothing honest to compute.
	if !in.Reads.Remediations.readable() {
		return out
	}

	ras := make([]dorguv1.RemediationAction, len(in.Remediations))
	for i, r := range in.Remediations {
		ras[i] = *r
	}
	incidents := make([]dorguv1.IncidentMemory, len(in.Incidents))
	for i, m := range in.Incidents {
		incidents[i] = *m
	}
	events := make([]corev1.Event, len(in.Events))
	for i, e := range in.Events {
		events[i] = *e
	}

	inputs := report.Inputs{
		Remediations:    ras,
		Incidents:       incidents,
		Events:          events,
		EventsRead:      in.Reads.Events.readable(),
		OperatorVersion: op.Version,
	}
	var sources report.Sources
	sources.RemediationActions = report.Source{OK: true, Count: len(ras)}
	sources.Incidents = sourceFor("incidents", in.Reads.Incidents, len(incidents), &inputs)
	sources.Events = sourceFor("events", in.Reads.Events, len(events), &inputs)
	sources.Personas = sourceFor("personas", in.Reads.Personas, len(in.AppPersonas), &inputs)
	sources.ClusterPersonas = sourceFor("cluster_personas", in.Reads.ClusterPersonas, len(in.ClusterPersonas), &inputs)

	doc := report.Build(inputs, in.Now.UTC(), in.Since)
	doc.Sources = sources
	doc.Cluster = report.Cluster{
		Context:         in.Context,
		Namespace:       "all",
		OperatorVersion: op.Version,
		SelfHealing:     []string{},
	}
	if in.Namespace != "" {
		doc.Cluster.Namespace = in.Namespace
	}
	if in.Reads.Personas.readable() {
		n := len(in.AppPersonas)
		doc.Cluster.Personas = &n
	}
	if in.Reads.ClusterPersonas.readable() {
		for _, cp := range in.ClusterPersonas {
			mode := dorguv1.SelfHealingModePropose
			if p := cp.Spec.Policies; p != nil && p.SelfHealing != nil && p.SelfHealing.Mode != "" {
				mode = p.SelfHealing.Mode
			}
			doc.Cluster.SelfHealing = append(doc.Cluster.SelfHealing, mode)
		}
	}
	out.Report = &doc
	return out
}

// sourceFor describes one input's read and records a failed one in the report's
// input errors, in the same bounded vocabulary the CLI uses. An unread input is
// the reason a section says "no data", so it is named here and not left as an
// absence.
func sourceFor(name string, read InputRead, count int, inputs *report.Inputs) report.Source {
	switch {
	case read.readable():
		return report.Source{OK: true, Count: count}
	case !read.Present:
		// Not installed. Events are core and always served; the others are CRDs.
		inputs.Errors = append(inputs.Errors, report.InputError{Input: name, Error: "not_found"})
		return report.Source{Error: "the API server does not serve this resource"}
	default:
		inputs.Errors = append(inputs.Errors, report.InputError{Input: name, Error: "timeout"})
		return report.Source{Error: read.Failure}
	}
}

// OperatorStatusOf reads the operator Deployment out of the cache.
func OperatorStatusOf(deployments []*appsv1.Deployment) OperatorStatus {
	for _, d := range deployments {
		if d.Labels[OperatorLabel] != OperatorLabelValue {
			continue
		}
		st := OperatorStatus{Found: true, Ready: d.Status.ReadyReplicas}
		if d.Spec.Replicas != nil {
			st.Desired = *d.Spec.Replicas
		} else {
			st.Desired = 1
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if tag := imageTag(c.Image); tag != "" {
				st.Version = &tag
				break
			}
		}
		return st
	}
	return OperatorStatus{}
}

// imageTag is the tag of an image reference, or "" when it has none or is pinned
// by digest.
func imageTag(image string) string {
	if strings.Contains(image, "@") {
		return ""
	}
	slash := strings.LastIndex(image, "/")
	if colon := strings.LastIndex(image, ":"); colon > slash {
		return image[colon+1:]
	}
	return ""
}
