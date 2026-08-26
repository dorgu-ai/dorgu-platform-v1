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

package main

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// runtimeConfig is an alias so main.go reads as prose rather than as client-go.
type runtimeConfig = rest.Config

// workloadFixture is one Deployment plus the Pod status a kubelet would have
// written. envtest runs no kubelet, so the fixture supplies it.
type workloadFixture struct {
	namespace string
	name      string
	// appLabel goes on the Deployment's own labels when set, which is the rung
	// the match chain prefers. Left empty on purpose for the workloads that
	// should resolve by selector or by name instead.
	appLabel    string
	container   string
	image       string
	replicas    int32
	ready       int32
	cpuRequest  string
	memoryLimit string
	// annotations carry the ownership evidence: Helm, ArgoCD, Flux or nothing.
	annotations map[string]string
	labels      map[string]string
	// fieldManager is the server-side-apply manager name recorded on the
	// object. It matters more than it looks: the typed Go client always
	// serialises `resources: {}` even for a container that sets none, so
	// whatever name creates these Deployments ends up owning the one field a
	// Dorgu remediation writes. A real cluster records
	// `kubectl-client-side-apply` there, which ownership detection treats as a
	// human with kubectl; leaving it as this program's own name would make
	// every fixture read as managedBy=unknown for a reason no real user hits.
	fieldManager string
}

// managerKubectl is what a real `kubectl apply -f` records.
const managerKubectl = "kubectl-client-side-apply"

// fixtureWorkloads is a brownfield cluster in four Deployments.
//
// Each one exercises a path the views have to get right:
//
//   - checkout: Helm-owned, persona name differs from the Deployment name, so it
//     resolves by label and reports managedBy=helm (Dorgu explains, not patches)
//   - reports: unmanaged, resolves by metadata.name, has a memory limit and no
//     CPU limit, which is the shape a remediation is allowed to touch
//   - frontend: ArgoCD-owned and degraded, so the Owner column has a second
//     non-patchable value and the health column a middle state
//   - legacy-billing: no persona at all, in its own namespace, which is the
//     unmonitored row the Apps view exists for
//   - coredns: in kube-system, which must not appear at all
func fixtureWorkloads() []workloadFixture {
	return []workloadFixture{
		{
			namespace:   "apps",
			name:        "checkout-podinfo",
			appLabel:    "checkout",
			container:   "podinfo",
			image:       "ghcr.io/stefanprodan/podinfo:6.7.1",
			replicas:    3,
			ready:       3,
			cpuRequest:  "100m",
			memoryLimit: "128Mi",
			annotations: map[string]string{
				"meta.helm.sh/release-name":      "checkout",
				"meta.helm.sh/release-namespace": "apps",
			},
			labels:       map[string]string{"app.kubernetes.io/managed-by": "Helm"},
			fieldManager: "helm",
		},
		{
			namespace:    "apps",
			name:         "reports",
			container:    "reports",
			image:        "example.internal/reports:2.4.0",
			replicas:     2,
			ready:        2,
			memoryLimit:  "256Mi",
			fieldManager: managerKubectl,
		},
		{
			namespace:    "apps",
			name:         "frontend-web",
			appLabel:     "frontend",
			container:    "web",
			image:        "example.internal/frontend:1.9.2",
			replicas:     4,
			ready:        1,
			cpuRequest:   "250m",
			labels:       map[string]string{"argocd.argoproj.io/instance": "storefront"},
			fieldManager: "argocd-application-controller",
		},
		{
			namespace:    "legacy",
			name:         "legacy-billing",
			container:    "billing",
			image:        "example.internal/billing:0.3.1",
			replicas:     1,
			ready:        1,
			fieldManager: managerKubectl,
		},
		{
			namespace:    "kube-system",
			name:         "coredns",
			container:    "coredns",
			image:        "registry.k8s.io/coredns/coredns:v1.11.3",
			replicas:     2,
			ready:        2,
			fieldManager: managerKubectl,
		},
	}
}

