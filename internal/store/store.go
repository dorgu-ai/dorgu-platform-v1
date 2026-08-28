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

// Package store is the in-memory cache the HTTP layer reads.
//
// Informers write here; handlers read here. Nothing in the HTTP layer talks to
// the Kubernetes API server, so a page render costs no API calls no matter how
// many browser tabs are open.
//
// # Ownership of the objects inside
//
// Objects handed to the store by an informer belong to the informer's shared
// cache and must never be mutated: every other consumer in the process sees
// the same pointer. The store therefore deep-copies on the way in and hands
// out copies on the way out, so a caller can neither corrupt the informer cache
// nor observe a row changing under it mid-render.
package store

import (
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// Key identifies a namespaced object.
type Key struct {
	Namespace string
	Name      string
}

// Store holds the latest observed state of every resource the dashboard reads.
//
// It is safe for concurrent use. All mutation goes through the Put/Delete
// methods, which notify the change callback with the affected topics.
type Store struct {
	mu sync.RWMutex

	appPersonas     map[Key]*dorguv1.ApplicationPersona
	clusterPersonas map[Key]*dorguv1.ClusterPersona
	incidents       map[Key]*dorguv1.IncidentMemory
	remediations    map[Key]*dorguv1.RemediationAction
	dorguEvents     map[Key]*dorguv1.DorguEvent
	deployments     map[Key]*appsv1.Deployment
	pods            map[Key]*corev1.Pod
	// nodes is keyed like everything else even though a Node is cluster-scoped,
	// so its namespace is always empty. One key type beats a second one that
	// exists only to omit a field.
	nodes map[Key]*corev1.Node

	// nodeUsage is the last answer from metrics-server, or the last reason it
	// gave none. It is the one piece of state here that is polled rather than
	// watched, because the metrics API serves no watch verb; see
	// internal/metrics for why that is the API's constraint and not a choice.
	nodeUsage NodeUsage

	// crdsPresent records which dorgu.io kinds the API server actually serves.
	// A missing CRD and an empty list are different facts and the UI states
	// them differently: "the operator is not installed" is not "you have no
	// apps".
	crdsPresent map[string]bool

	// synced records whether each informer has completed its initial list.
	// Until it has, an empty view means "still loading", and saying "no apps"
	// would be a lie with a green tick on it.
	synced map[string]bool

	// syncFailed records why an informer gave up on its initial list, which is
	// a third state the two above cannot express.
	//
	// Without it, a watch the API server refuses leaves synced=false forever
	// and the view shows a loading skeleton that never resolves: the screen
	// says "reading your cluster" about a read that already failed. The most
	// likely cause on a real cluster is RBAC, and a namespace-scoped kubeconfig
	// cannot list Nodes at all, so the Cluster view hits this by design rather
	// than by accident.
	syncFailed map[string]string

	// notify is called after every committed change, outside the lock.
	notify func(Topic)
}

// New returns an empty store. notify is invoked once per changed topic after
// each mutation commits; a nil notify is allowed and makes the store inert,
// which is what tests want.
func New(notify func(Topic)) *Store {
	if notify == nil {
		notify = func(Topic) {}
	}
	return &Store{
		appPersonas:     map[Key]*dorguv1.ApplicationPersona{},
		clusterPersonas: map[Key]*dorguv1.ClusterPersona{},
		incidents:       map[Key]*dorguv1.IncidentMemory{},
		remediations:    map[Key]*dorguv1.RemediationAction{},
		dorguEvents:     map[Key]*dorguv1.DorguEvent{},
		deployments:     map[Key]*appsv1.Deployment{},
		pods:            map[Key]*corev1.Pod{},
		nodes:           map[Key]*corev1.Node{},
		crdsPresent:     map[string]bool{},
		synced:          map[string]bool{},
		syncFailed:      map[string]string{},
		notify:          notify,
	}
}

// ---------------------------------------------------------------------------
// ApplicationPersona
// ---------------------------------------------------------------------------

// PutAppPersona stores a copy of the persona.
func (s *Store) PutAppPersona(obj *dorguv1.ApplicationPersona) {
	if obj == nil {
		return
	}
	s.write(func() {
		s.appPersonas[keyOf(obj.Namespace, obj.Name)] = obj.DeepCopy()
	}, TopicApps)
}

// DeleteAppPersona removes a persona.
func (s *Store) DeleteAppPersona(namespace, name string) {
	s.write(func() { delete(s.appPersonas, keyOf(namespace, name)) }, TopicApps)
}

// AppPersonas returns copies of every stored persona.
func (s *Store) AppPersonas() []*dorguv1.ApplicationPersona {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*dorguv1.ApplicationPersona, 0, len(s.appPersonas))
	for _, p := range s.appPersonas {
		out = append(out, p.DeepCopy())
	}
	return out
}

