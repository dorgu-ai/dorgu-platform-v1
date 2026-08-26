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

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

type ownershipFixture struct {
	labels      map[string]string
	annotations map[string]string
	managed     []metav1.ManagedFieldsEntry
}

func (f ownershipFixture) deployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:          "frontend",
			Namespace:     "apps",
			Labels:        f.labels,
			Annotations:   f.annotations,
			ManagedFields: f.managed,
		},
	}
}

func applyEntry(manager string) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{Manager: manager, Operation: metav1.ManagedFieldsOperationApply}
}

func updateEntry(manager string, fields string) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{
		Manager:   manager,
		Operation: metav1.ManagedFieldsOperationUpdate,
		FieldsV1:  &metav1.FieldsV1{Raw: []byte(fields)},
	}
}

// resourcesFields is a fieldsV1 tree claiming a container's resource block,
// which is the exact set of fields a Dorgu remediation writes.
const resourcesFields = `{"f:spec":{"f:template":{"f:spec":{"f:containers":` +
	`{"k:{\"name\":\"podinfo\"}":{"f:resources":{"f:limits":{"f:memory":{}}}}}}}}}`

// replicasFields claims spec.replicas only, which an autoscaler does and which
// is not in the way of a resource patch.
const replicasFields = `{"f:spec":{"f:replicas":{}}}`

func TestDetectOwner(t *testing.T) {
	tests := []struct {
		name        string
		fixture     ownershipFixture
		wantManaged string
		wantDetail  string
	}{
		{
			name:        "nil-safe: unknown, which every consumer treats as owned",
			wantManaged: dorguv1.ManagedByUnknown,
		},
		{
			name: "argocd tracking annotation names the application",
			fixture: ownershipFixture{annotations: map[string]string{
				"argocd.argoproj.io/tracking-id": "storefront:apps/Deployment:apps/frontend",
			}},
			wantManaged: dorguv1.ManagedByArgoCD,
			wantDetail:  `ArgoCD application "storefront"`,
		},
		{
			name: "argocd instance label",
			fixture: ownershipFixture{labels: map[string]string{
				"argocd.argoproj.io/instance": "storefront",
			}},
			wantManaged: dorguv1.ManagedByArgoCD,
		},
		{
			name:        "argocd field manager alone",
			fixture:     ownershipFixture{managed: []metav1.ManagedFieldsEntry{applyEntry("argocd-application-controller")}},
			wantManaged: dorguv1.ManagedByArgoCD,
		},
		{
			name: "flux wins over the Helm labels its HelmRelease renders",
			fixture: ownershipFixture{
				labels: map[string]string{
					"helm.toolkit.fluxcd.io/name": "frontend",
					LabelManagedBy:                "Helm",
				},
				annotations: map[string]string{"meta.helm.sh/release-name": "frontend"},
			},
			wantManaged: dorguv1.ManagedByFlux,
			wantDetail:  `Flux HelmRelease "frontend"`,
		},
		{
			name: "argocd wins over the Helm metadata it renders",
			fixture: ownershipFixture{
				labels:      map[string]string{"argocd.argoproj.io/instance": "storefront"},
				annotations: map[string]string{"meta.helm.sh/release-name": "frontend"},
			},
			wantManaged: dorguv1.ManagedByArgoCD,
		},
		{
			name: "helm release annotations name the release and namespace",
			fixture: ownershipFixture{annotations: map[string]string{
				"meta.helm.sh/release-name":      "frontend",
				"meta.helm.sh/release-namespace": "apps",
			}},
			wantManaged: dorguv1.ManagedByHelm,
			wantDetail:  `Helm release "frontend" in namespace apps`,
		},
		{
			name:        "helm managed-by label, case insensitive",
			fixture:     ownershipFixture{labels: map[string]string{LabelManagedBy: "helm"}},
			wantManaged: dorguv1.ManagedByHelm,
		},
		{
			name:        "kustomize versioned managed-by label",
			fixture:     ownershipFixture{labels: map[string]string{LabelManagedBy: "kustomize-v5.8.1"}},
			wantManaged: dorguv1.ManagedByKustomize,
		},
		{
			name:        "kustomize bare managed-by label",
			fixture:     ownershipFixture{labels: map[string]string{LabelManagedBy: "kustomize"}},
			wantManaged: dorguv1.ManagedByKustomize,
		},
		{
			name: "kustomize origin annotation",
			fixture: ownershipFixture{annotations: map[string]string{
				"config.kubernetes.io/origin": "path: base/deployment.yaml",
			}},
			wantManaged: dorguv1.ManagedByKustomize,
		},
		{
			name:        "nothing reconciles it: unmanaged, the only patchable case",
			fixture:     ownershipFixture{managed: []metav1.ManagedFieldsEntry{updateEntry("kubectl-client-side-apply", replicasFields)}},
			wantManaged: dorguv1.ManagedByUnmanaged,
		},
		{
			name: "the user's own kubectl-set on resources is still unmanaged",
			fixture: ownershipFixture{managed: []metav1.ManagedFieldsEntry{
				updateEntry("kubectl-set", resourcesFields),
			}},
			wantManaged: dorguv1.ManagedByUnmanaged,
		},
		{
			name: "dorgu's own field manager is not an owner",
			fixture: ownershipFixture{managed: []metav1.ManagedFieldsEntry{
				updateEntry("dorgu-cli", resourcesFields),
			}},
			wantManaged: dorguv1.ManagedByUnmanaged,
		},
		{
			name: "kube-controller-manager is not an owner",
			fixture: ownershipFixture{managed: []metav1.ManagedFieldsEntry{
				updateEntry("kube-controller-manager", replicasFields),
			}},
			wantManaged: dorguv1.ManagedByUnmanaged,
		},
		{
			name:        "an unrecognised server-side applier is unknown, and named",
			fixture:     ownershipFixture{managed: []metav1.ManagedFieldsEntry{applyEntry("some-gitops-tool")}},
			wantManaged: dorguv1.ManagedByUnknown,
			wantDetail:  `server-side applied by field manager "some-gitops-tool"`,
		},
		{
			name: "a foreign updater owning container resources is unknown",
			fixture: ownershipFixture{managed: []metav1.ManagedFieldsEntry{
				updateEntry("some-mutating-webhook", resourcesFields),
			}},
			wantManaged: dorguv1.ManagedByUnknown,
			wantDetail:  `field manager "some-mutating-webhook" already owns this container's resources`,
		},
		{
			name: "a foreign updater owning only replicas is not in the way",
			fixture: ownershipFixture{managed: []metav1.ManagedFieldsEntry{
				updateEntry("horizontal-pod-autoscaler", replicasFields),
			}},
			wantManaged: dorguv1.ManagedByUnmanaged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var deploy *appsv1.Deployment
			if tt.name != "nil-safe: unknown, which every consumer treats as owned" {
				deploy = tt.fixture.deployment()
			}

			got := DetectOwner(deploy)
			assert.Equal(t, tt.wantManaged, got.ManagedBy)
			if tt.wantDetail != "" {
				assert.Equal(t, tt.wantDetail, got.Detail)
			}
			assert.Equal(t, tt.wantManaged != dorguv1.ManagedByUnmanaged, got.IsOwned(),
				"unknown must count as owned")
		})
	}
}

