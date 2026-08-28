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

// Package view projects the cache into the payloads the browser renders.
//
// Every function here is pure: cache contents in, view struct out, no clock and
// no I/O except a caller-supplied "now". That is deliberate, because these
// functions encode the judgements a reader will trust (this app is unhealthy,
// this Deployment is unmonitored, this incident is critical) and those need to
// be pinned by tests rather than eyeballed in a browser.
package view

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// Health status values reported for a persona-backed app. These are the
// operator's own vocabulary from ApplicationPersonaStatus.Health.
const (
	HealthHealthy   = "Healthy"
	HealthDegraded  = "Degraded"
	HealthUnhealthy = "Unhealthy"
	HealthUnknown   = "Unknown"
)

// Health sources. The distinction is load-bearing: a persona status is the
// operator's verdict, and an observed status is what this process read off the
// Deployment and its Pods a moment ago. When the operator is not running, only
// the second exists, and the UI must not present it as the first.
const (
	// HealthSourcePersona means the value came from
	// ApplicationPersonaStatus.Health, written by the operator.
	HealthSourcePersona = "persona-status"
	// HealthSourceObserved means the dashboard derived the value from live
	// Deployment replica counts and Pod container states.
	HealthSourceObserved = "observed"
	// HealthSourceNone means there was nothing to derive a value from.
	HealthSourceNone = "none"
)

// App source values. An app row is either a persona the operator knows about or
// a Deployment nothing is watching.
const (
	// SourcePersona is a row backed by an ApplicationPersona.
	SourcePersona = "persona"
	// SourceDeployment is a row backed only by a Deployment: an app Dorgu is
	// blind to until someone runs `dorgu persona import`.
	SourceDeployment = "deployment"
)

// AppsPayload is the whole Apps view in one object. The SSE stream pushes this
// same shape, so the REST read and the live update cannot drift.
type AppsPayload struct {
	// Apps holds persona-backed rows first, then unmonitored Deployments.
	Apps []App `json:"apps"`
	// Summary counts what the user is looking at, so the header does not have
	// to derive it and disagree with the table.
	Summary AppsSummary `json:"summary"`
	// Readiness states what this payload does and does not know. An empty Apps
	// list means something different under each combination of these flags,
	// and the UI is required to say which.
	Readiness Readiness `json:"readiness"`
}

// AppsSummary is the count line above the Apps table.
type AppsSummary struct {
	Total       int `json:"total"`
	Monitored   int `json:"monitored"`
	Unmonitored int `json:"unmonitored"`
	Healthy     int `json:"healthy"`
	Degraded    int `json:"degraded"`
	Unhealthy   int `json:"unhealthy"`
	// UnmonitoredNamespaces lists the namespaces holding uncovered Deployments,
	// so the import prompt can name each one the way `dorgu health` does.
	UnmonitoredNamespaces []string `json:"unmonitoredNamespaces"`
}

