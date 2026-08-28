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

package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/informers"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/view"
)

// ---------------------------------------------------------------------------
// Remediations
// ---------------------------------------------------------------------------

func TestRemediationsEndpointServesThePlanAndItsVerdicts(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	h.store.PutRemediation(&dorguv1.RemediationAction{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "fix-oom-checkout"},
		Spec: dorguv1.RemediationActionSpec{
			PersonaRef: dorguv1.PersonaReference{Name: "checkout", Namespace: "apps"},
			Confidence: "0.85",
			Steps: []dorguv1.RemediationStep{{
				Order: 1, ID: "raise-memory", Type: dorguv1.StepTypePersonaUpdate,
				Risk: "low", AutoExecutable: true,
				Description: "Raise the memory limit to 512Mi.",
				Patch: &apiextensionsv1.JSON{
					Raw: []byte(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
				},
				Safety: []dorguv1.StepSafety{{
					Rule: dorguv1.SafetyRuleBlastRadius, Verdict: dorguv1.SafetyVerdictClamped,
					Field: "spec.resources.limits.memory", Requested: "4Gi", Permitted: "512Mi",
					Message: "Blast-radius guardrail.",
				}},
			}},
		},
	})

	rec := h.get(t, "/api/v1/remediations")
	require.Equal(t, http.StatusOK, rec.Code)

	var payload view.RemediationsPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Remediations, 1)

	row := payload.Remediations[0]
	assert.Equal(t, "apps/fix-oom-checkout", row.ID)
	require.Len(t, row.Steps, 1)
	require.Len(t, row.Steps[0].Safety, 1)
	assert.Equal(t, "4Gi", row.Steps[0].Safety[0].Requested,
		"the verdict survives the JSON round trip, since it is the only record of the refusal")
	assert.Equal(t, dorguv1.SafetyVerdictClamped, row.StrongestVerdict)
	assert.Equal(t, "dorgu remediation diff fix-oom-checkout -n apps", row.DiffCommand)
	assert.True(t, payload.Readiness.Synced)
	assert.True(t, payload.Readiness.CRDsInstalled)
}

// Without the CRD the view says the operator is not installed rather than "no
// remediations". They are different facts and only one is about the cluster
// being healthy.
func TestRemediationsWithoutTheCRDReportsTheOperatorMissing(t *testing.T) {
	h := newHarness(t, nil)
	h.store.SetCRDPresent(kube.ResourceRemediationActions, false)

	var payload view.RemediationsPayload
	require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/remediations").Body.Bytes(), &payload))
	assert.False(t, payload.Readiness.CRDsInstalled)
	assert.Contains(t, payload.Readiness.MissingCRDs, kube.ResourceRemediationActions)
	assert.Empty(t, payload.Remediations)
}

// ---------------------------------------------------------------------------
// Cluster
// ---------------------------------------------------------------------------

func TestClusterEndpointServesSaturationComputedFromLiveObjects(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()

	h.store.PutNode(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.33.4", Architecture: "arm64"},
		},
	})
	h.store.PutPod(scheduledPod("apps", "checkout", "node-a", "500m"))
	h.store.PutPod(scheduledPod("apps", "queued", "", "8"))

	rec := h.get(t, "/api/v1/cluster")
	require.Equal(t, http.StatusOK, rec.Code)

	var payload view.ClusterPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))

	require.NotNil(t, payload.Saturation.CPU)
	assert.Equal(t, "2000m", payload.Saturation.CPU.Allocatable)
	assert.Equal(t, "500m", payload.Saturation.CPU.Requested)
	assert.InDelta(t, 25.0, payload.Saturation.CPU.RequestedPercent, 0.1)
	assert.Equal(t, 1, payload.Saturation.UnscheduledPods,
		"the pod nobody can place is excluded and reported")

	assert.Equal(t, "v1.33.4", payload.Identity.KubernetesVersion)
	require.Len(t, payload.Nodes, 1)
	assert.Equal(t, "arm64", payload.Nodes[0].Architecture)
}

// The ordinary case on every managed Kubernetes offering. Used has to render as
// n/a with a reason, never as zero.
func TestClusterEndpointReportsWhyUsedIsAbsent(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	h.store.PutNode(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
		},
	})
	h.store.SetNodeUsage(store.UnavailableNodeUsage(
		"metrics-server did not answer: the server could not find the requested resource"))

	var payload view.ClusterPayload
	require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/cluster").Body.Bytes(), &payload))

	require.NotNil(t, payload.Saturation.CPU)
	assert.Nil(t, payload.Saturation.CPU.UsedPercent, "an unmeasured figure must not arrive as 0")
	assert.Contains(t, payload.Saturation.UsedUnavailable, "metrics-server did not answer")
}

// The Cluster view works on a cluster with no operator at all: nodes, capacity
// and saturation are all computed from live objects.
func TestClusterViewIsUsefulWithNoOperatorInstalled(t *testing.T) {
	h := newHarness(t, nil)
	for _, name := range []string{informers.NameNodes, informers.NamePods} {
		h.store.SetSynced(name, true)
	}
	h.store.PutNode(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
		},
	})

	var payload view.ClusterPayload
	require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/cluster").Body.Bytes(), &payload))

	assert.False(t, payload.Readiness.CRDsInstalled)
	assert.True(t, payload.Readiness.Synced, "nodes and pods synced, so this view is ready")
	assert.Len(t, payload.Nodes, 1, "a missing ClusterPersona CRD costs the name, not the nodes")
	assert.False(t, payload.Identity.PersonaPresent)
	require.NotNil(t, payload.Saturation.CPU)
}

// ---------------------------------------------------------------------------
// A refused read is not a loading state
// ---------------------------------------------------------------------------

// The fifth cause of an empty view, and the one the other four cannot express:
// the resource exists, the operator is installed, and the read was refused. A
// namespace-scoped kubeconfig cannot list Nodes, so the Cluster view reaches this
// by design.
func TestARefusedNodeListIsReportedRatherThanLeftLoading(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	h.store.SetSynced(informers.NameNodes, false)
	h.store.SetSyncFailed(informers.NameNodes,
		"the initial list did not complete within 30s, which usually means your kubeconfig user "+
			"may not list or watch this resource")

	var payload view.ClusterPayload
	require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/cluster").Body.Bytes(), &payload))

	assert.False(t, payload.Readiness.Synced)
	require.Len(t, payload.Readiness.Unavailable, 1)
	assert.Equal(t, informers.NameNodes, payload.Readiness.Unavailable[0].Resource)
	assert.Contains(t, payload.Readiness.Unavailable[0].Reason, "may not list or watch")
}

func scheduledPod(namespace, name, nodeName, cpu string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{{
				Name: "app",
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse(cpu),
				}},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}
