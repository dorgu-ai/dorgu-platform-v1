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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The Cluster view, and the one rule that shapes all of it: every number says
// where it came from.
//
// This view was gated for a release because cluster health reported 1689% CPU on
// a cluster where 25% was requested and 1% was in use. The figure was
// ClusterPersona.status.resourceSummary.cpuUtilization rendered verbatim, and
// the operator was summing the requests of pods no node had accepted. That is
// fixed at the source in operator v0.11.0, and the CLI stopped depending on the
// field in v0.12.0 by computing saturation itself.
//
// This view does the same, for the same reason plus one more: a card is read as
// authoritative in a way terminal output is not, and the field is written on a
// reconcile interval by whatever operator version happens to be installed. So
// saturation here is arithmetic over the live Node and Pod lists, and the only
// things read from ClusterPersona are the ones the operator is genuinely the
// source for (its own discovery: add-ons, namespace counts, platform). Each is
// labelled with which of the two it is.

// Sources a Cluster-view figure can come from. The vocabulary matches the Apps
// view's health sources deliberately: a reader who has learned what "observed"
// means on one screen must not have to learn it again on another.
const (
	// SourceObserved means the dashboard computed the value from live objects
	// it watched: Nodes, Pods, or its own read of metrics-server.
	SourceObserved = HealthSourceObserved
	// SourcePersonaStatus means the value was written by the operator into
	// ClusterPersona.status.
	SourcePersonaStatus = HealthSourcePersona
	// SourceNone means there was nothing to derive the value from.
	SourceNone = HealthSourceNone
)

// SaturationWarnPercent is the requested share at which the view stops being
// neutral, matching the CLI's threshold so the two agree about the same cluster.
//
// Above it the cluster genuinely cannot take much more, and the reader should
// not have to do the division that the old output had already got wrong.
const SaturationWarnPercent = 90

// ClusterPayload is the whole Cluster view in one object.
type ClusterPayload struct {
	Identity   ClusterIdentity `json:"identity"`
	Saturation Saturation      `json:"saturation"`
	Nodes      []Node          `json:"nodes"`
	Addons     []Addon         `json:"addons"`
	Summary    ClusterSummary  `json:"summary"`
	Readiness  Readiness       `json:"readiness"`
}

// ClusterIdentity is which cluster this is, and who says so.
type ClusterIdentity struct {
	// PersonaPresent reports whether a ClusterPersona exists. False means the
	// operator has not described this cluster, which is why the name, the
	// environment and the add-on list are empty rather than unknown.
	PersonaPresent bool `json:"personaPresent"`

	// Name and Environment come from the ClusterPersona spec, which an admin or
	// GitOps owns. They are empty with no persona.
	Name        string `json:"name,omitempty"`
	Environment string `json:"environment,omitempty"`
	Description string `json:"description,omitempty"`

	// KubernetesVersion is the server version. Source says whether it was read
	// off the nodes' kubelets or taken from the operator's discovery.
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	// KubernetesVersionSource is one of the Source* constants.
	KubernetesVersionSource string `json:"kubernetesVersionSource"`
	// KubeletVersions lists every distinct kubelet version, in sorted order,
	// when the nodes do not agree. A mixed-version cluster is a real state and
	// a mid-upgrade one is exactly when somebody opens this screen, so it is
	// stated rather than collapsed to whichever node sorted first.
	KubeletVersions []string `json:"kubeletVersions,omitempty"`

	// Architectures lists every distinct node architecture, sorted.
	//
	// It is here because of a defect that shipped: every operator image before
	// v0.11.1 was amd64-only, so an arm64 node could not pull it and the pod sat
	// in ImagePullBackOff. A reader debugging that needs to know their nodes are
	// arm64 without running kubectl.
	Architectures []string `json:"architectures,omitempty"`

	// Platform is the operator's guess at the hosting platform (eks, gke, kind
	// and so on). Operator-sourced, so it is absent without a persona.
	Platform string `json:"platform,omitempty"`

	// Namespaces is the operator's namespace count. Namespaces are not watched
	// by the dashboard, so this is persona-sourced or absent; it is a pointer
	// rather than a zero-valued struct because "no persona" and "zero
	// namespaces" are different claims and zero namespaces is impossible.
	Namespaces *NamespaceCounts `json:"namespaces,omitempty"`

	// LastDiscovery is when the operator last described this cluster. It is
	// shown next to every persona-sourced figure, because a reconcile interval
	// is exactly how stale those can be.
	LastDiscovery *metav1.Time `json:"lastDiscovery,omitempty"`
}

// NamespaceCounts is the operator's namespace tally.
type NamespaceCounts struct {
	Total        int32 `json:"total"`
	Active       int32 `json:"active"`
	WithPersonas int32 `json:"withPersonas"`
}

// Saturation is what the cluster has committed and what it is using.
//
// Two figures, never one. Requested is what the scheduler has handed out; used
// is what the containers are burning. A single number cannot say which it means,
// and on the cluster that produced the 1689% those were 25% and 1%.
type Saturation struct {
	CPU    *SaturationDetail `json:"cpu,omitempty"`
	Memory *SaturationDetail `json:"memory,omitempty"`

	// Nodes and ScheduledPods say what the figures cover, so a reader can tell
	// a quiet cluster from a partial read.
	Nodes         int `json:"nodes"`
	ScheduledPods int `json:"scheduledPods"`

	// UnscheduledPods counts the pods left out of the requested figure because
	// no node has accepted them.
	//
	// Reporting the count keeps the exclusion visible and surfaces the real
	// problem the old percentage was burying. A pod no node has accepted holds
	// no allocation on any node, and because it can request more than the
	// cluster owns, counting it has no upper bound: that is the whole of the
	// 1689%.
	UnscheduledPods int `json:"unscheduledPods"`

	// UsedUnavailable is why there is no used figure, ready to render. It is
	// non-empty exactly when the details carry no UsedPercent, so the UI shows
	// "n/a (<reason>)" and never an empty operand or a zero bar.
	UsedUnavailable string `json:"usedUnavailable,omitempty"`
	// UsedReadAt is when the used figures were measured. Shown because a polled
	// number should not be presented as a live one.
	UsedReadAt *metav1.Time `json:"usedReadAt,omitempty"`
}

