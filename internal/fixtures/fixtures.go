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

// Package fixtures fills a store with a cluster that needs no cluster.
//
// It exists for one reason: the end-to-end tests and the CI screenshots have to
// drive the real dashboard server, real handlers and real SPA, and a real API
// server is a heavy thing to ask a CI runner for. So the server is built over a
// store that this package populates directly, and everything above the store is
// the production code path.
//
// Every timestamp is relative to the instant Populate is given, so the report
// window always contains the data, and the scenarios are the five reasons a list
// can be empty plus the full cluster.
package fixtures

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/informers"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

// Scenario names what the fixture cluster looks like.
type Scenario string

const (
	// Full is a brownfield cluster with every state a view has to render.
	Full Scenario = "full"
	// Warming is a cluster whose informers have not finished their first list.
	Warming Scenario = "warming"
	// CRDsMissing is a cluster with no dorgu.io resources: the operator is not installed.
	CRDsMissing Scenario = "crds-missing"
	// OperatorIdle has the CRDs and an operator Deployment with no ready replica,
	// so an empty list says nothing about the cluster.
	OperatorIdle Scenario = "operator-idle"
	// None has the CRDs and a running operator that has recorded nothing.
	None Scenario = "none"
	// Refused is a cluster whose operator resources the kubeconfig may not read.
	Refused Scenario = "refused"
)

// Scenarios lists every scenario, in the order the tests visit them.
var Scenarios = []Scenario{Full, Warming, CRDsMissing, OperatorIdle, None, Refused}

// Parse returns the scenario with that name.
func Parse(name string) (Scenario, error) {
	for _, s := range Scenarios {
		if string(s) == name {
			return s, nil
		}
	}
	return "", fmt.Errorf("unknown scenario %q: valid scenarios are %v", name, Scenarios)
}

// Populate fills the store for the scenario, with every time relative to now.
func Populate(st *store.Store, scenario Scenario, now time.Time) {
	allInformers := append([]string{
		informers.NameDeployments, informers.NamePods, informers.NameNodes, informers.NameActionEvents,
	}, kube.DorguResources...)

	switch scenario {
	case Warming:
		for _, r := range kube.DorguResources {
			st.SetCRDPresent(r, true)
		}
		for _, name := range allInformers {
			st.SetSynced(name, false)
		}
		return
	case CRDsMissing:
		for _, r := range kube.DorguResources {
			st.SetCRDPresent(r, false)
		}
	default:
		for _, r := range kube.DorguResources {
			st.SetCRDPresent(r, true)
		}
	}
	for _, name := range allInformers {
		if name == informers.NameDeployments || name == informers.NamePods || name == informers.NameNodes ||
			name == informers.NameActionEvents || st.CRDPresent(name) {
			st.SetSynced(name, true)
		}
	}

	// Nodes and workloads exist in every settled scenario: the Apps and Cluster
	// views work without the operator, which is the point of them.
	for _, n := range nodes(now) {
		st.PutNode(n)
	}
	for _, d := range deployments(now) {
		st.PutDeployment(d)
	}
	for _, p := range pods(now) {
		st.PutPod(p)
	}

	switch scenario {
	case CRDsMissing:
		return
	case Refused:
		for _, name := range []string{kube.ResourceRemediationActions, kube.ResourceIncidentMemories} {
			st.SetSynced(name, false)
			st.SetSyncFailed(name, "the initial list did not complete within 30s, which usually means "+
				"your kubeconfig user may not list or watch this resource")
		}
		return
	case OperatorIdle:
		st.PutDeployment(operator(now, 0))
		return
	case None:
		st.PutDeployment(operator(now, 1))
		for _, p := range personas(now) {
			st.PutAppPersona(p)
		}
		st.PutClusterPersona(clusterPersona(now))
		return
	}

	st.PutDeployment(operator(now, 1))
	for _, p := range personas(now) {
		st.PutAppPersona(p)
	}
	st.PutClusterPersona(clusterPersona(now))
	for _, i := range incidents(now) {
		st.PutIncident(i)
	}
	for _, r := range remediations(now) {
		st.PutRemediation(r)
	}
	for _, e := range actionEvents(now) {
		st.PutActionEvent(e)
	}
}