// App is one row of the Apps view.
type App struct {
	// ID is a stable row identity for the client: "<source>/<namespace>/<name>".
	// It survives every field of the row changing, which is what keeps a
	// virtualised table from re-keying on every SSE push.
	ID string `json:"id"`
	// Source is SourcePersona or SourceDeployment.
	Source string `json:"source"`
	// Monitored reports whether an ApplicationPersona covers this app.
	Monitored bool `json:"monitored"`

	// Namespace is the namespace of the persona, or of the Deployment for an
	// unmonitored row.
	Namespace string `json:"namespace"`
	// Name is the object's metadata.name.
	Name string `json:"name"`
	// AppName is the persona's spec.name, which is what the match chain
	// resolves by and is often not Name. For an unmonitored row it is the
	// Deployment name.
	AppName string `json:"appName"`

	// Type and Tier come from the persona spec and are empty for unmonitored
	// rows: nothing has classified those apps yet, and guessing a tier for a
	// workload nobody has described is exactly the confident-wrong-answer
	// failure this product exists to avoid.
	Type string `json:"type,omitempty"`
	Tier string `json:"tier,omitempty"`
	// Phase is the persona lifecycle phase, empty for unmonitored rows.
	Phase string `json:"phase,omitempty"`

	// Health is the app's health and where that reading came from.
	Health AppHealth `json:"health"`
	// Workload is the live Deployment this app resolved to, absent when no
	// Deployment matched.
	Workload *Workload `json:"workload,omitempty"`
	// Ownership names the team responsible, from the persona spec.
	Ownership *AppOwnership `json:"ownership,omitempty"`
	// Incidents counts what is open against this app.
	Incidents AppIncidents `json:"incidents"`

	// ImportCommand is the exact command that onboards an unmonitored
	// Deployment, ready to copy. Empty for monitored rows.
	ImportCommand string `json:"importCommand,omitempty"`

	// Warnings are things a reader needs to know before trusting this row:
	// an ambiguous match, a persona whose Deployment could not be found. They
	// are shown, not logged.
	Warnings []string `json:"warnings,omitempty"`

	CreatedAt   metav1.Time  `json:"createdAt"`
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// AppHealth is a health reading plus its provenance.
type AppHealth struct {
	// Status is one of the Health* constants.
	Status string `json:"status"`
	// Source is one of the HealthSource* constants.
	Source string `json:"source"`
	// Message explains the status in one line.
	Message string `json:"message,omitempty"`
	// LastCheck is when the operator last checked, for a persona-sourced
	// status.
	LastCheck *metav1.Time `json:"lastCheck,omitempty"`
	// PodIssues are container-level failures read live from Pods. They are
	// present regardless of Source, because they are a direct observation
	// rather than a verdict.
	PodIssues []PodIssue `json:"podIssues,omitempty"`
	// Disagreement is set when the operator's recorded verdict is more
	// flattering than what the dashboard can see, and states both readings.
	//
	// It is never empty for the sake of it. A persona's status is written on a
	// reconcile interval, so a stale Healthy over a live crash loop is the one
	// way this view could tell a comfortable lie, and this field is where that
	// gets said out loud instead.
	Disagreement string `json:"disagreement,omitempty"`
}

// PodIssue is one container in a bad state right now.
//
// It mirrors dorguv1.PodFailure field for field and adds what the Pod object
// carries but the CRD does not record: the phase and restart count, which are
// what a reader uses to tell a crash loop from a single bad start.
type PodIssue struct {
	PodName      string `json:"podName"`
	Container    string `json:"container"`
	Reason       string `json:"reason"`
	Message      string `json:"message,omitempty"`
	Phase        string `json:"phase,omitempty"`
	RestartCount int32  `json:"restartCount,omitempty"`
}

// Workload is the live Deployment an app resolved to.
type Workload struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	// Container is the container whose image and resources were read.
	Container string `json:"container,omitempty"`
	// MatchedBy names the rung of the match chain that tied this Deployment to
	// the persona, so a reader can check the link rather than assume it. Empty
	// for an unmonitored row, where there is no persona to match.
	MatchedBy string `json:"matchedBy,omitempty"`

	// ManagedBy is who owns this workload's desired state, one of the
	// dorguv1.ManagedBy* values. Anything other than "unmanaged" means Dorgu
	// will explain rather than patch.
	ManagedBy string `json:"managedBy"`
	// ManagedByDetail names that owner in prose.
	ManagedByDetail string `json:"managedByDetail,omitempty"`

	// Image is the container's live image reference including its tag.
	Image string `json:"image,omitempty"`
	// ObservedResources is the live container resource block, with absent keys
	// preserved as empty strings. Reuses the CRD type so the dashboard and a
	// RemediationAction describe resources identically.
	ObservedResources *dorguv1.ObservedResources `json:"observedResources,omitempty"`

	Replicas WorkloadReplicas `json:"replicas"`
}

// WorkloadReplicas is a Deployment's replica arithmetic.
type WorkloadReplicas struct {
	Desired   int32 `json:"desired"`
	Ready     int32 `json:"ready"`
	Available int32 `json:"available"`
	Updated   int32 `json:"updated"`
}

// AppOwnership is the human owner of an app, from the persona spec.
type AppOwnership struct {
	Team       string `json:"team,omitempty"`
	Owner      string `json:"owner,omitempty"`
	OnCall     string `json:"oncall,omitempty"`
	Runbook    string `json:"runbook,omitempty"`
	Repository string `json:"repository,omitempty"`
}

// AppIncidents counts incidents against an app.
type AppIncidents struct {
	// Open is the number of unresolved IncidentMemories the dashboard can see
	// for this app, counted from the cache rather than taken from persona
	// status. The two can disagree, and when they do the cache is the fresher
	// of the two.
	Open int `json:"open"`
	// Critical is how many of Open are severity critical.
	Critical int `json:"critical"`
	// PersonaReported is the operator's own activeIncidents count. Shown
	// separately rather than reconciled: a mismatch is a real signal about the
	// operator and hiding it would be the silent failure the house rules
	// forbid.
	PersonaReported int32 `json:"personaReported"`
	// LastIncidentTime is the most recent incident for this app.
	LastIncidentTime *metav1.Time `json:"lastIncidentTime,omitempty"`
}

// IncidentsPayload is the whole Incidents view in one object.
type IncidentsPayload struct {
	Incidents  []Incident        `json:"incidents"`
	Summary    IncidentsSummary  `json:"summary"`
	Readiness  Readiness         `json:"readiness"`
	Truncation *TruncationNotice `json:"truncation,omitempty"`
}

// IncidentsSummary is the count line above the incident feed.
type IncidentsSummary struct {
	Total    int `json:"total"`
	Open     int `json:"open"`
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
	// Unattributed counts incidents Dorgu can see but cannot tie to a persona,
	// which are real outages it cannot diagnose or remediate.
	Unattributed int `json:"unattributed"`
	// Diagnosed counts incidents that carry a root cause.
	Diagnosed int `json:"diagnosed"`
}