func createWorkload(ctx context.Context, typed kubernetes.Interface, spec workloadFixture) error {
	selector := map[string]string{"app.kubernetes.io/name": spec.appLabel}
	if spec.appLabel == "" {
		selector = map[string]string{"app": spec.name}
	}

	labels := map[string]string{}
	for k, v := range spec.labels {
		labels[k] = v
	}
	if spec.appLabel != "" {
		labels["app.kubernetes.io/name"] = spec.appLabel
	}

	container := corev1.Container{Name: spec.container, Image: spec.image}
	if spec.cpuRequest != "" {
		container.Resources.Requests = corev1.ResourceList{corev1.ResourceCPU: quantity(spec.cpuRequest)}
	}
	if spec.memoryLimit != "" {
		container.Resources.Limits = corev1.ResourceList{corev1.ResourceMemory: quantity(spec.memoryLimit)}
	}

	replicas := spec.replicas
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   spec.namespace,
			Name:        spec.name,
			Labels:      labels,
			Annotations: spec.annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{container}},
			},
		},
	}

	manager := spec.fieldManager
	if manager == "" {
		manager = managerKubectl
	}

	created, err := typed.AppsV1().Deployments(spec.namespace).
		Create(ctx, deployment, metav1.CreateOptions{FieldManager: manager})
	if err != nil {
		return fmt.Errorf("creating Deployment %s/%s: %w", spec.namespace, spec.name, err)
	}

	// There is no Deployment controller in envtest, so the status a reader sees
	// has to be written here.
	created.Status = appsv1.DeploymentStatus{
		Replicas:          spec.replicas,
		ReadyReplicas:     spec.ready,
		AvailableReplicas: spec.ready,
		UpdatedReplicas:   spec.replicas,
	}
	if _, err := typed.AppsV1().Deployments(spec.namespace).
		UpdateStatus(ctx, created, metav1.UpdateOptions{FieldManager: manager}); err != nil {
		return fmt.Errorf("setting status on %s/%s: %w", spec.namespace, spec.name, err)
	}

	// One Pod per ready replica, plus one not-ready if the Deployment is short,
	// so the pod-issue path has something to read.
	for i := int32(0); i < spec.replicas; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: spec.namespace,
				Name:      fmt.Sprintf("%s-%d", spec.name, i),
				Labels:    selector,
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:  spec.container,
				Image: spec.image,
			}}},
		}
		createdPod, err := typed.CoreV1().Pods(spec.namespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating pod for %s/%s: %w", spec.namespace, spec.name, err)
		}

		if i < spec.ready {
			createdPod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  spec.container,
					Ready: true,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				}},
			}
		} else {
			createdPod.Status = corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: spec.container,
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason:  "ImagePullBackOff",
						Message: fmt.Sprintf("Back-off pulling image %q", spec.image),
					}},
				}},
			}
		}
		if _, err := typed.CoreV1().Pods(spec.namespace).
			UpdateStatus(ctx, createdPod, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("setting pod status for %s/%s: %w", spec.namespace, spec.name, err)
		}
	}

	return nil
}

// fixturePersonas covers the three states an app row can be in: healthy with an
// operator verdict, healthy with no verdict at all (so the dashboard falls back
// to its own observation), and a persona whose Deployment cannot be found.
func fixturePersonas() []*dorguv1.ApplicationPersona {
	checked := metav1.NewTime(time.Now().Add(-90 * time.Second))
	critical := "critical"

	return []*dorguv1.ApplicationPersona{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout"},
			Spec: dorguv1.ApplicationPersonaSpec{
				Name: "checkout",
				Type: "api",
				Tier: critical,
				Ownership: &dorguv1.OwnershipSpec{
					Team:   "payments",
					Owner:  "alex",
					OnCall: "payments-oncall",
				},
			},
			Status: dorguv1.ApplicationPersonaStatus{
				Phase:       "Active",
				LastUpdated: &checked,
				Health: &dorguv1.HealthStatus{
					Status:    "Healthy",
					Message:   "3 of 3 replicas ready",
					LastCheck: &checked,
				},
			},
		},
		{
			// No status at all: the operator has not reconciled this one, which
			// is what makes the dashboard's observed-health fallback visible.
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "reports"},
			Spec: dorguv1.ApplicationPersonaSpec{
				Name: "reports",
				Type: "worker",
				Tier: "standard",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "frontend"},
			Spec: dorguv1.ApplicationPersonaSpec{
				Name:      "frontend",
				Type:      "web",
				Tier:      critical,
				Ownership: &dorguv1.OwnershipSpec{Team: "storefront"},
			},
			Status: dorguv1.ApplicationPersonaStatus{
				Phase: "Degraded",
				Health: &dorguv1.HealthStatus{
					Status:  "Degraded",
					Message: "1 of 4 replicas ready",
				},
				ActiveIncidents: 1,
			},
		},
	}
}

