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
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/informers"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/sse"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/view"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// harness is a server wired over a real store and broker, with no cluster.
type harness struct {
	server *Server
	store  *store.Store
	broker *sse.Broker
}

func newHarness(t *testing.T, assets fs.FS, opts ...func(*Config)) *harness {
	t.Helper()

	broker := sse.NewBroker()
	cache := store.New(nil)

	cfg := Config{
		Store:     cache,
		Snapshots: NewSnapshots(cache, SnapshotOptions{}),
		Broker:    broker,
		Logger:    discardLogger(),
		Assets:    assets,
		Cluster:   ClusterInfo{Context: "dorgu-dev", Server: "https://example.eks.amazonaws.com"},
		Version:   "test",
	}
	for _, o := range opts {
		o(&cfg)
	}

	server, err := New(cfg)
	require.NoError(t, err)
	return &harness{server: server, store: cache, broker: broker}
}

// ready marks every informer synced and every CRD installed, which is the
// steady state on a cluster with the operator running.
func (h *harness) ready() {
	for _, name := range []string{informers.NameDeployments, informers.NamePods, informers.NameNodes} {
		h.store.SetSynced(name, true)
	}
	for _, resource := range kube.DorguResources {
		h.store.SetCRDPresent(resource, true)
		h.store.SetSynced(resource, true)
	}
}

func (h *harness) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://localhost:7171"+path, nil)
	rec := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

func TestNewRejectsAnIncompleteConfig(t *testing.T) {
	cache := store.New(nil)
	full := Config{
		Store:     cache,
		Snapshots: NewSnapshots(cache, SnapshotOptions{}),
		Broker:    sse.NewBroker(),
		Logger:    discardLogger(),
	}

	tests := map[string]func(*Config){
		"Store":     func(c *Config) { c.Store = nil },
		"Snapshots": func(c *Config) { c.Snapshots = nil },
		"Broker":    func(c *Config) { c.Broker = nil },
		"Logger":    func(c *Config) { c.Logger = nil },
	}
	for field, drop := range tests {
		t.Run("missing "+field, func(t *testing.T) {
			cfg := full
			drop(&cfg)
			_, err := New(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), field)
		})
	}

	_, err := New(full)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// REST reads
// ---------------------------------------------------------------------------

func TestAppsEndpointServesTheProjectedView(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	h.store.PutAppPersona(&dorguv1.ApplicationPersona{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout"},
		Spec:       dorguv1.ApplicationPersonaSpec{Name: "checkout", Type: "api"},
	})
	h.store.PutDeployment(deployment("apps", "legacy-billing"))

	rec := h.get(t, "/api/v1/apps")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"),
		"a cached read is the one way a user ends up reaching for reload")

	var payload view.AppsPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.Equal(t, 2, payload.Summary.Total)
	assert.Equal(t, 1, payload.Summary.Unmonitored)
	assert.True(t, payload.Readiness.Synced)
	assert.True(t, payload.Readiness.CRDsInstalled)
}

func TestIncidentsEndpointServesTheProjectedFeed(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	h.store.PutIncident(&dorguv1.IncidentMemory{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout-oom"},
		Spec: dorguv1.IncidentMemorySpec{
			PersonaRef: dorguv1.PersonaReference{Kind: "ApplicationPersona", Name: "checkout", Namespace: "apps"},
			Category:   "resource",
			Severity:   "critical",
			Detection:  dorguv1.DetectionInfo{Signal: "OOMKilled", Source: "pod-failure-detector"},
		},
	})

	rec := h.get(t, "/api/v1/incidents")
	require.Equal(t, http.StatusOK, rec.Code)

	var payload view.IncidentsPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Incidents, 1)
	assert.Equal(t, "OOMKilled", payload.Incidents[0].Signal)
	assert.Equal(t, 1, payload.Summary.Critical)
}

// An empty list must not read as an answer while the caches are cold or the
// CRDs are absent. Four different causes, four different screens.
func TestReadinessDistinguishesLoadingFromNoOperatorFromEmpty(t *testing.T) {
	t.Run("cold caches", func(t *testing.T) {
		h := newHarness(t, nil)
		var payload view.AppsPayload
		require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/apps").Body.Bytes(), &payload))
		assert.False(t, payload.Readiness.Synced)
	})

	t.Run("no operator installed", func(t *testing.T) {
		h := newHarness(t, nil)
		h.store.SetSynced(informers.NameDeployments, true)
		h.store.SetSynced(informers.NamePods, true)
		for _, r := range kube.DorguResources {
			h.store.SetCRDPresent(r, false)
		}

		var payload view.AppsPayload
		require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/apps").Body.Bytes(), &payload))
		assert.True(t, payload.Readiness.Synced,
			"a missing CRD has no informer, so it cannot hold sync back forever")
		assert.False(t, payload.Readiness.CRDsInstalled)
		assert.Equal(t, []string{kube.ResourceApplicationPersonas}, payload.Readiness.MissingCRDs)
	})

	t.Run("genuinely empty", func(t *testing.T) {
		h := newHarness(t, nil)
		h.ready()
		var payload view.AppsPayload
		require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/apps").Body.Bytes(), &payload))
		assert.True(t, payload.Readiness.Synced)
		assert.True(t, payload.Readiness.CRDsInstalled)
		assert.Empty(t, payload.Apps)
	})
}

