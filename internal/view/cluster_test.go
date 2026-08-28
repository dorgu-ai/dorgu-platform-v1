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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

func clusterPersona(name string, mutate ...func(*dorguv1.ClusterPersona)) *dorguv1.ClusterPersona {
	p := &dorguv1.ClusterPersona{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: dorguv1.ClusterPersonaSpec{
			Name:        "dorgu-dev",
			Environment: "development",
			Description: "The spike cluster.",
		},
		Status: dorguv1.ClusterPersonaStatus{
			Platform:          "eks",
			KubernetesVersion: "v1.32.0",
			LastDiscovery:     &metav1.Time{Time: baseTime},
			Namespaces:        &dorguv1.NamespaceSummary{Total: 9, Active: 9, WithPersonas: 3},
		},
	}
	for _, m := range mutate {
		m(p)
	}
	return p
}

// ---------------------------------------------------------------------------
// Provenance: every figure says where it came from
// ---------------------------------------------------------------------------

// The kubelet versions read off the live nodes win over the operator's recorded
// one. Both are real answers, but the observed one cannot be stale and cannot
// come from an operator version with a bug in it. This is the same judgement the
// Apps view makes when an observation overrides a persona's verdict.
func TestTheObservedKubernetesVersionWinsOverThePersonasRecord(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes:     []*corev1.Node{node("node-a", "1", "1Gi")},
		Personas:  []*dorguv1.ClusterPersona{clusterPersona("dorgu-dev")},
		NodeUsage: store.UnavailableNodeUsage("no metrics-server"),
	})

	assert.Equal(t, "v1.33.4", got.Identity.KubernetesVersion, "the kubelet, not the persona")
	assert.Equal(t, SourceObserved, got.Identity.KubernetesVersionSource)
}

// With no nodes to read, the operator's record is the only answer there is, and
// it is labelled as such rather than presented as an observation.
func TestThePersonasVersionIsUsedWhenThereAreNoNodesToRead(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Personas:  []*dorguv1.ClusterPersona{clusterPersona("dorgu-dev")},
		NodeUsage: store.UnavailableNodeUsage("no metrics-server"),
	})

	assert.Equal(t, "v1.32.0", got.Identity.KubernetesVersion)
	assert.Equal(t, SourcePersonaStatus, got.Identity.KubernetesVersionSource)
}

func TestNoVersionAnywhereReportsNoSource(t *testing.T) {
	got := BuildCluster(ClusterInput{NodeUsage: store.UnavailableNodeUsage("none")})
	assert.Empty(t, got.Identity.KubernetesVersion)
	assert.Equal(t, SourceNone, got.Identity.KubernetesVersionSource)
	assert.False(t, got.Identity.PersonaPresent)
}

// A mid-upgrade cluster is exactly when somebody opens this screen, so a
// disagreement between nodes is stated rather than collapsed to whichever node
// sorted first.
func TestMixedKubeletVersionsAreListedRatherThanCollapsed(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes: []*corev1.Node{
			node("node-a", "1", "1Gi"),
			node("node-b", "1", "1Gi", func(n *corev1.Node) {
				n.Status.NodeInfo.KubeletVersion = "v1.32.7"
			}),
		},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})

	assert.Equal(t, []string{"v1.32.7", "v1.33.4"}, got.Identity.KubeletVersions)
}

// One agreed version is not a disagreement, so it is not listed twice.
func TestASingleAgreedVersionIsNotListedSeparately(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes:     []*corev1.Node{node("node-a", "1", "1Gi"), node("node-b", "1", "1Gi")},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})
	assert.Nil(t, got.Identity.KubeletVersions)
	assert.Equal(t, "v1.33.4", got.Identity.KubernetesVersion)
}