func ago(now time.Time, d time.Duration) metav1.Time { return metav1.NewTime(now.Add(-d)) }

func ptr[T any](v T) *T { return &v }

func qty(s string) resource.Quantity { return resource.MustParse(s) }

// ---------------------------------------------------------------------------
// Operator, nodes, workloads
// ---------------------------------------------------------------------------

func operator(now time.Time, ready int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "dorgu-system", Name: "dorgu-operator",
			Labels:            map[string]string{"app.kubernetes.io/name": "dorgu-operator"},
			CreationTimestamp: ago(now, 30*24*time.Hour),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(1)),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "manager", Image: "ghcr.io/dorgu-ai/dorgu-operator:v0.13.1",
			}}}},
		},
		Status: appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: ready},
	}
}

func nodes(now time.Time) []*corev1.Node {
	mk := func(name string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, CreationTimestamp: ago(now, 20*24*time.Hour),
				Labels: map[string]string{"node-role.kubernetes.io/worker": ""},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU: qty("2000m"), corev1.ResourceMemory: qty("7Gi"), corev1.ResourcePods: qty("58"),
				},
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU: qty("2000m"), corev1.ResourceMemory: qty("8Gi"), corev1.ResourcePods: qty("58"),
				},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
				NodeInfo: corev1.NodeSystemInfo{
					KubeletVersion: "v1.33.4", Architecture: "arm64", OperatingSystem: "linux",
					ContainerRuntimeVersion: "containerd://2.0.1",
				},
			},
		}
	}
	return []*corev1.Node{mk("ip-10-0-1-11"), mk("ip-10-0-2-27")}
}

type workload struct {
	namespace, name, appLabel, container, image string
	replicas, ready                             int32
	annotations, labels                         map[string]string
	cpuRequest                                  string
}

func workloads() []workload {
	return []workload{
		{
			namespace: "apps", name: "checkout-podinfo", appLabel: "checkout", container: "podinfo",
			image: "ghcr.io/stefanprodan/podinfo:6.7.1", replicas: 3, ready: 3, cpuRequest: "100m",
			annotations: map[string]string{
				"meta.helm.sh/release-name": "checkout", "meta.helm.sh/release-namespace": "apps",
			},
			labels: map[string]string{"app.kubernetes.io/managed-by": "Helm"},
		},
		{
			namespace: "apps", name: "reports", container: "reports",
			image: "example.internal/reports:2.4.0", replicas: 2, ready: 2, cpuRequest: "250m",
		},
		{
			namespace: "apps", name: "frontend-web", appLabel: "frontend", container: "web",
			image: "example.internal/frontend:1.9.2", replicas: 4, ready: 1, cpuRequest: "250m",
			labels: map[string]string{"argocd.argoproj.io/instance": "storefront"},
		},
		{
			namespace: "legacy", name: "legacy-billing", container: "billing",
			image: "example.internal/billing:0.3.1", replicas: 1, ready: 1, cpuRequest: "100m",
		},
		{
			namespace: "kube-system", name: "coredns", container: "coredns",
			image: "registry.k8s.io/coredns/coredns:v1.11.3", replicas: 2, ready: 2, cpuRequest: "100m",
		},
	}
}

func selectorFor(w workload) map[string]string {
	if w.appLabel != "" {
		return map[string]string{"app.kubernetes.io/name": w.appLabel}
	}
	return map[string]string{"app": w.name}
}

func deployments(now time.Time) []*appsv1.Deployment {
	var out []*appsv1.Deployment
	for _, w := range workloads() {
		labels := map[string]string{}
		for k, v := range w.labels {
			labels[k] = v
		}
		if w.appLabel != "" {
			labels["app.kubernetes.io/name"] = w.appLabel
		}
		out = append(out, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: w.namespace, Name: w.name, Labels: labels, Annotations: w.annotations,
				CreationTimestamp: ago(now, 14*24*time.Hour),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr(w.replicas),
				Selector: &metav1.LabelSelector{MatchLabels: selectorFor(w)},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: selectorFor(w)},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: w.container, Image: w.image,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: qty(w.cpuRequest)},
							Limits:   corev1.ResourceList{corev1.ResourceMemory: qty("128Mi")},
						},
					}}},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas: w.replicas, ReadyReplicas: w.ready, AvailableReplicas: w.ready, UpdatedReplicas: w.replicas,
			},
		})
	}
	return out
}

