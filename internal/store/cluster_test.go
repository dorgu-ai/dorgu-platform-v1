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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodesAreStoredAndHandedOutAsCopies(t *testing.T) {
	s := New(nil)
	s.PutNode(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"zone": "a"}},
	})

	nodes := s.Nodes()
	require.Len(t, nodes, 1)

	// Mutating what the store handed out must not reach the store, or one
	// render could corrupt the next.
	nodes[0].Labels["zone"] = "mutated"
	assert.Equal(t, "a", s.Nodes()[0].Labels["zone"])

	s.DeleteNode("", "node-a")
	assert.Empty(t, s.Nodes())
}

// An unavailable reading always carries a reason, so no consumer can render
// "n/a ()" with an empty operand.
func TestUnavailableNodeUsageAlwaysHasAReason(t *testing.T) {
	assert.NotEmpty(t, UnavailableNodeUsage("").Unavailable)
	assert.Equal(t, "no metrics-server", UnavailableNodeUsage("no metrics-server").Unavailable)
	assert.False(t, UnavailableNodeUsage("x").Available())
}

// A measurement and its absence are different states, and reading a quantity is
// only legal through the method that reports which one it is.
func TestNodeUsageDistinguishesZeroFromUnmeasured(t *testing.T) {
	measuredZero := NewNodeUsage(resource.MustParse("0"), resource.MustParse("0"), time.Now())
	cpu, ok := measuredZero.CPU()
	assert.True(t, ok, "a measured zero is a measurement")
	assert.Equal(t, int64(0), cpu.MilliValue())

	_, ok = UnavailableNodeUsage("no metrics-server").CPU()
	assert.False(t, ok, "an absent measurement must not read as zero")
}

// ---------------------------------------------------------------------------
// Sync failures
// ---------------------------------------------------------------------------

// The third state Synced cannot express. Without it a refused watch leaves a view
// on a loading skeleton for as long as the process runs, telling the reader it is
// still reading something it gave up on.
func TestSyncFailureIsRecordedAndReported(t *testing.T) {
	s := New(nil)
	assert.Empty(t, s.SyncFailure("nodes"))

	s.SetSyncFailed("nodes", "your kubeconfig user may not list nodes")
	assert.Contains(t, s.SyncFailure("nodes"), "may not list nodes")
	assert.False(t, s.Synced("nodes"))
}

// An informer that syncs on a retry is no longer failed. Leaving the reason
// behind would put a permanent warning on a view that is now correct.
func TestSyncingClearsARecordedFailure(t *testing.T) {
	s := New(nil)
	s.SetSyncFailed("nodes", "timed out")
	s.SetSynced("nodes", true)

	assert.Empty(t, s.SyncFailure("nodes"))
	assert.True(t, s.Synced("nodes"))
}

// Readiness changes have to republish every view that reports readiness, or a
// view sits on a stale skeleton until something else happens to change.
func TestReadinessChangesNotifyEveryViewThatReportsIt(t *testing.T) {
	r := &recorder{}
	s := New(r.notify)
	s.SetSyncFailed("nodes", "timed out")

	seen := r.seen()
	for _, topic := range []Topic{TopicMeta, TopicApps, TopicIncidents, TopicRemediations, TopicCluster} {
		assert.Contains(t, seen, topic, "%s carries a readiness block", topic)
	}
}
