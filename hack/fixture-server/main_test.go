package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/fixtures"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/view"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4173"+path, nil)
	req.Header.Set("Authorization", "Bearer "+fixtureToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEveryScenarioServesTheRealAPI(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}
	for _, scenario := range fixtures.Scenarios {
		t.Run(string(scenario), func(t *testing.T) {
			srv, coalescer, err := build(scenario, assets, time.Now())
			require.NoError(t, err)
			require.NotNil(t, coalescer)

			for _, path := range []string{"/api/v1/meta", "/api/v1/overview", "/api/v1/incidents", "/api/v1/remediations",
				"/api/v1/memory", "/api/v1/apps", "/api/v1/cluster"} {
				assert.Equal(t, http.StatusOK, get(t, srv.Handler(), path).Code, path)
			}
		})
	}
}

func TestTheFixtureServerStillRequiresTheToken(t *testing.T) {
	srv, _, err := build(fixtures.Full, fstest.MapFS{"index.html": {Data: []byte("x")}}, time.Now())
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4173/api/v1/meta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestTheFullScenarioHasAReportAndTheOthersTellTheTruth(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("x")}}
	overview := func(s fixtures.Scenario) view.OverviewPayload {
		srv, _, err := build(s, assets, time.Now())
		require.NoError(t, err)
		var p view.OverviewPayload
		require.NoError(t, json.Unmarshal(get(t, srv.Handler(), "/api/v1/overview").Body.Bytes(), &p))
		return p
	}

	full := overview(fixtures.Full)
	require.NotNil(t, full.Report)
	assert.Equal(t, 9, full.Report.HealAccounting.Total)

	assert.Nil(t, overview(fixtures.Warming).Report)
	assert.False(t, overview(fixtures.Warming).Readiness.Synced)
	assert.False(t, overview(fixtures.CRDsMissing).Readiness.CRDsInstalled)
	assert.NotEmpty(t, overview(fixtures.Refused).Readiness.Unavailable)

	idle := overview(fixtures.OperatorIdle)
	require.NotNil(t, idle.Report)
	assert.False(t, idle.Operator.Running())
	assert.Equal(t, 0, idle.Report.HealAccounting.Total)

	none := overview(fixtures.None)
	assert.True(t, none.Operator.Running())
	assert.Equal(t, 0, none.Report.HealAccounting.Total)
}

func TestRunRejectsAnUnknownScenario(t *testing.T) {
	err := run("nonsense", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown scenario")
}

func TestServeStopsWhenItsContextIsCancelled(t *testing.T) {
	srv, coalescer, err := build(fixtures.Full, fstest.MapFS{"index.html": {Data: []byte("x")}}, time.Now())
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, listener, srv.Handler(), coalescer) }()

	resp, err := http.Get("http://" + listener.Addr().String() + "/healthz")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop")
	}
}

func TestServeReportsAListenerThatFails(t *testing.T) {
	srv, coalescer, err := build(fixtures.Full, fstest.MapFS{"index.html": {Data: []byte("x")}}, time.Now())
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, listener.Close())

	assert.Error(t, serve(t.Context(), listener, srv.Handler(), coalescer))
}
