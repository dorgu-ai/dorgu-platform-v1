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

package metrics

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

var readAt = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)

func TestNodeUsageIsSummedAcrossNodes(t *testing.T) {
	raw := []byte(`{"items":[
		{"metadata":{"name":"node-a"},"usage":{"cpu":"21m","memory":"500Mi"}},
		{"metadata":{"name":"node-b"},"usage":{"cpu":"17m","memory":"510Mi"}}
	]}`)

	got, err := ParseNodeUsage(raw, readAt)
	require.NoError(t, err)
	require.True(t, got.Available())

	cpu, ok := got.CPU()
	require.True(t, ok)
	assert.Equal(t, int64(38), cpu.MilliValue())

	memory, ok := got.Memory()
	require.True(t, ok)
	assert.Equal(t, int64(1010*1024*1024), memory.Value())
	assert.Equal(t, readAt, got.ReadAt)
}

// metrics-server running but having sampled nothing is not zero usage. Reporting
// it as 0% is the exact false statement this package exists to avoid.
func TestAnEmptyItemsListIsAnErrorNotAZeroReading(t *testing.T) {
	_, err := ParseNodeUsage([]byte(`{"items":[]}`), readAt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reported no nodes")
}

func TestAnUnparseableReplyIsAnError(t *testing.T) {
	_, err := ParseNodeUsage([]byte(`<html>404 not found</html>`), readAt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not parse")
}

// A malformed quantity on one node costs that node's contribution, not the whole
// figure. Refusing to report usage because one field is wrong would trade a
// slightly low number for no number.
func TestAMalformedQuantityDoesNotDiscardTheWholeReading(t *testing.T) {
	raw := []byte(`{"items":[
		{"metadata":{"name":"node-a"},"usage":{"cpu":"21m","memory":"500Mi"}},
		{"metadata":{"name":"node-b"},"usage":{"cpu":"not-a-quantity","memory":""}}
	]}`)

	got, err := ParseNodeUsage(raw, readAt)
	require.NoError(t, err)
	cpu, _ := got.CPU()
	assert.Equal(t, int64(21), cpu.MilliValue())
}

func TestPollerRequiresItsDependencies(t *testing.T) {
	_, err := NewPoller(Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Kubernetes client is required")
}

// The defaults are the ones documented: metrics-server samples every fifteen
// seconds and serves the last sample, so asking more often returns the same
// numbers with more requests.
func TestPollerDefaultsAreApplied(t *testing.T) {
	p, err := NewPoller(Options{
		Client: fake.NewSimpleClientset(),
		Store:  store.New(nil),
		Logger: quietLogger(),
	})
	require.NoError(t, err)
	assert.Equal(t, DefaultInterval, p.opts.Interval)
	assert.Equal(t, DefaultTimeout, p.opts.Timeout)
}

// ---------------------------------------------------------------------------
// The poll loop, against a real HTTP transport
// ---------------------------------------------------------------------------

// clientAgainst builds a real Kubernetes client pointed at a test server, so the
// tests below exercise the actual request path: AbsPath, DoRaw and the error
// wrapping around them. A fake clientset would skip exactly the part most likely
// to be wrong.
func clientAgainst(t *testing.T, handler http.HandlerFunc) (kubernetes.Interface, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)
	return client, server
}

func TestASuccessfulPollRecordsTheUsage(t *testing.T) {
	client, _ := clientAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, NodesPath, r.URL.Path, "the metrics endpoint is read by absolute path")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"node-a"},"usage":{"cpu":"38m","memory":"1010Mi"}}]}`))
	})

	cache := store.New(nil)
	poller, err := NewPoller(Options{Client: client, Store: cache, Logger: quietLogger()})
	require.NoError(t, err)

	poller.once(context.Background())

	got := cache.NodeUsage()
	require.True(t, got.Available())
	cpu, _ := got.CPU()
	assert.Equal(t, int64(38), cpu.MilliValue())
	assert.Empty(t, got.Unavailable)
}

// A cluster with no metrics-server answers 404, which is the ordinary case and
// not a failure. The reason has to reach the store so the Cluster view can print
// it beside the absent figure.
func TestAFailedPollRecordsTheReasonRatherThanZero(t *testing.T) {
	client, _ := clientAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`404 page not found`))
	})

	cache := store.New(nil)
	poller, err := NewPoller(Options{Client: client, Store: cache, Logger: quietLogger()})
	require.NoError(t, err)

	poller.once(context.Background())

	got := cache.NodeUsage()
	assert.False(t, got.Available())
	assert.Contains(t, got.Unavailable, "metrics-server did not answer")
	_, ok := got.CPU()
	assert.False(t, ok, "a failed poll must not leave a zero reading behind")
}

// A cluster without metrics-server would otherwise log once per interval for as
// long as the dashboard is open.
func TestARepeatedFailureIsLoggedOnce(t *testing.T) {
	client, _ := clientAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	var logged bytes.Buffer
	poller, err := NewPoller(Options{
		Client: client,
		Store:  store.New(nil),
		Logger: slog.New(slog.NewTextHandler(&logged, nil)),
	})
	require.NoError(t, err)

	poller.once(context.Background())
	first := logged.Len()
	require.Positive(t, first, "the first failure is worth saying")

	poller.once(context.Background())
	poller.once(context.Background())
	assert.Equal(t, first, logged.Len(), "the same reason is not repeated every interval")
}

// Recovery is worth a line too, so a reader who saw the warning learns it is over.
func TestRecoveryIsLogged(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)

	client, _ := clientAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"n"},"usage":{"cpu":"1m","memory":"1Mi"}}]}`))
	})

	var logged bytes.Buffer
	cache := store.New(nil)
	poller, err := NewPoller(Options{
		Client: client, Store: cache,
		Logger: slog.New(slog.NewTextHandler(&logged, nil)),
	})
	require.NoError(t, err)

	poller.once(context.Background())
	failing.Store(false)
	poller.once(context.Background())

	assert.Contains(t, logged.String(), "available again")
	assert.True(t, cache.NodeUsage().Available())
}

// Shutdown is not a metrics failure. Overwriting a good reading with a
// cancellation error on the way out would be the last thing the UI saw.
func TestACancelledPollLeavesTheLastReadingAlone(t *testing.T) {
	client, _ := clientAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"n"},"usage":{"cpu":"5m","memory":"1Mi"}}]}`))
	})

	cache := store.New(nil)
	poller, err := NewPoller(Options{Client: client, Store: cache, Logger: quietLogger()})
	require.NoError(t, err)

	poller.once(context.Background())
	require.True(t, cache.NodeUsage().Available())

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	poller.once(cancelled)

	assert.True(t, cache.NodeUsage().Available(),
		"a cancelled poll during shutdown must not erase what was measured")
}

// The first read happens immediately rather than after one interval, so the
// Cluster view has a used figure on its first paint.
func TestRunReadsImmediatelyAndStopsWithItsContext(t *testing.T) {
	var reads atomic.Int32
	client, _ := clientAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		reads.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"n"},"usage":{"cpu":"1m","memory":"1Mi"}}]}`))
	})

	cache := store.New(nil)
	poller, err := NewPoller(Options{
		Client: client, Store: cache, Logger: quietLogger(),
		Interval: 10 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		poller.Run(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool { return reads.Load() >= 2 }, time.Second, 5*time.Millisecond,
		"the loop reads once on entry and then on each tick")

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return when its context was cancelled")
	}
}