// Every operator image before v0.11.1 was amd64-only, so an arm64 node could not
// pull it. A reader debugging that needs their node architectures without
// reaching for kubectl.
func TestNodeArchitecturesAreReported(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes: []*corev1.Node{
			node("node-a", "1", "1Gi"),
			node("node-b", "1", "1Gi", func(n *corev1.Node) {
				n.Status.NodeInfo.Architecture = "amd64"
			}),
		},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})
	assert.Equal(t, []string{"amd64", "arm64"}, got.Identity.Architectures)
}

// Namespaces are not watched, so the count is the operator's or absent. A nil
// pointer is the encoding: zero namespaces is impossible, so a zero-valued
// struct would be a claim nobody could make.
func TestNamespaceCountsAreAbsentWithoutAPersona(t *testing.T) {
	withPersona := BuildCluster(ClusterInput{
		Personas:  []*dorguv1.ClusterPersona{clusterPersona("dorgu-dev")},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})
	require.NotNil(t, withPersona.Identity.Namespaces)
	assert.Equal(t, int32(9), withPersona.Identity.Namespaces.Total)

	without := BuildCluster(ClusterInput{NodeUsage: store.UnavailableNodeUsage("none")})
	assert.Nil(t, without.Identity.Namespaces)
}

// A cluster with two ClusterPersonas is a misconfiguration, but rendering it
// non-deterministically would make that look like a flickering dashboard.
func TestTheRenderedClusterPersonaIsChosenDeterministically(t *testing.T) {
	personas := []*dorguv1.ClusterPersona{
		clusterPersona("zeta", func(p *dorguv1.ClusterPersona) { p.Spec.Name = "zeta-cluster" }),
		clusterPersona("alpha", func(p *dorguv1.ClusterPersona) { p.Spec.Name = "alpha-cluster" }),
	}
	for range 5 {
		got := BuildCluster(ClusterInput{
			Personas:  personas,
			NodeUsage: store.UnavailableNodeUsage("none"),
		})
		assert.Equal(t, "alpha-cluster", got.Identity.Name)
	}
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

func TestNodeRowsCarryRolesTaintsAndTheirPools(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes: []*corev1.Node{node("node-a", "2000m", "4Gi", func(n *corev1.Node) {
			n.Labels = map[string]string{
				"node-role.kubernetes.io/worker":        "",
				"node-role.kubernetes.io/control-plane": "",
				"topology.kubernetes.io/zone":           "eu-west-1a",
			}
			n.Spec.Taints = []corev1.Taint{
				{Key: "dedicated", Value: "batch", Effect: corev1.TaintEffectNoSchedule},
			}
		})},
		Pods:      []*corev1.Pod{claimingPod("checkout", "node-a", "500m", "1Gi")},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})

	require.Len(t, got.Nodes, 1)
	row := got.Nodes[0]
	assert.Equal(t, "node-a", row.ID)
	assert.True(t, row.Ready)
	assert.True(t, row.Schedulable)
	assert.Equal(t, []string{"control-plane", "worker"}, row.Roles,
		"roles come off the node-role labels, sorted for a stable row")
	assert.Equal(t, []string{"dedicated=batch:NoSchedule"}, row.Taints)
	assert.Equal(t, "2000m", row.Allocatable.CPU)
	assert.Equal(t, "4.0Gi", row.Allocatable.Memory)
	assert.Equal(t, "arm64", row.Architecture)

	// Per-node saturation, because a cluster at 40% with one node at 98% is the
	// shape that stops a rollout.
	assert.InDelta(t, 25.0, row.RequestedCPUPercent, 0.1)
	assert.InDelta(t, 25.0, row.RequestedMemoryPercent, 0.1)
	assert.Equal(t, 1, row.Pods.Scheduled)
	assert.Equal(t, int64(110), row.Pods.Capacity)
}

