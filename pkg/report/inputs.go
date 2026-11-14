package report

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// remAction and incidentMemory are the operator's own api/v1 types. The report
// reads them as they are served, so a caller with a typed client or an informer
// cache passes them straight in, and one that parsed kubectl JSON converts into
// them first. Nothing in this package reads a cluster.
type (
	remAction      = dorguv1.RemediationAction
	incidentMemory = dorguv1.IncidentMemory
)

// Inputs is everything the report is computed from.
type Inputs struct {
	Remediations []dorguv1.RemediationAction
	Incidents    []dorguv1.IncidentMemory

	// Events are the core/v1 Events whose involved object is a RemediationAction.
	// Only the CLI's own decline Events are counted from them.
	Events []corev1.Event

	// EventsRead is true when the Events were listed. An empty list that was read
	// is a measured zero; one that was not read is no data.
	EventsRead bool

	// Errors are the cluster reads that failed. A section computed from one of them
	// is no data (see Build), never zero.
	Errors []InputError

	// OperatorVersion is the operator's image tag, nil when it could not be read.
	OperatorVersion *string
}

// unreadable reports whether the named input failed to read.
func (in Inputs) unreadable(input string) bool {
	for _, e := range in.Errors {
		if e.Input == input {
			return true
		}
	}
	return false
}

// timeOf reads a metav1.Time as "a time or none". The zero time is what a missing
// or unparseable timestamp decodes to, and it is never a real moment.
func timeOf(t metav1.Time) (time.Time, bool) {
	if t.IsZero() {
		return time.Time{}, false
	}
	return t.Time, true
}

func timePtr(t *metav1.Time) (time.Time, bool) {
	if t == nil {
		return time.Time{}, false
	}
	return timeOf(*t)
}

// microTimeOf is timeOf for the MicroTime an events.k8s.io Event carries.
func microTimeOf(t metav1.MicroTime) (time.Time, bool) {
	if t.IsZero() {
		return time.Time{}, false
	}
	return t.Time, true
}

// eventAt is when the event happened: the newest timestamp it carries.
func eventAt(e *corev1.Event) (time.Time, bool) {
	if t, ok := timeOf(e.LastTimestamp); ok {
		return t, true
	}
	if t, ok := microTimeOf(e.EventTime); ok {
		return t, true
	}
	return timeOf(e.FirstTimestamp)
}

// eventOccurrences is how many times the event fired (Kubernetes folds repeats).
func eventOccurrences(e *corev1.Event) int {
	if e.Series != nil && e.Series.Count > 0 {
		return int(e.Series.Count)
	}
	if e.Count > 0 {
		return int(e.Count)
	}
	return 1
}

// condition finds a status condition by type.
func condition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

// condTime is when a condition last changed.
func condTime(c *metav1.Condition) (time.Time, bool) {
	if c == nil {
		return time.Time{}, false
	}
	return timeOf(c.LastTransitionTime)
}

func createdAt(r *remAction) (time.Time, bool) { return timeOf(r.CreationTimestamp) }
