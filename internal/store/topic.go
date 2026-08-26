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

package store

// Topic names a view whose contents changed. The store reports topics rather
// than individual object deltas because every view is a projection over several
// resource kinds at once: an app row folds a persona, its Deployment, its Pods
// and its incident count into one row, so "a Pod changed" is only ever
// actionable as "the Apps view changed".
type Topic string

const (
	// TopicApps covers the Apps view: ApplicationPersonas, Deployments, Pods.
	TopicApps Topic = "apps"
	// TopicIncidents covers the Incidents view: IncidentMemories.
	TopicIncidents Topic = "incidents"
	// TopicRemediations covers RemediationActions. Stored and streamed from M1
	// so the M3 view is a frontend change only; no view consumes it yet.
	TopicRemediations Topic = "remediations"
	// TopicEvents covers DorguEvents.
	TopicEvents Topic = "events"
	// TopicCluster covers ClusterPersonas. Gated on CF6-2 (CR-03); stored, not
	// rendered.
	TopicCluster Topic = "cluster"
	// TopicMeta covers server-side facts that are not cluster objects: which
	// CRDs are installed, whether the informers have synced.
	TopicMeta Topic = "meta"
)

// AllTopics is every topic a client can be pushed, in a stable order.
var AllTopics = []Topic{
	TopicApps,
	TopicIncidents,
	TopicRemediations,
	TopicEvents,
	TopicCluster,
	TopicMeta,
}