// A cordoned node is healthy and deliberately closed. Folding it into Ready
// would report a maintenance window as a fault.
func TestACordonedNodeIsUnschedulableRatherThanNotReady(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes: []*corev1.Node{node("node-a", "1", "1Gi", func(n *corev1.Node) {
			n.Spec.Unschedulable = true
		})},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})

	require.Len(t, got.Nodes, 1)
	assert.True(t, got.Nodes[0].Ready)
	assert.False(t, got.Nodes[0].Schedulable)
	assert.Equal(t, 1, got.Summary.NodesUnschedulable)
	assert.Equal(t, 1, got.Summary.NodesReady)
}

// The row says why, not only that.
func TestANotReadyNodeCarriesTheConditionsOwnReason(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes: []*corev1.Node{node("node-a", "1", "1Gi", func(n *corev1.Node) {
			n.Status.Conditions = []corev1.NodeCondition{{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionFalse,
				Reason:  "KubeletNotReady",
				Message: "container runtime network not ready",
			}}
		})},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})

	require.Len(t, got.Nodes, 1)
	assert.False(t, got.Nodes[0].Ready)
	assert.Equal(t, "KubeletNotReady: container runtime network not ready",
		got.Nodes[0].NotReadyReason)
	assert.Equal(t, 0, got.Summary.NodesReady)
}

// A node with no Ready condition has never been heard from. Defaulting a missing
// condition to ready would put a green tick on a machine nobody has contacted.
func TestANodeWithNoReadyConditionIsNotReportedAsReady(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes: []*corev1.Node{node("node-a", "1", "1Gi", func(n *corev1.Node) {
			n.Status.Conditions = nil
		})},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})
	assert.False(t, got.Nodes[0].Ready)
	assert.Contains(t, got.Nodes[0].NotReadyReason, "has not reported")
}

// A fixed inventory a reader scans and returns to, not a feed. A node that goes
// NotReady must not move, because a row that jumps under the cursor is how the
// wrong node gets read.
func TestNodesAreOrderedByNameNotByHealth(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes: []*corev1.Node{
			node("node-c", "1", "1Gi"),
			node("node-a", "1", "1Gi", func(n *corev1.Node) {
				n.Status.Conditions = []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
				}
			}),
			node("node-b", "1", "1Gi"),
		},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})

	require.Len(t, got.Nodes, 3)
	assert.Equal(t, []string{"node-a", "node-b", "node-c"},
		[]string{got.Nodes[0].Name, got.Nodes[1].Name, got.Nodes[2].Name})
}

// ---------------------------------------------------------------------------
// Add-ons
// ---------------------------------------------------------------------------

// The one add-on this process can check first-hand. A record that disagrees with
// a direct observation is worth saying out loud, which is the same judgement the
// Apps view makes about a stale Healthy over a live crash loop.
func TestMetricsServerRecordedAsInstalledButSilentIsContradicted(t *testing.T) {
	persona := clusterPersona("dorgu-dev", func(p *dorguv1.ClusterPersona) {
		healthy := true
		p.Status.Addons = []dorguv1.AddonInfo{
			{Name: "metrics-server", Installed: true, Namespace: "kube-system", Healthy: &healthy},
			{Name: "coredns", Installed: true, Namespace: "kube-system", Healthy: &healthy},
		}
	})

	got := BuildCluster(ClusterInput{
		Personas:  []*dorguv1.ClusterPersona{persona},
		NodeUsage: store.UnavailableNodeUsage("metrics-server did not answer: 503"),
	})

	require.Len(t, got.Addons, 2)
	metricsServer := findAddon(t, got.Addons, "metrics-server")
	assert.Contains(t, metricsServer.Disagreement, "asked it for node usage and got nothing")
	assert.Contains(t, metricsServer.Disagreement, "503", "the reason travels with the disagreement")

	assert.Empty(t, findAddon(t, got.Addons, "coredns").Disagreement,
		"nothing here observes coredns first-hand, so there is nothing to contradict")
}

