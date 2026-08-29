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
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/view"
)

// frame is one parsed server-sent event.
type frame struct {
	Event string
	Data  string
	ID    string
}

// streamReader reads SSE frames off a live connection.
type streamReader struct {
	t      *testing.T
	reader *bufio.Reader
}

// next reads until one complete frame has arrived. A test that hangs here is a
// test that would have hung in the browser, which is the point.
func (s *streamReader) next() frame {
	s.t.Helper()

	var got frame
	for {
		line, err := s.reader.ReadString('\n')
		require.NoError(s.t, err, "reading the event stream")
		line = strings.TrimRight(line, "\r\n")

		switch {
		case line == "":
			if got.Event != "" {
				return got
			}
		case strings.HasPrefix(line, ":"):
			// A comment: the keep-alive. Not a frame.
		case strings.HasPrefix(line, "id: "):
			got.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			got.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			got.Data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// nextOf reads frames until one for the given topic arrives.
func (s *streamReader) nextOf(topic string) frame {
	s.t.Helper()
	for range 20 {
		if f := s.next(); f.Event == topic {
			return f
		}
	}
	s.t.Fatalf("no %s frame within 20 frames", topic)
	return frame{}
}

// openStream starts a real HTTP server and connects to the SSE endpoint.
func openStream(t *testing.T, h *harness) (*streamReader, func()) {
	t.Helper()

	ts := httptest.NewServer(h.server.Handler())
	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/stream", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	require.Equal(t, "no", resp.Header.Get("X-Accel-Buffering"),
		"a buffering proxy turns a live feed into a batch one")

	return &streamReader{t: t, reader: bufio.NewReader(resp.Body)}, func() {
		cancel()
		resp.Body.Close()
		ts.Close()
	}
}

// A new or reconnecting client is immediately correct without fetching
// anything. This is what makes "no manual refresh" true rather than aspirational.
func TestConnectingClientGetsASnapshotOfEveryView(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	h.store.PutDeployment(deployment("apps", "legacy-billing"))

	stream, close := openStream(t, h)
	defer close()

	seen := map[string]string{}
	for range len(StreamTopics) {
		f := stream.next()
		seen[f.Event] = f.Data
	}

	require.Contains(t, seen, "meta")
	require.Contains(t, seen, "apps")
	require.Contains(t, seen, "incidents")

	var apps view.AppsPayload
	require.NoError(t, json.Unmarshal([]byte(seen["apps"]), &apps))
	require.Len(t, apps.Apps, 1)
	assert.Equal(t, "legacy-billing", apps.Apps[0].Name)
}

// The whole M2 requirement in one test: a change in the cluster reaches an
// already-open browser with nothing asked of the user.
func TestAChangeInTheClusterReachesAnOpenClientWithNoRefresh(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()

	stream, close := openStream(t, h)
	defer close()

	initial := stream.nextOf("apps")
	var before view.AppsPayload
	require.NoError(t, json.Unmarshal([]byte(initial.Data), &before))
	require.Empty(t, before.Apps)

	// A Deployment appears. In production the informer writes it and the
	// coalescer flushes; here the publish is called directly, which is the same
	// code path minus the timer.
	h.store.PutDeployment(deployment("apps", "checkout"))
	h.server.Publish(store.TopicApps)

	pushed := stream.nextOf("apps")
	var after view.AppsPayload
	require.NoError(t, json.Unmarshal([]byte(pushed.Data), &after))
	require.Len(t, after.Apps, 1)
	assert.Equal(t, "checkout", after.Apps[0].Name)
	assert.NotEmpty(t, pushed.ID, "a pushed frame carries an id")
}

func TestAnIncidentReachesAnOpenClient(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()

	stream, close := openStream(t, h)
	defer close()

	stream.nextOf("incidents")

	h.store.PutIncident(&dorguv1.IncidentMemory{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout-oom"},
		Spec: dorguv1.IncidentMemorySpec{
			PersonaRef: dorguv1.PersonaReference{Kind: "ApplicationPersona", Name: "checkout", Namespace: "apps"},
			Category:   "resource",
			Severity:   "critical",
			Detection:  dorguv1.DetectionInfo{Signal: "OOMKilled", Source: "pod-failure-detector"},
		},
	})
	h.server.Publish(store.TopicIncidents)

	pushed := stream.nextOf("incidents")
	var payload view.IncidentsPayload
	require.NoError(t, json.Unmarshal([]byte(pushed.Data), &payload))
	require.Len(t, payload.Incidents, 1)
	assert.Equal(t, "OOMKilled", payload.Incidents[0].Signal)
	assert.Equal(t, "critical", payload.Incidents[0].Severity)
}

// The pushed payload is byte-for-byte what a fresh GET would return, which is
// what keeps the live update and the REST read from ever disagreeing.
func TestPushedPayloadMatchesTheRESTRead(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	h.store.PutDeployment(deployment("apps", "checkout"))

	pushed, ok := h.server.EncodeTopic(store.TopicApps)
	require.True(t, ok)

	rest := h.get(t, "/api/v1/apps").Body.Bytes()
	assert.JSONEq(t, string(pushed), string(rest))
}

func TestTopicsWithNoViewAreNotEncoded(t *testing.T) {
	h := newHarness(t, nil)

	// DorguEvents is the only topic left without a view. It is watched and
	// cached, and nothing renders it, so it must encode to nothing rather than
	// to an empty payload a client would apply.
	_, ok := h.server.EncodeTopic(store.TopicEvents)
	assert.False(t, ok, "events has no view in this build")
	assert.NotPanics(t, func() { h.server.Publish(store.TopicEvents) })
}

// Every topic a connecting client is sent a snapshot of has to encode to one.
// A topic in StreamTopics that EncodeTopic refuses would leave that view with no
// initial data and no way to know it was missing.
func TestEveryStreamedTopicEncodes(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()

	for _, topic := range StreamTopics {
		data, ok := h.server.EncodeTopic(topic)
		require.True(t, ok, "%s is streamed, so it must encode", topic)
		assert.NotEmpty(t, data)
	}
}

func TestHeartbeatKeepsAnIdleStreamAlive(t *testing.T) {
	h := newHarness(t, nil, func(c *Config) { c.Heartbeat = 20 * time.Millisecond })
	h.ready()

	ts := httptest.NewServer(h.server.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/stream", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	sawComment := false
	for range 40 {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		if strings.HasPrefix(line, ": keep-alive") {
			sawComment = true
			break
		}
	}
	assert.True(t, sawComment, "an idle stream must prove it is alive")
}

func TestStreamAnnouncesItsReconnectInterval(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()

	ts := httptest.NewServer(h.server.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/stream", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "retry: 2000\n", line,
		"the browser reconnects on its own; this is how it knows how soon")
}

// A disconnect must release the subscription, or a page reloaded a hundred times
// leaves a hundred mailboxes being published to.
func TestDisconnectingReleasesTheSubscription(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()

	stream, closeStream := openStream(t, h)
	stream.next()
	require.Eventually(t, func() bool { return h.broker.Subscribers() == 1 },
		2*time.Second, 10*time.Millisecond)

	closeStream()
	assert.Eventually(t, func() bool { return h.broker.Subscribers() == 0 },
		2*time.Second, 10*time.Millisecond)
}

func TestSeveralClientsAllReceiveTheSamePush(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()

	first, closeFirst := openStream(t, h)
	defer closeFirst()
	second, closeSecond := openStream(t, h)
	defer closeSecond()

	first.nextOf("apps")
	second.nextOf("apps")

	h.store.PutDeployment(deployment("apps", "checkout"))
	h.server.Publish(store.TopicApps)

	for _, stream := range []*streamReader{first, second} {
		var payload view.AppsPayload
		require.NoError(t, json.Unmarshal([]byte(stream.nextOf("apps").Data), &payload))
		require.Len(t, payload.Apps, 1)
	}
}

// SSE data lines are newline-delimited, so a payload containing a raw newline
// would corrupt the frame. Nothing json.Marshal produces does, and this pins it.
func TestEncodedPayloadsNeverContainRawNewlines(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	h.store.PutAppPersona(&dorguv1.ApplicationPersona{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout"},
		Spec:       dorguv1.ApplicationPersonaSpec{Name: "checkout", Type: "api"},
		Status: dorguv1.ApplicationPersonaStatus{
			Health: &dorguv1.HealthStatus{
				Status:  view.HealthUnhealthy,
				Message: "line one\nline two\r\nline three",
			},
		},
	})

	data, ok := h.server.EncodeTopic(store.TopicApps)
	require.True(t, ok)
	assert.NotContains(t, string(data), "\n")
	assert.NotContains(t, string(data), "\r")
}

var _ = appsv1.Deployment{}
