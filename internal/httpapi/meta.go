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
	"net/http"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
)

// Meta is what the dashboard says about itself.
//
// It exists so a reader can answer "which cluster is this and can I trust what
// it is showing me" without leaving the page: the context name, whether the
// operator's CRDs are installed, and whether the caches are warm.
type Meta struct {
	Version string      `json:"version"`
	Cluster ClusterInfo `json:"cluster"`

	// CRDs reports each dorgu.io resource and whether the API server serves it.
	CRDs map[string]bool `json:"crds"`
	// MissingCRDs names the ones that are not installed, in a stable order, so
	// the UI can print a sentence rather than iterate a map.
	MissingCRDs []string `json:"missingCRDs,omitempty"`
	// OperatorInstalled is true when every dorgu.io CRD is served. It is the
	// one-line answer to "is Dorgu actually running here".
	OperatorInstalled bool `json:"operatorInstalled"`

	// Synced reports whether every informer has completed its initial list.
	Synced bool `json:"synced"`
	// StreamClients is how many SSE clients are connected. Shown because a
	// dashboard claiming to be live should be able to prove the stream is up.
	StreamClients int `json:"streamClients"`

	// Views lists what this build renders and what is deliberately not built
	// yet. Stating the gap is the same instinct as the self-healing doc's
	// admission about unenforced trust levels: a reader who can see what is
	// missing trusts what is present.
	Views []ViewStatus `json:"views"`
}

// ViewStatus is one view and its state in this build.
type ViewStatus struct {
	// ID is the route segment, e.g. "apps".
	ID string `json:"id"`
	// Label is the display name.
	Label string `json:"label"`
	// Available reports whether this build renders the view.
	Available bool `json:"available"`
	// Reason explains an unavailable view in one sentence.
	Reason string `json:"reason,omitempty"`
}

// views is the fixed four-view plan plus the honest reason each unbuilt one is
// unbuilt. Both gates are recorded in the dashboard plan.
var views = []ViewStatus{
	{ID: "apps", Label: "Apps", Available: true},
	{ID: "incidents", Label: "Incidents", Available: true},
	{
		ID:    "remediations",
		Label: "Remediations",
		Reason: "Not built yet. AI-planned remediations are not reliably applicable, " +
			"so showing a plan here would imply an action the product cannot take.",
	},
	{
		ID:    "cluster",
		Label: "Cluster",
		Reason: "Not built yet. Cluster health can report impossible CPU figures, " +
			"and a wrong number in a card looks authoritative in a way terminal output does not.",
	},
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, s.cfg.Logger, http.StatusOK, s.meta())
}

func (s *Server) meta() Meta {
	crds := make(map[string]bool, len(kube.DorguResources))
	installed := true
	for _, resource := range kube.DorguResources {
		present := s.cfg.Store.CRDPresent(resource)
		crds[resource] = present
		if !present {
			installed = false
		}
	}

	return Meta{
		Version:           s.cfg.Version,
		Cluster:           s.cfg.Cluster,
		CRDs:              crds,
		MissingCRDs:       kube.MissingResources(crds),
		OperatorInstalled: installed,
		Synced:            s.cfg.Store.SyncedAll(),
		StreamClients:     s.cfg.Broker.Subscribers(),
		Views:             views,
	}
}