// ---------------------------------------------------------------------------
// ClusterPersona
// ---------------------------------------------------------------------------

// PutClusterPersona stores a copy of the cluster persona.
func (s *Store) PutClusterPersona(obj *dorguv1.ClusterPersona) {
	if obj == nil {
		return
	}
	s.write(func() {
		s.clusterPersonas[keyOf(obj.Namespace, obj.Name)] = obj.DeepCopy()
	}, TopicCluster)
}

// DeleteClusterPersona removes a cluster persona.
func (s *Store) DeleteClusterPersona(namespace, name string) {
	s.write(func() { delete(s.clusterPersonas, keyOf(namespace, name)) }, TopicCluster)
}

// ClusterPersonas returns copies of every stored cluster persona.
func (s *Store) ClusterPersonas() []*dorguv1.ClusterPersona {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*dorguv1.ClusterPersona, 0, len(s.clusterPersonas))
	for _, p := range s.clusterPersonas {
		out = append(out, p.DeepCopy())
	}
	return out
}

// ---------------------------------------------------------------------------
// IncidentMemory
// ---------------------------------------------------------------------------

// PutIncident stores a copy of the incident. It touches the Apps topic too:
// an app row shows its open incident count, so a new incident changes both
// views at once.
func (s *Store) PutIncident(obj *dorguv1.IncidentMemory) {
	if obj == nil {
		return
	}
	s.write(func() {
		s.incidents[keyOf(obj.Namespace, obj.Name)] = obj.DeepCopy()
	}, TopicIncidents, TopicApps)
}

// DeleteIncident removes an incident.
func (s *Store) DeleteIncident(namespace, name string) {
	s.write(func() { delete(s.incidents, keyOf(namespace, name)) }, TopicIncidents, TopicApps)
}

// Incidents returns copies of every stored incident.
func (s *Store) Incidents() []*dorguv1.IncidentMemory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*dorguv1.IncidentMemory, 0, len(s.incidents))
	for _, i := range s.incidents {
		out = append(out, i.DeepCopy())
	}
	return out
}

// ---------------------------------------------------------------------------
// RemediationAction
// ---------------------------------------------------------------------------

// PutRemediation stores a copy of the remediation.
func (s *Store) PutRemediation(obj *dorguv1.RemediationAction) {
	if obj == nil {
		return
	}
	s.write(func() {
		s.remediations[keyOf(obj.Namespace, obj.Name)] = obj.DeepCopy()
	}, TopicRemediations)
}

// DeleteRemediation removes a remediation.
func (s *Store) DeleteRemediation(namespace, name string) {
	s.write(func() { delete(s.remediations, keyOf(namespace, name)) }, TopicRemediations)
}

// Remediations returns copies of every stored remediation.
func (s *Store) Remediations() []*dorguv1.RemediationAction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*dorguv1.RemediationAction, 0, len(s.remediations))
	for _, r := range s.remediations {
		out = append(out, r.DeepCopy())
	}
	return out
}

// ---------------------------------------------------------------------------
// DorguEvent
// ---------------------------------------------------------------------------

// PutDorguEvent stores a copy of the event.
func (s *Store) PutDorguEvent(obj *dorguv1.DorguEvent) {
	if obj == nil {
		return
	}
	s.write(func() {
		s.dorguEvents[keyOf(obj.Namespace, obj.Name)] = obj.DeepCopy()
	}, TopicEvents)
}

// DeleteDorguEvent removes an event.
func (s *Store) DeleteDorguEvent(namespace, name string) {
	s.write(func() { delete(s.dorguEvents, keyOf(namespace, name)) }, TopicEvents)
}

// DorguEvents returns copies of every stored event.
func (s *Store) DorguEvents() []*dorguv1.DorguEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*dorguv1.DorguEvent, 0, len(s.dorguEvents))
	for _, e := range s.dorguEvents {
		out = append(out, e.DeepCopy())
	}
	return out
}

// ---------------------------------------------------------------------------
// Deployment
// ---------------------------------------------------------------------------

