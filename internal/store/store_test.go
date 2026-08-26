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

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// recorder collects the topics a store reported, safely under concurrency.
type recorder struct {
	mu     sync.Mutex
	topics []Topic
}

func (r *recorder) notify(topic Topic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.topics = append(r.topics, topic)
}

func (r *recorder) seen() []Topic {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Topic, len(r.topics))
	copy(out, r.topics)
	return out
}

func TestNilNotifyIsAllowed(t *testing.T) {
	s := New(nil)
	assert.NotPanics(t, func() {
		s.PutDeployment(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "apps"}})
	})
}

func TestPutAndDeleteRoundTrip(t *testing.T) {
	s := New(nil)

	s.PutAppPersona(&dorguv1.ApplicationPersona{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout"},
	})
	require.Len(t, s.AppPersonas(), 1)
	s.DeleteAppPersona("apps", "checkout")
	assert.Empty(t, s.AppPersonas())

	s.PutIncident(&dorguv1.IncidentMemory{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "oom"},
	})
	require.Len(t, s.Incidents(), 1)
	s.DeleteIncident("apps", "oom")
	assert.Empty(t, s.Incidents())

	s.PutRemediation(&dorguv1.RemediationAction{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "fix"},
	})
	require.Len(t, s.Remediations(), 1)
	s.DeleteRemediation("apps", "fix")
	assert.Empty(t, s.Remediations())

	s.PutDorguEvent(&dorguv1.DorguEvent{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "ev"},
	})
	require.Len(t, s.DorguEvents(), 1)
	s.DeleteDorguEvent("apps", "ev")
	assert.Empty(t, s.DorguEvents())

	s.PutClusterPersona(&dorguv1.ClusterPersona{ObjectMeta: metav1.ObjectMeta{Name: "soul"}})
	require.Len(t, s.ClusterPersonas(), 1)
	s.DeleteClusterPersona("", "soul")
	assert.Empty(t, s.ClusterPersonas())

	s.PutPod(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "p"}})
	require.Len(t, s.Pods(), 1)
	s.DeletePod("apps", "p")
	assert.Empty(t, s.Pods())
}

func TestNilPutsAreIgnored(t *testing.T) {
	s := New(nil)
	s.PutAppPersona(nil)
	s.PutClusterPersona(nil)
	s.PutIncident(nil)
	s.PutRemediation(nil)
	s.PutDorguEvent(nil)
	s.PutDeployment(nil)
	s.PutPod(nil)

	assert.Empty(t, s.AppPersonas())
	assert.Empty(t, s.Deployments())
}

// The objects an informer hands over belong to its shared cache: every other
// consumer in the process sees the same pointer. Mutating one would corrupt the
// informer's own view of the cluster.
func TestStoreDoesNotAliasTheObjectItWasGiven(t *testing.T) {
	s := New(nil)
	original := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout",
			Labels: map[string]string{"app": "checkout"}},
	}
	s.PutDeployment(original)

	original.Labels["app"] = "mutated-by-someone-else"

	stored := s.Deployments()
	require.Len(t, stored, 1)
	assert.Equal(t, "checkout", stored[0].Labels["app"],
		"the store must hold a copy, not the informer's object")
}

// A reader must not be able to change what the next reader sees.
func TestReadersGetIndependentCopies(t *testing.T) {
	s := New(nil)
	s.PutDeployment(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout",
			Labels: map[string]string{"app": "checkout"}},
	})

	first := s.Deployments()
	first[0].Labels["app"] = "scribbled-on"

	second := s.Deployments()
	assert.Equal(t, "checkout", second[0].Labels["app"])
}

func TestIncidentChangesTouchBothViews(t *testing.T) {
	r := &recorder{}
	s := New(r.notify)

	s.PutIncident(&dorguv1.IncidentMemory{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "oom"},
	})

	// An app row shows its open incident count, so a new incident changes the
	// Apps view as well as the Incidents feed.
	assert.Equal(t, []Topic{TopicIncidents, TopicApps}, r.seen())
}

func TestTopicsPerResourceKind(t *testing.T) {
	tests := []struct {
		name string
		act  func(*Store)
		want []Topic
	}{
		{"persona", func(s *Store) {
			s.PutAppPersona(&dorguv1.ApplicationPersona{ObjectMeta: metav1.ObjectMeta{Name: "a"}})
		}, []Topic{TopicApps}},
		{"deployment", func(s *Store) {
			s.PutDeployment(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "a"}})
		}, []Topic{TopicApps}},
		{"pod", func(s *Store) {
			s.PutPod(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a"}})
		}, []Topic{TopicApps}},
		{"remediation", func(s *Store) {
			s.PutRemediation(&dorguv1.RemediationAction{ObjectMeta: metav1.ObjectMeta{Name: "a"}})
		}, []Topic{TopicRemediations}},
		{"event", func(s *Store) {
			s.PutDorguEvent(&dorguv1.DorguEvent{ObjectMeta: metav1.ObjectMeta{Name: "a"}})
		}, []Topic{TopicEvents}},
		{"cluster persona", func(s *Store) {
			s.PutClusterPersona(&dorguv1.ClusterPersona{ObjectMeta: metav1.ObjectMeta{Name: "a"}})
		}, []Topic{TopicCluster}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{}
			tt.act(New(r.notify))
			assert.Equal(t, tt.want, r.seen())
		})
	}
}

// A missing CRD and an empty list are different facts, so an unrecorded resource
// reports false: we have no evidence the operator's CRDs are installed.
func TestCRDPresenceDefaultsToAbsent(t *testing.T) {
	s := New(nil)
	assert.False(t, s.CRDPresent("applicationpersonas"))

	s.SetCRDPresent("applicationpersonas", true)
	assert.True(t, s.CRDPresent("applicationpersonas"))
}

func TestSyncedAllRequiresAtLeastOneInformer(t *testing.T) {
	s := New(nil)
	assert.False(t, s.SyncedAll(), "no informers is not a synced cache")

	s.SetSynced("deployments", false)
	assert.False(t, s.SyncedAll())

	s.SetSynced("deployments", true)
	assert.True(t, s.SyncedAll())

	s.SetSynced("pods", false)
	assert.False(t, s.SyncedAll(), "one lagging informer holds the whole answer back")
}

// Notification happens outside the write lock, because notify fans out to SSE
// clients and holding the store lock across that would let one slow reader stall
// every informer.
func TestNotifyRunsOutsideTheLock(t *testing.T) {
	var s *Store
	done := make(chan struct{})

	s = New(func(Topic) {
		// A read from inside the callback would deadlock if the write lock were
		// still held.
		_ = s.Deployments()
		close(done)
	})

	s.PutDeployment(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "a"}})
	<-done
}

func TestConcurrentWritesAndReads(t *testing.T) {
	s := New(func(Topic) {})

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for n := range 100 {
				s.PutDeployment(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Namespace: "apps",
					Name:      string(rune('a'+worker)) + string(rune('0'+n%10)),
				}})
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				_ = s.Deployments()
				_ = s.SyncedAll()
			}
		}()
	}
	wg.Wait()

	assert.NotEmpty(t, s.Deployments())
}

func TestAllTopicsCoversEveryConstant(t *testing.T) {
	assert.ElementsMatch(t,
		[]Topic{TopicApps, TopicIncidents, TopicRemediations, TopicEvents, TopicCluster, TopicMeta},
		AllTopics)
}
