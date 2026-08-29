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
	"github.com/dorgu-ai/dorgu-platform-v1/internal/informers"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/view"
)

// SystemNamespaces are excluded when scanning for unmonitored Deployments in
// the all-namespaces view.
//
// The list matches the CLI's, which exists because burying three real apps under
// forty control-plane Deployments recreates the blind spot that scan is there to
// fix. An explicit namespace scope overrides the exclusion, because asking for a
// namespace is asking for all of it.
var SystemNamespaces = map[string]bool{
	"kube-system":          true,
	"kube-public":          true,
	"kube-node-lease":      true,
	"local-path-storage":   true,
	"dorgu-system":         true,
	"gatekeeper-system":    true,
	"kubernetes-dashboard": true,
}

// SnapshotOptions configures how payloads are projected.
type SnapshotOptions struct {
	// Namespace scopes every view. Empty means all namespaces.
	//
	// The Cluster view is the one exception and it ignores this: saturation is a
	// property of the whole cluster, and summing one namespace's requests
	// against every node's allocatable would produce a figure that is wrong in a
	// way nobody could detect from the screen.
	Namespace string
	// IncidentLimit caps the incident feed. Zero uses
	// view.DefaultIncidentLimit; negative means no cap.
	IncidentLimit int
	// RemediationLimit caps the remediation list. Zero uses
	// view.DefaultRemediationLimit; negative means no cap.
	RemediationLimit int
	// ExcludedNamespaces overrides SystemNamespaces when non-nil.
	ExcludedNamespaces map[string]bool
}

// Snapshots builds view payloads from the cache.
//
// Both the REST read and the SSE push go through here, which is what guarantees
// they cannot drift: a live update is byte-for-byte the payload a fresh GET
// would return.
type Snapshots struct {
	store *store.Store
	opts  SnapshotOptions
}

// NewSnapshots returns a snapshot builder over the given cache.
func NewSnapshots(cache *store.Store, opts SnapshotOptions) *Snapshots {
	if opts.ExcludedNamespaces == nil {
		opts.ExcludedNamespaces = SystemNamespaces
	}
	return &Snapshots{store: cache, opts: opts}
}

// Apps projects the Apps view.
func (s *Snapshots) Apps() view.AppsPayload {
	return view.BuildApps(view.AppsInput{
		Personas:           s.store.AppPersonas(),
		Deployments:        s.store.Deployments(),
		Pods:               s.store.Pods(),
		Incidents:          s.store.Incidents(),
		Namespace:          s.opts.Namespace,
		ExcludedNamespaces: s.opts.ExcludedNamespaces,
		Readiness:          s.appsReadiness(),
	})
}

// Incidents projects the Incidents view.
func (s *Snapshots) Incidents() view.IncidentsPayload {
	return view.BuildIncidents(view.IncidentsInput{
		Incidents: s.store.Incidents(),
		Personas:  s.store.AppPersonas(),
		Namespace: s.opts.Namespace,
		Limit:     s.opts.IncidentLimit,
		Readiness: s.incidentsReadiness(),
	})
}

// Remediations projects the Remediations view.
func (s *Snapshots) Remediations() view.RemediationsPayload {
	return view.BuildRemediations(view.RemediationsInput{
		Remediations: s.store.Remediations(),
		Personas:     s.store.AppPersonas(),
		Incidents:    s.store.Incidents(),
		Namespace:    s.opts.Namespace,
		Limit:        s.opts.RemediationLimit,
		Readiness:    s.remediationsReadiness(),
	})
}

// Cluster projects the Cluster view.
func (s *Snapshots) Cluster() view.ClusterPayload {
	return view.BuildCluster(view.ClusterInput{
		Nodes:     s.store.Nodes(),
		Pods:      s.store.Pods(),
		Personas:  s.store.ClusterPersonas(),
		NodeUsage: s.store.NodeUsage(),
		Readiness: s.clusterReadiness(),
	})
}

