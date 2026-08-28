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
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

// nodeRoleLabelPrefix is where a node's role is recorded. kubeadm, EKS, GKE and
// kind all write it, and there is no field on the Node object that carries it.
const nodeRoleLabelPrefix = "node-role.kubernetes.io/"

// metricsServerAddon is the add-on name the operator records for
// metrics-server, and the one add-on this process can check first-hand.
const metricsServerAddon = "metrics-server"

// ClusterInput is everything the Cluster view is built from.
type ClusterInput struct {
	// Nodes and Pods are the live lists saturation is computed over. They are
	// the whole of the arithmetic: nothing here reads
	// ClusterPersona.status.resourceSummary, which is the field that reported
	// 1689%.
	Nodes []*corev1.Node
	Pods  []*corev1.Pod

	// Personas holds the ClusterPersonas in the cache. The kind is
	// cluster-scoped and the operator creates one, but the list is taken rather
	// than a single object because nothing stops a second existing, and picking
	// one deterministically beats rendering whichever the map iterated first.
	Personas []*dorguv1.ClusterPersona

	// NodeUsage is the last answer from metrics-server, or the last reason there
	// was none.
	NodeUsage store.NodeUsage

	Readiness Readiness
}

// BuildCluster projects the live cluster into the Cluster view.
//
// Pods are deliberately not filtered by the namespace scope. Saturation is a
// property of the whole cluster, and summing one namespace's requests against
// every node's allocatable would produce a number that is wrong in a way nobody
// could detect from the screen. A --namespace run scopes the Apps and Incidents
// views; this one says what the cluster is doing.
func BuildCluster(in ClusterInput) ClusterPayload {
	capacity := sumNodeCapacity(in.Nodes)
	claims := sumPodClaims(in.Pods)
	persona := pickClusterPersona(in.Personas)

	nodes := projectNodes(in.Nodes, claims)
	addons := projectAddons(persona, in.NodeUsage)

	return ClusterPayload{
		Identity:   buildIdentity(persona, in.Nodes),
		Saturation: buildSaturation(capacity, claims, in.NodeUsage),
		Nodes:      nodes,
		Addons:     addons,
		Summary:    summariseCluster(nodes, claims, addons),
		Readiness:  in.Readiness,
	}
}

// pickClusterPersona returns the persona to render, or nil.
//
// Sorted by name and the first taken, so two runs against the same cluster show
// the same one. A cluster with two ClusterPersonas is a misconfiguration, but
// rendering it non-deterministically would make that misconfiguration look like
// a flickering dashboard.
func pickClusterPersona(personas []*dorguv1.ClusterPersona) *dorguv1.ClusterPersona {
	var present []*dorguv1.ClusterPersona
	for _, persona := range personas {
		if persona != nil {
			present = append(present, persona)
		}
	}
	if len(present) == 0 {
		return nil
	}
	sort.Slice(present, func(a, b int) bool { return present[a].Name < present[b].Name })
	return present[0]
}

// buildIdentity assembles which cluster this is, labelling each figure with its
// source.
//
// The Kubernetes version prefers the kubelet versions read off the live nodes
// over the operator's recorded one. Both are real answers, but the observed one
// cannot be stale and cannot come from an operator version with a bug in it,
// which is the same reasoning the Apps view uses when it lets an observation
// override a persona's verdict.
func buildIdentity(persona *dorguv1.ClusterPersona, nodes []*corev1.Node) ClusterIdentity {
	out := ClusterIdentity{
		KubernetesVersionSource: SourceNone,
		KubeletVersions:         distinctKubeletVersions(nodes),
		Architectures:           distinctArchitectures(nodes),
	}

	if len(out.KubeletVersions) > 0 {
		out.KubernetesVersion = out.KubeletVersions[0]
		out.KubernetesVersionSource = SourceObserved
		// One agreed version is not a disagreement worth listing twice.
		if len(out.KubeletVersions) == 1 {
			out.KubeletVersions = nil
		}
	}

	if persona == nil {
		return out
	}

	out.PersonaPresent = true
	out.Name = persona.Spec.Name
	out.Environment = persona.Spec.Environment
	out.Description = persona.Spec.Description
	out.Platform = persona.Status.Platform
	out.LastDiscovery = persona.Status.LastDiscovery

	if out.KubernetesVersion == "" && persona.Status.KubernetesVersion != "" {
		out.KubernetesVersion = persona.Status.KubernetesVersion
		out.KubernetesVersionSource = SourcePersonaStatus
	}

	if counts := persona.Status.Namespaces; counts != nil {
		out.Namespaces = &NamespaceCounts{
			Total:        counts.Total,
			Active:       counts.Active,
			WithPersonas: counts.WithPersonas,
		}
	}
	return out
}