// TruncationNotice reports that a payload was capped. It exists so a capped
// list can never read as a complete one: the count that was dropped is stated
// in the UI rather than swallowed.
type TruncationNotice struct {
	// Shown is how many rows the payload carries.
	Shown int `json:"shown"`
	// Total is how many exist in the cache.
	Total int `json:"total"`
	// Limit is the cap that was applied.
	Limit int `json:"limit"`
}

// Incident is one row of the Incidents view.
type Incident struct {
	// ID is "<namespace>/<name>", stable across every other field changing.
	ID        string `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`

	Severity string `json:"severity"`
	Category string `json:"category"`
	Phase    string `json:"phase"`
	// Attribution is "persona" or "unattributed". An unattributed incident
	// names the workload the signals came from and no persona of that name need
	// exist.
	Attribution string `json:"attribution"`

	// Signal is the primary detection signal, e.g. OOMKilled.
	Signal string `json:"signal"`
	// Source is the detector that raised it.
	Source string `json:"source"`

	Persona IncidentPersona `json:"persona"`

	// RootCause is the diagnosis, absent when the incident has not been
	// diagnosed. Absent is a real and common state: detection runs for free and
	// AI diagnosis is opt-in.
	RootCause *IncidentRootCause `json:"rootCause,omitempty"`

	// AffectedResources lists the Kubernetes objects involved.
	AffectedResources []dorguv1.ResourceReference `json:"affectedResources,omitempty"`

	// Resolution is how the incident was closed, absent while open. An outcome
	// of "acknowledged" means a human approved an advisory plan: the decision
	// is recorded, nothing was applied, and the incident is still open.
	Resolution *IncidentResolution `json:"resolution,omitempty"`

	FirstSeen       metav1.Time  `json:"firstSeen"`
	LastSeen        metav1.Time  `json:"lastSeen"`
	OccurrenceCount int32        `json:"occurrenceCount"`
	LastOccurrence  *metav1.Time `json:"lastOccurrence,omitempty"`
	CreatedAt       metav1.Time  `json:"createdAt"`
}

// IncidentPersona is the persona an incident is filed against.
type IncidentPersona struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	// Exists reports whether a persona of that name is actually in the cache.
	// False on an unattributed incident, where personaRef names a workload
	// rather than a persona.
	Exists bool `json:"exists"`
}

// IncidentRootCause is a diagnosis, carried with its provenance.
type IncidentRootCause struct {
	Summary string `json:"summary"`
	// Confidence is the CRD's decimal string, passed through unparsed. The
	// server does not round it: a confidence is a claim the diagnosis made and
	// reformatting it here would put words in its mouth.
	Confidence string `json:"confidence"`
	// Provider is the diagnosis source, e.g. "rule-engine" or "ai-enhanced".
	Provider     string                       `json:"provider"`
	Contributing []dorguv1.ContributingSignal `json:"contributing,omitempty"`
}

// IncidentResolution is how an incident was closed.
type IncidentResolution struct {
	Action    string       `json:"action"`
	Outcome   string       `json:"outcome,omitempty"`
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`
	// RemediationName is the RemediationAction that closed it, for the M3 view
	// to link to.
	RemediationName      string `json:"remediationName,omitempty"`
	RemediationNamespace string `json:"remediationNamespace,omitempty"`
}

// Readiness is the honesty header on every payload.
//
// It exists because an empty list has at least four different causes and they
// call for four different screens: the informers have not synced, the CRDs are
// not installed, the operator is installed but has found nothing, or the user
// genuinely has no apps. Reporting an empty array without this is how a tool
// ends up presenting a blind spot as good news.
type Readiness struct {
	// Synced reports that every informer feeding this view has completed its
	// initial list. Until it is true, an empty list means "loading".
	Synced bool `json:"synced"`
	// CRDsInstalled reports that the dorgu.io kinds this view needs are served
	// by the API server. False means the operator is not installed, which is a
	// statement about the cluster and not about the user's apps.
	CRDsInstalled bool `json:"crdsInstalled"`
	// MissingCRDs names the kinds that were looked for and not found.
	MissingCRDs []string `json:"missingCRDs,omitempty"`

	// Unavailable names the resources this view needs that could not be read at
	// all, with the reason.
	//
	// It is the fifth cause of an empty list and the only one the first four
	// cannot express: the resource exists, the operator is installed, and the
	// read was refused. Without it a watch the API server rejects leaves Synced
	// false forever and the screen sits on a loading skeleton, telling the reader
	// it is still reading something it gave up on. The likeliest cause is RBAC,
	// and a namespace-scoped kubeconfig cannot list Nodes at all, so the Cluster
	// view reaches this by design rather than by accident.
	Unavailable []UnavailableResource `json:"unavailable,omitempty"`
}

// UnavailableResource is one resource a view needs and could not read.
type UnavailableResource struct {
	// Resource is the plural resource name, as the API server serves it.
	Resource string `json:"resource"`
	// Reason is why the read did not succeed, ready to render.
	Reason string `json:"reason"`
}