// appsReadiness states what the Apps payload knows.
//
// The Apps view has a property the Incidents view does not: it is useful with no
// dorgu.io CRDs installed at all, because every Deployment is then unmonitored
// and the whole screen becomes the onboarding prompt. So a missing
// ApplicationPersona CRD is reported, not fatal.
func (s *Snapshots) appsReadiness() view.Readiness {
	needed := []string{kube.ResourceApplicationPersonas}
	return view.Readiness{
		Synced: s.store.Synced(informers.NameDeployments) &&
			s.store.Synced(informers.NamePods) &&
			s.syncedIfPresent(needed...),
		CRDsInstalled: s.store.CRDPresent(kube.ResourceApplicationPersonas),
		MissingCRDs:   s.missing(needed...),
		Unavailable: s.unavailable(
			informers.NameDeployments, informers.NamePods, kube.ResourceApplicationPersonas),
	}
}

// remediationsReadiness states what the Remediations payload knows.
//
// Without the RemediationAction CRD there is nothing to show and nothing to
// imply, so the view says the operator is not installed rather than "no
// remediations". The two are different facts and only one of them is about the
// user's cluster being healthy.
func (s *Snapshots) remediationsReadiness() view.Readiness {
	needed := []string{kube.ResourceRemediationActions}
	return view.Readiness{
		Synced:        s.syncedIfPresent(needed...),
		CRDsInstalled: s.store.CRDPresent(kube.ResourceRemediationActions),
		MissingCRDs:   s.missing(needed...),
		Unavailable:   s.unavailable(needed...),
	}
}

// clusterReadiness states what the Cluster payload knows.
//
// The Cluster view shares the Apps view's property: it is useful with no
// dorgu.io CRDs installed at all, because nodes, capacity and saturation are all
// computed from live objects. A missing ClusterPersona CRD costs the cluster
// name, the environment and the add-on list, and is reported rather than fatal.
//
// Nodes are the one resource this view cannot do without, and the one a
// namespace-scoped kubeconfig cannot read, which is why Unavailable matters more
// here than anywhere else.
func (s *Snapshots) clusterReadiness() view.Readiness {
	needed := []string{kube.ResourceClusterPersonas}
	return view.Readiness{
		Synced: s.store.Synced(informers.NameNodes) &&
			s.store.Synced(informers.NamePods) &&
			s.syncedIfPresent(needed...),
		CRDsInstalled: s.store.CRDPresent(kube.ResourceClusterPersonas),
		MissingCRDs:   s.missing(needed...),
		Unavailable: s.unavailable(
			informers.NameNodes, informers.NamePods, kube.ResourceClusterPersonas),
	}
}

// incidentsReadiness states what the Incidents payload knows. Without the
// IncidentMemory CRD there is nothing to show and nothing to imply, and the UI
// says the operator is not installed rather than "no incidents".
func (s *Snapshots) incidentsReadiness() view.Readiness {
	needed := []string{kube.ResourceIncidentMemories}
	return view.Readiness{
		Synced:        s.syncedIfPresent(needed...),
		CRDsInstalled: s.store.CRDPresent(kube.ResourceIncidentMemories),
		MissingCRDs:   s.missing(needed...),
		Unavailable:   s.unavailable(needed...),
	}
}

// syncedIfPresent reports sync for the resources that exist. A CRD that is not
// installed has no informer, so demanding its sync would hold every view at
// "loading" forever on a cluster without the operator.
func (s *Snapshots) syncedIfPresent(resources ...string) bool {
	for _, r := range resources {
		if s.store.CRDPresent(r) && !s.store.Synced(r) {
			return false
		}
	}
	return true
}

func (s *Snapshots) missing(resources ...string) []string {
	var out []string
	for _, r := range resources {
		if !s.store.CRDPresent(r) {
			out = append(out, r)
		}
	}
	return out
}

// incidentsReadiness and the rest report a CRD that is not installed. unavailable
// reports the different case: an informer that was started and gave up.
//
// The two are kept apart because they call for different screens. "Install the
// operator" is the answer to the first and would be actively misleading for the
// second, where the resource is right there and the read was refused.
func (s *Snapshots) unavailable(names ...string) []view.UnavailableResource {
	var out []view.UnavailableResource
	for _, name := range names {
		if reason := s.store.SyncFailure(name); reason != "" {
			out = append(out, view.UnavailableResource{Resource: name, Reason: reason})
		}
	}
	return out
}