// fixtureIncidents covers a diagnosed AI incident, an undiagnosed one, and an
// unattributed one, which are the three shapes the Incidents view has to tell
// apart.
func fixtureIncidents() []*dorguv1.IncidentMemory {
	now := metav1.NewTime(time.Now())
	recent := metav1.NewTime(time.Now().Add(-4 * time.Minute))
	older := metav1.NewTime(time.Now().Add(-3 * time.Hour))

	return []*dorguv1.IncidentMemory{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "frontend-imagepull"},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: dorguv1.PersonaReference{
					Kind: "ApplicationPersona", Name: "frontend", Namespace: "apps",
				},
				Attribution: "persona",
				Category:    "deployment",
				Severity:    "critical",
				Detection: dorguv1.DetectionInfo{
					Signal:    "ImagePullBackOff",
					Source:    "pod-failure-detector",
					FirstSeen: recent,
					LastSeen:  now,
					AffectedResources: []dorguv1.ResourceReference{
						{Kind: "Pod", Name: "frontend-web-1", Namespace: "apps", Role: "affected"},
					},
				},
				RootCause: &dorguv1.RootCauseInfo{
					Summary:    "Image example.internal/frontend:1.9.2 cannot be pulled; three of four replicas never started",
					Confidence: "0.88",
					Provider:   "ai-enhanced",
					Contributing: []dorguv1.ContributingSignal{
						{Signal: "ImagePullBackOff", Detail: "3 pods waiting on the same tag"},
						{Signal: "ReplicaSetProgressing", Detail: "rollout stalled for 4 minutes"},
					},
				},
			},
			Status: dorguv1.IncidentMemoryStatus{Phase: "Investigating", OccurrenceCount: 3},
		},
		{
			// Detected and not diagnosed, which is the normal state when AI
			// diagnosis has not been enabled.
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "reports-restarts"},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: dorguv1.PersonaReference{
					Kind: "ApplicationPersona", Name: "reports", Namespace: "apps",
				},
				Attribution: "persona",
				Category:    "resource",
				Severity:    "warning",
				Detection: dorguv1.DetectionInfo{
					Signal:    "ContainerRestarted",
					Source:    "pod-failure-detector",
					FirstSeen: older,
					LastSeen:  recent,
					// affectedResources is required by the CRD, not optional.
					AffectedResources: []dorguv1.ResourceReference{
						{Kind: "Pod", Name: "reports-0", Namespace: "apps", Role: "affected"},
					},
				},
			},
			Status: dorguv1.IncidentMemoryStatus{Phase: "Detected", OccurrenceCount: 1},
		},
		{
			// No persona claimed these signals, so the incident is filed against
			// the workload itself. Dorgu can see it and cannot act on it.
			ObjectMeta: metav1.ObjectMeta{Namespace: "legacy", Name: "legacy-billing-oom"},
			Spec: dorguv1.IncidentMemorySpec{
				PersonaRef: dorguv1.PersonaReference{
					Kind: "ApplicationPersona", Name: "legacy-billing", Namespace: "legacy",
				},
				Attribution: "unattributed",
				Category:    "resource",
				Severity:    "warning",
				Detection: dorguv1.DetectionInfo{
					Signal:    "OOMKilled",
					Source:    "pod-failure-detector",
					FirstSeen: older,
					LastSeen:  older,
					AffectedResources: []dorguv1.ResourceReference{
						{Kind: "Pod", Name: "legacy-billing-0", Namespace: "legacy", Role: "affected"},
					},
				},
				RootCause: &dorguv1.RootCauseInfo{
					Summary:    "Container exceeded its memory limit",
					Confidence: "0.62",
					Provider:   "rule-engine",
				},
			},
			Status: dorguv1.IncidentMemoryStatus{Phase: "Detected", OccurrenceCount: 12},
		},
	}
}
