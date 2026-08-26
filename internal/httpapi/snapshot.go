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
	Namespace string
	// IncidentLimit caps the incident feed. Zero uses
	// view.DefaultIncidentLimit; negative means no cap.
	IncidentLimit int
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