// SaturationDetail reports one resource against the allocatable pool.
//
// Quantities are formatted here so the dashboard and `dorgu health` render the
// same cluster identically. Percentages are numbers, not strings, because the
// UI sizes a meter from them: shipping "25%" would make the browser parse a
// string this package had already rounded, which is two answers to one
// question.
type SaturationDetail struct {
	// Allocatable is what the scheduler may hand out, summed over the nodes.
	// Allocatable rather than capacity: capacity includes what the kubelet and
	// the OS reserve and will never be given to a pod.
	Allocatable string `json:"allocatable"`

	// Requested is what the scheduled pods have claimed.
	Requested        string  `json:"requested"`
	RequestedPercent float64 `json:"requestedPercent"`

	// Used is live consumption, absent when metrics-server did not answer. A
	// nil UsedPercent is the signal: it makes rendering zero impossible rather
	// than merely discouraged.
	Used        string   `json:"used,omitempty"`
	UsedPercent *float64 `json:"usedPercent,omitempty"`

	// Pressure reports that requests are at or above SaturationWarnPercent, so
	// new pods may not schedule. Computed here rather than compared in the UI,
	// because the threshold is a product decision and the CLI holds the same
	// one.
	Pressure bool `json:"pressure"`
}

// Node is one row of the node table.
type Node struct {
	// ID is the node name, which is unique cluster-wide and stable across every
	// other field changing.
	ID   string `json:"id"`
	Name string `json:"name"`

	// Ready is the Ready condition. NotReadyReason carries the condition's own
	// reason and message when it is not true, so the row says why rather than
	// only that.
	Ready          bool   `json:"ready"`
	NotReadyReason string `json:"notReadyReason,omitempty"`

	// Schedulable is false when the node is cordoned. It is separate from Ready
	// because a cordoned node is healthy and deliberately closed, and folding
	// the two would report a maintenance window as a fault.
	Schedulable bool `json:"schedulable"`

	// Roles are read from the node-role.kubernetes.io/* labels, sorted.
	Roles []string `json:"roles,omitempty"`
	// Taints are rendered as key=value:Effect, sorted. They are on the row
	// because a tainted node with free capacity explains an unschedulable pod.
	Taints []string `json:"taints,omitempty"`

	KubeletVersion   string `json:"kubeletVersion,omitempty"`
	ContainerRuntime string `json:"containerRuntime,omitempty"`
	OS               string `json:"os,omitempty"`
	Architecture     string `json:"architecture,omitempty"`

	// Allocatable and Capacity are both reported. The difference is what the
	// kubelet and the OS reserve, and a reader comparing a node's capacity to
	// the requests it is carrying needs to know which pool is which.
	Allocatable NodeResources `json:"allocatable"`
	Capacity    NodeResources `json:"capacity"`

	// Requested is what the pods on this node have claimed from it, as a share
	// of its allocatable pool. Per-node rather than only cluster-wide, because
	// a cluster at 40% with one node at 98% is the shape that stops a rollout.
	RequestedCPUPercent    float64 `json:"requestedCpuPercent"`
	RequestedMemoryPercent float64 `json:"requestedMemoryPercent"`

	// Pods is how many pods this node is carrying against how many it will
	// take. Pod count is a real scheduling limit and is invisible in the CPU
	// and memory figures.
	Pods NodePods `json:"pods"`

	CreatedAt metav1.Time `json:"createdAt"`
}

// NodeResources is one node's resource pool, formatted for display.
type NodeResources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	Pods   string `json:"pods,omitempty"`
}

// NodePods is a node's pod count against its pod capacity.
type NodePods struct {
	Scheduled int   `json:"scheduled"`
	Capacity  int64 `json:"capacity"`
	// Percent is Scheduled over Capacity, or 0 when the node reports no pod
	// capacity.
	Percent float64 `json:"percent"`
}

// Addon is one cluster add-on as the operator discovered it.
type Addon struct {
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Version   string `json:"version,omitempty"`
	Installed bool   `json:"installed"`
	// Healthy is the operator's verdict, absent when it did not reach one.
	Healthy *bool `json:"healthy,omitempty"`

	// Disagreement is set when the dashboard's own first-hand observation
	// contradicts the operator's record, and states both readings.
	//
	// Today that means one thing: the operator says metrics-server is installed
	// and this process asked it for node usage and got nothing. That is the same
	// judgement the Apps view makes when a persona's Healthy sits over a live
	// crash loop, and it is made here for the same reason: the observation is
	// first-hand and the record is on a reconcile interval.
	Disagreement string `json:"disagreement,omitempty"`
}

// ClusterSummary is the count line above the view.
type ClusterSummary struct {
	Nodes              int `json:"nodes"`
	NodesReady         int `json:"nodesReady"`
	NodesUnschedulable int `json:"nodesUnschedulable"`
	ScheduledPods      int `json:"scheduledPods"`
	UnscheduledPods    int `json:"unscheduledPods"`
	AddonsInstalled    int `json:"addonsInstalled"`
	AddonsUnhealthy    int `json:"addonsUnhealthy"`
}