// distinctKubeletVersions lists every kubelet version on the cluster, sorted.
func distinctKubeletVersions(nodes []*corev1.Node) []string {
	return distinctNodeStrings(nodes, func(node *corev1.Node) string {
		return node.Status.NodeInfo.KubeletVersion
	})
}

// distinctArchitectures lists every node architecture, sorted.
func distinctArchitectures(nodes []*corev1.Node) []string {
	return distinctNodeStrings(nodes, func(node *corev1.Node) string {
		return node.Status.NodeInfo.Architecture
	})
}

func distinctNodeStrings(nodes []*corev1.Node, read func(*corev1.Node) string) []string {
	seen := map[string]bool{}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if value := strings.TrimSpace(read(node)); value != "" {
			seen[value] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// projectNodes turns the live Node list into table rows, ordered by name.
//
// Name order rather than by health: this is a fixed inventory that a reader
// scans and returns to, not a feed. A node that goes NotReady must not move,
// because a row that jumps under the cursor is how the wrong node gets read.
func projectNodes(nodes []*corev1.Node, claims podClaims) []Node {
	rows := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		rows = append(rows, projectNode(node, claims.perNode[node.Name]))
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].Name < rows[b].Name })
	return rows
}

func projectNode(node *corev1.Node, claim *nodeClaim) Node {
	allocatable := node.Status.Allocatable
	capacity := node.Status.Capacity

	row := Node{
		ID:               node.Name,
		Name:             node.Name,
		Schedulable:      !node.Spec.Unschedulable,
		Roles:            nodeRoles(node),
		Taints:           nodeTaints(node),
		KubeletVersion:   node.Status.NodeInfo.KubeletVersion,
		ContainerRuntime: node.Status.NodeInfo.ContainerRuntimeVersion,
		OS:               node.Status.NodeInfo.OperatingSystem,
		Architecture:     node.Status.NodeInfo.Architecture,
		Allocatable:      nodeResources(allocatable),
		Capacity:         nodeResources(capacity),
		CreatedAt:        node.CreationTimestamp,
	}
	row.Ready, row.NotReadyReason = nodeReadiness(node)

	podCapacity := allocatable.Pods().Value()
	row.Pods = NodePods{Capacity: podCapacity}
	if claim != nil {
		row.Pods.Scheduled = claim.pods
		row.Pods.Percent = percentOf(int64(claim.pods), podCapacity)
		row.RequestedCPUPercent = percentOf(claim.cpu.MilliValue(), allocatable.Cpu().MilliValue())
		row.RequestedMemoryPercent = percentOf(claim.memory.Value(), allocatable.Memory().Value())
	}
	return row
}

