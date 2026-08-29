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
	// Limitation states what a view that does render still does not do.
	//
	// It exists because "available" and "finished" are not the same thing, and
	// the gap is the part a reader has to be told about. A screen that shows a
	// remediation plan and cannot approve it is trustworthy as long as it says
	// so; the same screen with the gap left implicit reads as a broken approve
	// button somebody forgot to wire.
	Limitation string `json:"limitation,omitempty"`
}

// views is the four-view plan and, for each, what this build does and does not
// do with it.
//
// # Both gates have lifted, and this is what lifted them
//
// Remediations was gated because AI-planned remediations were unappliable:
// clean-room run #4 measured nine of them and zero that could change a workload.
// Operator v0.11.0 fixed that at the source. A plan diagnosing a resource change
// now either carries an appliable patch or Dorgu supplies one from the rule
// engine's own calculation, and a plan it cannot make appliable is refused so the
// rule-based proposal takes over, recorded as planSource: rule-based. The same
// release added spec.steps[].safety, so the guardrail verdict arrives as
// structured data instead of a prefix spliced onto the model's prose.
//
// Cluster was gated because cluster health reported 1689% CPU by summing the
// requests of pods no node had accepted. Operator v0.11.0 fixed the field and
// CLI v0.12.0 stopped depending on it. This view goes further and computes
// saturation itself from the live Node and Pod lists, because the field is
// written on a reconcile interval by whatever operator version is installed and
// a card is read as authoritative in a way terminal output is not.
//
// # What is still not here
//
// The approve action. The dashboard stays read-only until that is a deliberate
// decision rather than a consequence of shipping a view, so the Remediations
// screen shows the plan and offers the CLI command that can act on it.
var views = []ViewStatus{
	{ID: "apps", Label: "Apps", Available: true},
	{ID: "incidents", Label: "Incidents", Available: true},
	{
		ID:        "remediations",
		Label:     "Remediations",
		Available: true,
		Limitation: "Read-only. This screen shows a plan, its guardrail verdicts and its diff, " +
			"and cannot approve or apply anything: the dashboard writes nothing to your cluster " +
			"in this release. Approve with the CLI, which prints the same plan first.",
	},
	{
		ID:        "cluster",
		Label:     "Cluster",
		Available: true,
		Limitation: "Saturation is computed here from your live Nodes and Pods rather than read " +
			"from the operator, so it does not depend on the operator version installed. " +
			"Used figures need metrics-server and say so when it does not answer.",
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