// When metrics-server does answer there is no disagreement to report.
func TestNoDisagreementWhenMetricsServerAnswers(t *testing.T) {
	persona := clusterPersona("dorgu-dev", func(p *dorguv1.ClusterPersona) {
		p.Status.Addons = []dorguv1.AddonInfo{{Name: "metrics-server", Installed: true}}
	})

	got := BuildCluster(ClusterInput{
		Personas:  []*dorguv1.ClusterPersona{persona},
		NodeUsage: usage("100m", "1Gi"),
	})
	assert.Empty(t, findAddon(t, got.Addons, "metrics-server").Disagreement)
}

// Add-ons are the operator's discovery. Without a persona there is no list, and
// the absence is not "you have no add-ons".
func TestNoPersonaMeansNoAddonList(t *testing.T) {
	got := BuildCluster(ClusterInput{NodeUsage: store.UnavailableNodeUsage("none")})
	assert.Nil(t, got.Addons)
	assert.Equal(t, 0, got.Summary.AddonsInstalled)
}

func TestUnhealthyAddonsAreCounted(t *testing.T) {
	unhealthy := false
	persona := clusterPersona("dorgu-dev", func(p *dorguv1.ClusterPersona) {
		p.Status.Addons = []dorguv1.AddonInfo{
			{Name: "coredns", Installed: true, Healthy: &unhealthy},
			{Name: "cluster-autoscaler", Installed: false},
		}
	})

	got := BuildCluster(ClusterInput{
		Personas:  []*dorguv1.ClusterPersona{persona},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})
	assert.Equal(t, 1, got.Summary.AddonsInstalled, "an absent add-on is not an installed one")
	assert.Equal(t, 1, got.Summary.AddonsUnhealthy)
}

// ---------------------------------------------------------------------------
// Scope
// ---------------------------------------------------------------------------

// Saturation is a property of the whole cluster. Summing one namespace's requests
// against every node's allocatable would produce a figure that is wrong in a way
// nobody could detect from the screen, so this view has no namespace filter at
// all and the input carries no namespace to honour.
func TestSaturationCoversEveryNamespace(t *testing.T) {
	pods := []*corev1.Pod{
		claimingPod("in-apps", "node-a", "500m", "512Mi"),
		claimingPod("in-other", "node-a", "500m", "512Mi", func(p *corev1.Pod) {
			p.Namespace = "other"
		}),
	}

	got := BuildCluster(ClusterInput{
		Nodes:     []*corev1.Node{node("node-a", "2000m", "4Gi")},
		Pods:      pods,
		NodeUsage: store.UnavailableNodeUsage("none"),
	})

	assert.Equal(t, 2, got.Saturation.ScheduledPods)
	require.NotNil(t, got.Saturation.CPU)
	assert.Equal(t, "1000m", got.Saturation.CPU.Requested)
}

func TestSummaryCountsAgreeWithTheRows(t *testing.T) {
	got := BuildCluster(ClusterInput{
		Nodes: []*corev1.Node{node("node-a", "1", "1Gi"), node("node-b", "1", "1Gi")},
		Pods: []*corev1.Pod{
			claimingPod("a", "node-a", "10m", "10Mi"),
			claimingPod("queued", "", "10m", "10Mi"),
		},
		NodeUsage: store.UnavailableNodeUsage("none"),
	})

	assert.Equal(t, 2, got.Summary.Nodes)
	assert.Equal(t, len(got.Nodes), got.Summary.Nodes)
	assert.Equal(t, 1, got.Summary.ScheduledPods)
	assert.Equal(t, 1, got.Summary.UnscheduledPods)
	assert.Equal(t, got.Saturation.UnscheduledPods, got.Summary.UnscheduledPods)
}

func findAddon(t *testing.T, addons []Addon, name string) Addon {
	t.Helper()
	for _, addon := range addons {
		if addon.Name == name {
			return addon
		}
	}
	t.Fatalf("no add-on named %q", name)
	return Addon{}
}