// PutDeployment stores a copy of the Deployment.
func (s *Store) PutDeployment(obj *appsv1.Deployment) {
	if obj == nil {
		return
	}
	s.write(func() {
		s.deployments[keyOf(obj.Namespace, obj.Name)] = obj.DeepCopy()
	}, TopicApps)
}

// DeleteDeployment removes a Deployment.
func (s *Store) DeleteDeployment(namespace, name string) {
	s.write(func() { delete(s.deployments, keyOf(namespace, name)) }, TopicApps)
}

// Deployments returns copies of every stored Deployment.
func (s *Store) Deployments() []*appsv1.Deployment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*appsv1.Deployment, 0, len(s.deployments))
	for _, d := range s.deployments {
		out = append(out, d.DeepCopy())
	}
	return out
}

// ---------------------------------------------------------------------------
// Pod
// ---------------------------------------------------------------------------

// PutPod stores a copy of the Pod. It touches the Cluster topic as well as
// Apps: saturation is the sum of what the scheduled Pods have claimed, so a Pod
// arriving or being scheduled changes the Cluster view too.
func (s *Store) PutPod(obj *corev1.Pod) {
	if obj == nil {
		return
	}
	s.write(func() {
		s.pods[keyOf(obj.Namespace, obj.Name)] = obj.DeepCopy()
	}, TopicApps, TopicCluster)
}

// DeletePod removes a Pod.
func (s *Store) DeletePod(namespace, name string) {
	s.write(func() { delete(s.pods, keyOf(namespace, name)) }, TopicApps, TopicCluster)
}

// Pods returns copies of every stored Pod.
func (s *Store) Pods() []*corev1.Pod {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*corev1.Pod, 0, len(s.pods))
	for _, p := range s.pods {
		out = append(out, p.DeepCopy())
	}
	return out
}

// ---------------------------------------------------------------------------
// Server-side facts
// ---------------------------------------------------------------------------

// SetCRDPresent records whether the API server serves a dorgu.io resource.
func (s *Store) SetCRDPresent(resource string, present bool) {
	s.write(func() { s.crdsPresent[resource] = present }, allTopicsForReadiness...)
}

// CRDPresent reports whether a dorgu.io resource is served. An unrecorded
// resource reports false, which is the safe reading: we have no evidence the
// operator's CRDs are installed.
func (s *Store) CRDPresent(resource string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.crdsPresent[resource]
}

// SetSynced records that a named informer finished its initial list. It clears
// any recorded failure: an informer that syncs on a retry is no longer failed,
// and leaving the reason behind would leave a permanent warning on a view that
// is now correct.
func (s *Store) SetSynced(name string, synced bool) {
	s.write(func() {
		s.synced[name] = synced
		if synced {
			delete(s.syncFailed, name)
		}
	}, allTopicsForReadiness...)
}

// SetSyncFailed records that an informer gave up on its initial list, and why.
//
// The reason is shown to the user rather than only logged. A view that cannot
// be read has to say so: "Dorgu could not list Nodes" is a fact the reader can
// act on, and an endless loading skeleton is not.
func (s *Store) SetSyncFailed(name, reason string) {
	s.write(func() { s.syncFailed[name] = reason }, allTopicsForReadiness...)
}

// SyncFailure returns why an informer gave up, or "" if it did not.
func (s *Store) SyncFailure(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.syncFailed[name]
}

// allTopicsForReadiness are the topics whose payload carries a readiness block,
// so a change in what the caches know republishes every view that reports it.
var allTopicsForReadiness = []Topic{
	TopicMeta, TopicApps, TopicIncidents, TopicRemediations, TopicCluster,
}

// Synced reports whether a named informer finished its initial list.
func (s *Store) Synced(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.synced[name]
}

// SyncedAll reports whether every informer that was started has synced. It is
// false when nothing has been registered, because "no informers" is not a
// synced cache.
func (s *Store) SyncedAll() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.synced) == 0 {
		return false
	}
	for _, ok := range s.synced {
		if !ok {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

// write applies mutate under the write lock, then notifies the topics outside
// it. Notifying outside the lock matters: notify fans out to SSE clients, and
// holding the store lock across that would let one slow reader stall every
// informer.
func (s *Store) write(mutate func(), topics ...Topic) {
	s.mu.Lock()
	mutate()
	s.mu.Unlock()

	for _, t := range topics {
		s.notify(t)
	}
}

func keyOf(namespace, name string) Key {
	return Key{Namespace: namespace, Name: name}
}
