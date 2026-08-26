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
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// Labels and annotations that identify the system owning a workload.
const (
	// LabelManagedBy is the Kubernetes recommended ownership label. Helm sets it
	// to "Helm" on every release object. kustomize writes it only when asked,
	// and then as a versioned value: see detectKustomize.
	LabelManagedBy = "app.kubernetes.io/managed-by"

	// kustomize build metadata, written only when the kustomization opts in
	// with `buildMetadata: [originAnnotations, transformerAnnotations]`. Both
	// are unambiguous when present: nothing else emits them.
	annotationKustomizeOrigin          = "config.kubernetes.io/origin"
	annotationKustomizeTransformations = "alpha.config.kubernetes.io/transformations"

	// annotationHelmReleaseName and annotationHelmReleaseNamespace are written
	// by Helm 3 on every object in a release.
	annotationHelmReleaseName      = "meta.helm.sh/release-name"
	annotationHelmReleaseNamespace = "meta.helm.sh/release-namespace"

	// labelArgoCDInstance is ArgoCD's legacy tracking label;
	// annotationArgoCDTracking is the newer annotation-based tracking method.
	labelArgoCDInstance      = "argocd.argoproj.io/instance"
	annotationArgoCDTracking = "argocd.argoproj.io/tracking-id"

	// Flux stamps the reconciling Kustomization or HelmRelease onto every object
	// it applies.
	labelFluxKustomizeName = "kustomize.toolkit.fluxcd.io/name"
	labelFluxHelmName      = "helm.toolkit.fluxcd.io/name"
)

// Server-side-apply field managers that identify an owner.
const (
	managerHelm            = "helm"
	managerArgoCD          = "argocd-controller"
	managerArgoCDApp       = "argocd-application-controller"
	managerFluxKustomize   = "kustomize-controller"
	managerFluxHelm        = "helm-controller"
	managerKubectlPrefix   = "kubectl"
	managerDorguPrefix     = "dorgu"
	managerKubeControllers = "kube-controller-manager"
)

// Ownership is the result of asking who owns a workload's desired state.
type Ownership struct {
	// ManagedBy is one of the dorguv1.ManagedBy* values.
	ManagedBy string
	// Detail names the specific owner in prose, e.g.
	// `Helm release "frontend" in namespace apps`. Empty when there is nothing
	// more specific to say than ManagedBy itself.
	Detail string
}

// IsOwned reports whether something other than a human with kubectl reconciles
// this workload. Unknown counts as owned: absence of evidence that patching is
// safe is not evidence that it is.
func (o Ownership) IsOwned() bool {
	return o.ManagedBy != dorguv1.ManagedByUnmanaged
}

// DetectOwner reports which system owns a live Deployment's desired state.
//
// Ported from dorgu-operator internal/workload.DetectOwner. In the operator
// this governs whether the CLI may patch the Deployment; here it is purely
// informational, and the Apps view shows it so a user can see in advance which
// of their apps Dorgu will explain rather than heal.
//
// Evidence is read most-specific first, because several owners layer on top of
// each other: a Flux HelmRelease sets Helm's own labels, so Flux has to win, and
// an ArgoCD-managed Helm chart likewise renders Helm metadata that ArgoCD, not
// Helm, actually reconciles.
func DetectOwner(deploy *appsv1.Deployment) Ownership {
	if deploy == nil {
		return Ownership{ManagedBy: dorguv1.ManagedByUnknown}
	}

	labels := deploy.GetLabels()
	annotations := deploy.GetAnnotations()
	managers := fieldManagers(deploy.GetManagedFields())

	if o, ok := detectArgoCD(labels, annotations, managers); ok {
		return o
	}
	if o, ok := detectFlux(labels, managers); ok {
		return o
	}
	if o, ok := detectHelm(labels, annotations, managers); ok {
		return o
	}
	if o, ok := detectKustomize(labels, annotations); ok {
		return o
	}

	// No declarative owner by name. A field manager Dorgu does not recognise
	// still owns whatever it holds, and a patch would collide with it, so say
	// unknown and name it rather than guess.
	if o, ok := foreignFieldOwner(deploy.GetManagedFields()); ok {
		return o
	}

	// Nothing reconciles this workload: it is kubectl and hands all the way
	// down, which is the only case where patching it is safe.
	return Ownership{ManagedBy: dorguv1.ManagedByUnmanaged}
}

func detectArgoCD(labels, annotations map[string]string, managers map[string]struct{}) (Ownership, bool) {
	if v := annotations[annotationArgoCDTracking]; v != "" {
		return Ownership{
			ManagedBy: dorguv1.ManagedByArgoCD,
			Detail:    fmt.Sprintf("ArgoCD application %q", argoCDAppFromTrackingID(v)),
		}, true
	}
	if v := labels[labelArgoCDInstance]; v != "" {
		return Ownership{
			ManagedBy: dorguv1.ManagedByArgoCD,
			Detail:    fmt.Sprintf("ArgoCD application %q", v),
		}, true
	}
	if hasManager(managers, managerArgoCD, managerArgoCDApp) {
		return Ownership{ManagedBy: dorguv1.ManagedByArgoCD, Detail: "an ArgoCD application"}, true
	}
	return Ownership{}, false
}

func detectFlux(labels map[string]string, managers map[string]struct{}) (Ownership, bool) {
	if v := labels[labelFluxKustomizeName]; v != "" {
		return Ownership{
			ManagedBy: dorguv1.ManagedByFlux,
			Detail:    fmt.Sprintf("Flux Kustomization %q", v),
		}, true
	}
	if v := labels[labelFluxHelmName]; v != "" {
		return Ownership{
			ManagedBy: dorguv1.ManagedByFlux,
			Detail:    fmt.Sprintf("Flux HelmRelease %q", v),
		}, true
	}
	// kustomize-controller and helm-controller are Flux's own reconcilers, so a
	// bare field manager is still Flux even without the labels.
	if hasManager(managers, managerFluxKustomize, managerFluxHelm) {
		return Ownership{ManagedBy: dorguv1.ManagedByFlux, Detail: "a Flux controller"}, true
	}
	return Ownership{}, false
}