// nodeReadiness reads the Ready condition and, when it is not true, why.
//
// A node with no Ready condition at all reports not ready with that stated. It
// is the honest reading: the kubelet has never checked in, which is what a node
// that has just been created looks like, and defaulting a missing condition to
// ready would put a green tick on a machine nobody has heard from.
func nodeReadiness(node *corev1.Node) (ready bool, notReadyReason string) {
	for i := range node.Status.Conditions {
		condition := node.Status.Conditions[i]
		if condition.Type != corev1.NodeReady {
			continue
		}
		if condition.Status == corev1.ConditionTrue {
			return true, ""
		}
		reason := strings.TrimSpace(condition.Reason)
		message := strings.TrimSpace(condition.Message)
		switch {
		case reason != "" && message != "":
			return false, reason + ": " + message
		case reason != "":
			return false, reason
		case message != "":
			return false, message
		default:
			return false, fmt.Sprintf("the Ready condition is %s and carries no reason", condition.Status)
		}
	}
	return false, "the kubelet has not reported a Ready condition yet"
}

// nodeRoles reads the roles off the node-role.kubernetes.io/* labels.
func nodeRoles(node *corev1.Node) []string {
	var roles []string
	for label := range node.Labels {
		if role, found := strings.CutPrefix(label, nodeRoleLabelPrefix); found && role != "" {
			roles = append(roles, role)
		}
	}
	sort.Strings(roles)
	return roles
}

// nodeTaints renders taints as kubectl writes them, sorted for a stable row.
func nodeTaints(node *corev1.Node) []string {
	var taints []string
	for i := range node.Spec.Taints {
		taint := node.Spec.Taints[i]
		rendered := taint.Key
		if taint.Value != "" {
			rendered += "=" + taint.Value
		}
		if taint.Effect != "" {
			rendered += ":" + string(taint.Effect)
		}
		taints = append(taints, rendered)
	}
	sort.Strings(taints)
	return taints
}

// nodeResources formats one resource pool for display.
func nodeResources(list corev1.ResourceList) NodeResources {
	out := NodeResources{}
	if cpu, ok := list[corev1.ResourceCPU]; ok {
		out.CPU = formatCPU(cpu)
	}
	if memory, ok := list[corev1.ResourceMemory]; ok {
		out.Memory = formatMemory(memory)
	}
	if pods, ok := list[corev1.ResourcePods]; ok {
		out.Pods = pods.String()
	}
	return out
}

// projectAddons renders the operator's add-on discovery, and contradicts it
// where this process knows better.
//
// Add-ons are the one part of this view that has to come from the operator: the
// dashboard does not watch Deployments in every add-on namespace and would be
// guessing. metrics-server is the exception, because asking it for node usage is
// a first-hand check that this process performs every thirty seconds, and a
// record that disagrees with a direct observation is worth saying out loud.
func projectAddons(persona *dorguv1.ClusterPersona, usage store.NodeUsage) []Addon {
	if persona == nil {
		return nil
	}

	rows := make([]Addon, 0, len(persona.Status.Addons))
	for _, addon := range persona.Status.Addons {
		row := Addon{
			Name:      addon.Name,
			Type:      addon.Type,
			Namespace: addon.Namespace,
			Version:   addon.Version,
			Installed: addon.Installed,
			Healthy:   addon.Healthy,
		}
		if addon.Name == metricsServerAddon && addon.Installed && !usage.Available() {
			row.Disagreement = "The operator recorded metrics-server as installed, and this dashboard " +
				"asked it for node usage and got nothing: " + usage.Unavailable +
				". The used figures above are therefore absent."
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].Name < rows[b].Name })
	return rows
}

// summariseCluster counts what the reader is looking at, so the header cannot
// disagree with the rows below it.
func summariseCluster(nodes []Node, claims podClaims, addons []Addon) ClusterSummary {
	summary := ClusterSummary{
		Nodes:           len(nodes),
		ScheduledPods:   claims.scheduled,
		UnscheduledPods: claims.unscheduled,
	}
	for _, node := range nodes {
		if node.Ready {
			summary.NodesReady++
		}
		if !node.Schedulable {
			summary.NodesUnschedulable++
		}
	}
	for _, addon := range addons {
		if !addon.Installed {
			continue
		}
		summary.AddonsInstalled++
		if addon.Healthy != nil && !*addon.Healthy {
			summary.AddonsUnhealthy++
		}
	}
	return summary
}