func TestMetaNamesTheClusterAndStatesWhatIsNotBuilt(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()

	rec := h.get(t, "/api/v1/meta")
	require.Equal(t, http.StatusOK, rec.Code)

	var meta Meta
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &meta))
	assert.Equal(t, "test", meta.Version)
	assert.Equal(t, "dorgu-dev", meta.Cluster.Context)
	assert.Equal(t, "https://example.eks.amazonaws.com", meta.Cluster.Server)
	assert.True(t, meta.OperatorInstalled)
	assert.True(t, meta.Synced)
	assert.Len(t, meta.CRDs, len(kube.DorguResources))

	byID := map[string]ViewStatus{}
	for _, v := range meta.Views {
		byID[v.ID] = v
	}
	// All four views render in this build. Both gates lifted: operator v0.11.1
	// makes AI-planned remediations appliable or falls back to rule-based, and
	// operator v0.11.0 with CLI v0.12.0 fixed the saturation figure.
	for _, id := range []string{"apps", "incidents", "remediations", "cluster"} {
		assert.True(t, byID[id].Available, "%s renders in this build", id)
		assert.Empty(t, byID[id].Reason, "%s is available, so it has nothing to excuse", id)
	}

	// Available is not finished, and the gap is the part a reader has to be
	// told. The Remediations screen shows a plan and cannot approve it, and the
	// nav says so rather than leaving it to be discovered.
	assert.NotEmpty(t, byID["remediations"].Limitation,
		"the read-only limit on Remediations has to be stated")
	assert.Contains(t, byID["remediations"].Limitation, "Read-only")
	assert.NotEmpty(t, byID["cluster"].Limitation)
	assert.Empty(t, byID["apps"].Limitation)
}

func TestMetaReportsMissingCRDs(t *testing.T) {
	h := newHarness(t, nil)
	for _, r := range kube.DorguResources {
		h.store.SetCRDPresent(r, true)
	}
	h.store.SetCRDPresent(kube.ResourceRemediationActions, false)

	var meta Meta
	require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/meta").Body.Bytes(), &meta))
	assert.False(t, meta.OperatorInstalled)
	assert.Equal(t, []string{kube.ResourceRemediationActions}, meta.MissingCRDs)
}

// Waiting on a cold cache is what something orchestrating this wants, so it is
// a 503 rather than a 200 with an empty page behind it.
func TestHealthzIsUnavailableUntilTheCachesAreWarm(t *testing.T) {
	h := newHarness(t, nil)
	assert.Equal(t, http.StatusServiceUnavailable, h.get(t, "/healthz").Code)

	h.ready()
	rec := h.get(t, "/healthz")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestUnknownAPIPathIsNotSwallowedByTheSPAFallback(t *testing.T) {
	h := newHarness(t, spaFS())
	assert.Equal(t, http.StatusNotFound, h.get(t, "/api/v1/nope").Code,
		"a typo in an API path must not return a page of HTML")
}

func TestNonGETMethodsAreRejected(t *testing.T) {
	h := newHarness(t, nil)
	req := httptest.NewRequest(http.MethodPost, "http://localhost:7171/api/v1/apps", nil)
	rec := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code,
		"M2 is read-only; there is no mutation to route yet")
}

// ---------------------------------------------------------------------------
// snapshot options
// ---------------------------------------------------------------------------

func TestNamespaceScopeIsAppliedToEveryView(t *testing.T) {
	h := newHarness(t, nil, func(c *Config) {
		c.Snapshots = NewSnapshots(c.Store, SnapshotOptions{Namespace: "apps"})
	})
	h.ready()
	h.store.PutDeployment(deployment("apps", "checkout"))
	h.store.PutDeployment(deployment("staging", "checkout"))

	var payload view.AppsPayload
	require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/apps").Body.Bytes(), &payload))
	require.Len(t, payload.Apps, 1)
	assert.Equal(t, "apps", payload.Apps[0].Namespace)
}

func TestSystemNamespacesAreExcludedByDefault(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	h.store.PutDeployment(deployment("kube-system", "coredns"))
	h.store.PutDeployment(deployment("apps", "checkout"))

	var payload view.AppsPayload
	require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/apps").Body.Bytes(), &payload))
	require.Len(t, payload.Apps, 1)
	assert.Equal(t, "checkout", payload.Apps[0].Name)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func deployment(namespace, name string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 1},
	}
}

func spaFS() fs.FS {
	return fstest.MapFS{
		"index.html":             {Data: []byte("<!doctype html><title>dorgu</title>")},
		"assets/index-abc123.js": {Data: []byte("console.log('dorgu')")},
	}
}
