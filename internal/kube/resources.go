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

package kube

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// The five dorgu.io resources, by plural name as the API server serves them.
const (
	ResourceApplicationPersonas = "applicationpersonas"
	ResourceClusterPersonas     = "clusterpersonas"
	ResourceIncidentMemories    = "incidentmemories"
	ResourceRemediationActions  = "remediationactions"
	ResourceDorguEvents         = "dorguevents"
)

// DorguResources lists every dorgu.io resource the dashboard watches, in a
// stable order.
var DorguResources = []string{
	ResourceApplicationPersonas,
	ResourceClusterPersonas,
	ResourceIncidentMemories,
	ResourceRemediationActions,
	ResourceDorguEvents,
}

// GroupVersion is the dorgu.io group version, taken from the operator's own
// api/v1 rather than restated, so a future group rename cannot leave the
// dashboard watching a group that no longer exists.
var GroupVersion = dorguv1.GroupVersion

// GVR returns the group-version-resource for a dorgu.io resource.
func GVR(resource string) schema.GroupVersionResource {
	return GroupVersion.WithResource(resource)
}

// ResourceInfo is what discovery reports about one dorgu.io resource.
type ResourceInfo struct {
	// Present reports that the API server serves this resource.
	Present bool
	// Namespaced reports that the resource is namespace-scoped.
	//
	// It is load-bearing, not informational. ClusterPersona is cluster-scoped,
	// and a namespaced LIST against a cluster-scoped resource is answered with
	// "the server could not find the requested resource": the informer then
	// never syncs, and a startup that waits for it never finishes. So the scope
	// is read from the server rather than assumed, which also means a CRD
	// changing scope cannot break this silently.
	Namespaced bool
}

// Resources is the discovery result for the dorgu.io group.
type Resources map[string]ResourceInfo

// Present reports whether a resource is served.
func (r Resources) Present(resource string) bool {
	return r[resource].Present
}

// Namespaced reports whether a served resource is namespace-scoped. An absent
// resource reports false, which is never acted on because nothing watches it.
func (r Resources) Namespaced(resource string) bool {
	return r[resource].Namespaced
}

// PresenceMap flattens to resource-to-present, which is what the store records.
func (r Resources) PresenceMap() map[string]bool {
	out := make(map[string]bool, len(r))
	for name, info := range r {
		out[name] = info.Present
	}
	return out
}

// Missing lists the served-nowhere resources, in DorguResources order.
func (r Resources) Missing() []string {
	var missing []string
	for _, name := range DorguResources {
		if !r[name].Present {
			missing = append(missing, name)
		}
	}
	return missing
}

// DiscoverDorguResources reports which dorgu.io resources the API server serves
// and how each one is scoped.
//
// Presence is the difference between "you have no apps" and "the operator is not
// installed", and the clean-room run showed that conflating the two is how a
// reliability tool presents a blind spot as good news. Starting an informer on an
// absent CRD would instead produce an endless stream of watch errors in the log
// and an empty list in the UI, which is the same lie with more noise.
//
// A discovery call that fails entirely is an error: it means the cluster could
// not be reached, and reporting "no CRDs" for an unreachable cluster would be a
// third wrong answer.
func DiscoverDorguResources(client discovery.DiscoveryInterface) (Resources, error) {
	found := make(Resources, len(DorguResources))
	for _, name := range DorguResources {
		found[name] = ResourceInfo{}
	}

	list, err := client.ServerResourcesForGroupVersion(GroupVersion.String())
	if err != nil {
		if apierrors.IsNotFound(err) || discovery.IsGroupDiscoveryFailedError(err) {
			// The group is not served at all: the operator's CRDs are not
			// installed. That is an answer, not a failure.
			return found, nil
		}
		return nil, fmt.Errorf("discovering %s resources: %w", GroupVersion.String(), err)
	}

	for _, resource := range list.APIResources {
		if _, watched := found[resource.Name]; !watched {
			continue
		}
		found[resource.Name] = ResourceInfo{Present: true, Namespaced: resource.Namespaced}
	}
	return found, nil
}

// MissingResources lists the watched resources that are not served, in
// DorguResources order. It takes the flattened map the store holds.
func MissingResources(present map[string]bool) []string {
	var missing []string
	for _, r := range DorguResources {
		if !present[r] {
			missing = append(missing, r)
		}
	}
	return missing
}