func pods(now time.Time) []*corev1.Pod {
	var out []*corev1.Pod
	nodeNames := []string{"ip-10-0-1-11", "ip-10-0-2-27"}
	n := 0
	for _, w := range workloads() {
		for i := int32(0); i < w.replicas; i++ {
			waiting := w.name == "frontend-web" && i >= w.ready
			status := corev1.ContainerStatus{Name: w.container, Ready: !waiting}
			phase := corev1.PodRunning
			if waiting {
				phase = corev1.PodPending
				status.State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
					Reason: "ImagePullBackOff", Message: "Back-off pulling image example.internal/frontend:1.9.2",
				}}
			} else {
				status.State = corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: ago(now, time.Hour)}}
			}
			out = append(out, &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: w.namespace, Name: fmt.Sprintf("%s-%d", w.name, i), Labels: selectorFor(w),
					CreationTimestamp: ago(now, time.Hour),
				},
				Spec: corev1.PodSpec{
					NodeName: nodeNames[n%2],
					Containers: []corev1.Container{{
						Name: w.container, Image: w.image,
						Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
							corev1.ResourceCPU: qty(w.cpuRequest), corev1.ResourceMemory: qty("128Mi"),
						}},
					}},
				},
				Status: corev1.PodStatus{Phase: phase, ContainerStatuses: []corev1.ContainerStatus{status}},
			})
			n++
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Personas
// ---------------------------------------------------------------------------

func personas(now time.Time) []*dorguv1.ApplicationPersona {
	checked := ago(now, 90*time.Second)
	return []*dorguv1.ApplicationPersona{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout", CreationTimestamp: ago(now, 14*24*time.Hour)},
			Spec: dorguv1.ApplicationPersonaSpec{
				Name: "checkout", Type: "api", Tier: "critical",
				Ownership: &dorguv1.OwnershipSpec{Team: "payments", Owner: "alex", OnCall: "payments-oncall"},
			},
			Status: dorguv1.ApplicationPersonaStatus{
				Phase: "Active", LastUpdated: &checked,
				Health: &dorguv1.HealthStatus{Status: "Healthy", Message: "3 of 3 replicas ready", LastCheck: &checked},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "reports", CreationTimestamp: ago(now, 14*24*time.Hour)},
			Spec:       dorguv1.ApplicationPersonaSpec{Name: "reports", Type: "worker", Tier: "standard"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "frontend", CreationTimestamp: ago(now, 14*24*time.Hour)},
			Spec: dorguv1.ApplicationPersonaSpec{
				Name: "frontend", Type: "web", Tier: "critical", Ownership: &dorguv1.OwnershipSpec{Team: "storefront"},
			},
			Status: dorguv1.ApplicationPersonaStatus{
				Phase: "Degraded", ActiveIncidents: 1,
				Health: &dorguv1.HealthStatus{Status: "Degraded", Message: "1 of 4 replicas ready"},
			},
		},
	}
}

func clusterPersona(now time.Time) *dorguv1.ClusterPersona {
	healthy := true
	discovered := ago(now, 4*time.Minute)
	return &dorguv1.ClusterPersona{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: dorguv1.ClusterPersonaSpec{
			Name: "fixture-cluster", Environment: "development",
			Description: "A brownfield-shaped fixture cluster.",
			Policies: &dorguv1.ClusterPolicies{
				SelfHealing: &dorguv1.SelfHealingPolicy{Mode: dorguv1.SelfHealingModePropose},
			},
		},
		Status: dorguv1.ClusterPersonaStatus{
			Phase: "Ready", LastDiscovery: &discovered, KubernetesVersion: "v1.33.4", Platform: "eks",
			ApplicationCount: 3, Namespaces: &dorguv1.NamespaceSummary{Total: 4, Active: 4, WithPersonas: 1},
			Addons: []dorguv1.AddonInfo{
				{Name: "coredns", Type: "other", Namespace: "kube-system", Installed: true, Version: "v1.11.3", Healthy: &healthy},
				{Name: "cluster-autoscaler", Type: "other", Installed: false},
			},
		},
	}
}