func detectHelm(labels, annotations map[string]string, managers map[string]struct{}) (Ownership, bool) {
	release := annotations[annotationHelmReleaseName]
	releaseNS := annotations[annotationHelmReleaseNamespace]
	if release != "" {
		detail := fmt.Sprintf("Helm release %q", release)
		if releaseNS != "" {
			detail = fmt.Sprintf("Helm release %q in namespace %s", release, releaseNS)
		}
		return Ownership{ManagedBy: dorguv1.ManagedByHelm, Detail: detail}, true
	}
	if strings.EqualFold(labels[LabelManagedBy], "helm") {
		return Ownership{ManagedBy: dorguv1.ManagedByHelm, Detail: "a Helm release"}, true
	}
	if hasManager(managers, managerHelm) {
		return Ownership{ManagedBy: dorguv1.ManagedByHelm, Detail: "a Helm release"}, true
	}
	return Ownership{}, false
}

// detectKustomize looks for the markers kustomize actually emits.
//
// There is no marker in the default case, and that is not a gap. kustomize is a
// client-side renderer with no controller: it stamps nothing on its output
// unless the kustomization asks it to, so a Deployment created by
// `kubectl apply -k` is indistinguishable at the API level from one created by
// `kubectl apply -f`. All three markers below are opt-in, and a plain overlay
// reads as unmanaged.
//
// The managed-by value is matched by prefix. kustomize's own
// `buildMetadata: [managedByLabel]` writes `kustomize-v5.8.1`, not `kustomize`.
func detectKustomize(labels, annotations map[string]string) (Ownership, bool) {
	if isKustomizeManagedBy(labels[LabelManagedBy]) {
		return Ownership{ManagedBy: dorguv1.ManagedByKustomize, Detail: "a kustomize overlay"}, true
	}
	for _, annotation := range []string{annotationKustomizeOrigin, annotationKustomizeTransformations} {
		if annotations[annotation] != "" {
			return Ownership{ManagedBy: dorguv1.ManagedByKustomize, Detail: "a kustomize overlay"}, true
		}
	}
	return Ownership{}, false
}

// isKustomizeManagedBy accepts both forms of the label: the bare value a user
// writes by hand, and the versioned `kustomize-v5.8.1` kustomize generates.
func isKustomizeManagedBy(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return lower == "kustomize" || strings.HasPrefix(lower, "kustomize-")
}

// fieldManagers collects the distinct field-manager names on an object,
// lowercased so comparison does not hinge on casing we do not control.
func fieldManagers(entries []metav1.ManagedFieldsEntry) map[string]struct{} {
	out := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.Manager != "" {
			out[strings.ToLower(e.Manager)] = struct{}{}
		}
	}
	return out
}

func hasManager(managers map[string]struct{}, names ...string) bool {
	for _, n := range names {
		if _, ok := managers[n]; ok {
			return true
		}
	}
	return false
}

// foreignFieldOwner reports a field manager whose claim on this Deployment
// means Dorgu is not the only writer, naming it so the UI can say who.
//
// Two kinds of claim count:
//
//   - Any Apply-operation entry. Server-side apply is what reconcilers use, so
//     a foreign applier is a reconciler whatever fields it happens to hold.
//   - An Update-operation entry that owns a container's `resources` block,
//     which is the exact set of fields a Dorgu remediation writes. Apply
//     conflict detection is about who owns the field, not how they came to own
//     it, and it conflicts with Update-operation managers just as readily.
//
// kubectl, Dorgu and kube-controller-manager are excluded from both rules.
// kubectl entries are the user's own hands, which is the definition of
// unmanaged rather than a counter-example to it.
func foreignFieldOwner(entries []metav1.ManagedFieldsEntry) (Ownership, bool) {
	for _, e := range entries {
		if e.Manager == "" || isKnownBenignManager(e.Manager) {
			continue
		}
		switch e.Operation {
		case metav1.ManagedFieldsOperationApply:
			return Ownership{
				ManagedBy: dorguv1.ManagedByUnknown,
				Detail:    fmt.Sprintf("server-side applied by field manager %q", e.Manager),
			}, true
		case metav1.ManagedFieldsOperationUpdate:
			if ownsContainerResources(e) {
				return Ownership{
					ManagedBy: dorguv1.ManagedByUnknown,
					Detail:    fmt.Sprintf("field manager %q already owns this container's resources", e.Manager),
				}, true
			}
		}
	}
	return Ownership{}, false
}

// isKnownBenignManager reports whether a field manager is one whose presence
// says nothing about a workload being reconciled: the user's own kubectl,
// Dorgu, or the Deployment controller.
func isKnownBenignManager(manager string) bool {
	lower := strings.ToLower(manager)
	return strings.HasPrefix(lower, managerKubectlPrefix) ||
		strings.HasPrefix(lower, managerDorguPrefix) ||
		lower == managerKubeControllers
}

// argoCDAppFromTrackingID pulls the application name out of an ArgoCD
// tracking-id annotation, whose form is `<app>:<group>/<kind>:<ns>/<name>`.
// A value that does not parse is returned unchanged rather than dropped.
func argoCDAppFromTrackingID(id string) string {
	if app, _, found := strings.Cut(id, ":"); found && app != "" {
		return app
	}
	return id
}