// An unreadable managedFields entry is treated as owning. Absence of evidence
// that patching is safe is not evidence that it is.
func TestUnparseableFieldsV1CountsAsOwning(t *testing.T) {
	deploy := ownershipFixture{managed: []metav1.ManagedFieldsEntry{
		updateEntry("mystery-controller", "{not json"),
	}}.deployment()

	got := DetectOwner(deploy)
	assert.Equal(t, dorguv1.ManagedByUnknown, got.ManagedBy)
	assert.True(t, got.IsOwned())
}

func TestArgoCDTrackingIDThatDoesNotParseIsKeptWhole(t *testing.T) {
	deploy := ownershipFixture{annotations: map[string]string{
		"argocd.argoproj.io/tracking-id": "no-colons-here",
	}}.deployment()

	assert.Equal(t, `ArgoCD application "no-colons-here"`, DetectOwner(deploy).Detail)
}

// This is the invariant the CLI's heal path depends on. If it ever flips, Dorgu
// starts patching Deployments that Helm or ArgoCD will fight it over.
func TestOnlyUnmanagedIsNotOwned(t *testing.T) {
	for _, value := range []string{
		dorguv1.ManagedByHelm,
		dorguv1.ManagedByArgoCD,
		dorguv1.ManagedByFlux,
		dorguv1.ManagedByKustomize,
		dorguv1.ManagedByUnknown,
	} {
		assert.True(t, Ownership{ManagedBy: value}.IsOwned(), value)
	}
	assert.False(t, Ownership{ManagedBy: dorguv1.ManagedByUnmanaged}.IsOwned())
}
