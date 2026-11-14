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

// Package httpapi serves the REST reads, the SSE stream and the embedded SPA.
//
// Nothing here talks to the Kubernetes API server. Handlers read the in-memory
// cache, so the cost of a page load is a map iteration and a JSON encode, and
// twenty open tabs cost the API server nothing.
package httpapi

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/sse"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/view"
	"github.com/dorgu-ai/dorgu-platform-v1/pkg/report"
)

// DefaultHeartbeat is how often an idle SSE stream emits a comment line.
//
// Twenty five seconds sits under the thirty second idle timeout most reverse
// proxies default to. The dashboard binds to localhost and normally has no proxy
// in front of it, but a heartbeat is also how a client learns the connection is
// alive on a cluster where nothing is changing, which is the healthy case.
const DefaultHeartbeat = 25 * time.Second

// Config is everything the server needs. Every field except Assets,
// Cluster, Version, Heartbeat and AllowedHosts is required.
type Config struct {
	Store     *store.Store
	Snapshots *Snapshots
	Broker    *sse.Broker
	Logger    *slog.Logger

	// Assets is the built SPA. A nil FS is allowed and serves an explanation
	// instead of a blank page: a Go build with no frontend build is a normal
	// state during development and it should say so.
	Assets fs.FS

	// Cluster describes what this process is pointed at, so the UI can say
	// which cluster it is showing.
	Cluster ClusterInfo
	// Identity is the authenticated identity shown in the header. The zero
	// value reads as "not available".
	Identity IdentityInfo
	// Version is the dashboard build version, reported by the meta endpoint.
	Version string
	// Heartbeat overrides DefaultHeartbeat.
	Heartbeat time.Duration
	// AllowedHosts overrides the default localhost-only Host allowlist. Set it
	// when binding to a non-loopback address on purpose.
	AllowedHosts []string
	// Token is the per-session secret every /api request must present, as the
	// session cookie or an Authorization: Bearer header. Required: there is no
	// way to run this server unauthenticated. See NewToken.
	Token string
}

// ClusterInfo is the connection the dashboard is reading.
type ClusterInfo struct {
	Context   string `json:"context,omitempty"`
	Server    string `json:"server"`
	Namespace string `json:"namespace,omitempty"`
	InCluster bool   `json:"inCluster"`
}

// Server routes HTTP requests. Build it with New and serve Handler().
type Server struct {
	cfg       Config
	handler   http.Handler
	heartbeat time.Duration
}

// New validates the config and builds the route table.
func New(cfg Config) (*Server, error) {
	switch {
	case cfg.Store == nil:
		return nil, fmt.Errorf("httpapi: Store is required")
	case cfg.Snapshots == nil:
		return nil, fmt.Errorf("httpapi: Snapshots is required")
	case cfg.Broker == nil:
		return nil, fmt.Errorf("httpapi: Broker is required")
	case cfg.Logger == nil:
		return nil, fmt.Errorf("httpapi: Logger is required")
	case cfg.Token == "":
		return nil, fmt.Errorf("httpapi: Token is required")
	}

	heartbeat := cfg.Heartbeat
	if heartbeat == 0 {
		heartbeat = DefaultHeartbeat
	}

	s := &Server{cfg: cfg, heartbeat: heartbeat}
	s.handler = protect(cfg, s.routes())
	return s, nil
}

// protect wraps a route table in every request check, outermost first: Host
// (DNS rebinding), then Origin (cross-site), then the security headers, then
// the session token. A request that fails an earlier check never reaches a
// later one, and every route is behind all of them because they wrap the whole
// mux rather than being attached per route. It is a function of its own so a
// test can put a write route behind it and prove nothing reaches it unchecked.
func protect(cfg Config, routes http.Handler) http.Handler {
	return guardHost(cfg.AllowedHosts,
		guardOrigin(securityHeaders(requireSession(cfg.Token, routes))))
}

// Handler returns the root handler.
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) routes() http.Handler {
	// The API lives on its own mux, mounted under /api/. That separation is
	// what makes both of these behave: the outer mux has a catch-all serving
	// the SPA, and a catch-all would otherwise swallow a mistyped API path as
	// a page of HTML and turn a wrong method into the SPA's own status code.
	// With no catch-all inside, this mux answers an unknown /api/ path with 404
	// and a wrong method with 405, which is what a client needs to hear.
	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/apps", s.handleApps)
	api.HandleFunc("GET /api/v1/incidents", s.handleIncidents)
	api.HandleFunc("GET /api/v1/remediations", s.handleRemediations)
	api.HandleFunc("GET /api/v1/memory", s.handleMemory)
	api.HandleFunc("GET /api/v1/overview", s.handleOverview)
	api.HandleFunc("GET /api/v1/cluster", s.handleCluster)
	api.HandleFunc("GET /api/v1/meta", s.handleMeta)
	api.HandleFunc("GET /api/v1/stream", s.handleStream)

	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Anything not claimed above is the SPA: its client-side router owns those
	// paths and needs index.html served for each of them.
	mux.Handle("/", spaHandler(s.cfg.Assets, s.cfg.Logger))
	return mux
}

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, s.cfg.Logger, http.StatusOK, s.cfg.Snapshots.Apps())
}

func (s *Server) handleIncidents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, s.cfg.Logger, http.StatusOK, s.cfg.Snapshots.Incidents())
}

func (s *Server) handleRemediations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, s.cfg.Logger, http.StatusOK, s.cfg.Snapshots.Remediations())
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, s.cfg.Logger, http.StatusOK, s.cfg.Snapshots.Memory())
}

// handleOverview serves the report for ?since=, defaulting to the live window.
// A window that does not parse is a 400 that says what was wrong: silently
// falling back to the default would show the user numbers for a window they did
// not ask for, under a label that names the one they did.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	since := view.DefaultOverviewWindow
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := report.ParseWindow("since", raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		since = parsed
	}
	writeJSON(w, r, s.cfg.Logger, http.StatusOK, s.cfg.Snapshots.Overview(since))
}

func (s *Server) handleCluster(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, s.cfg.Logger, http.StatusOK, s.cfg.Snapshots.Cluster())
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// Reports this process, not the cluster. A caches-not-synced answer is a
	// 503 on purpose: something waiting on this endpoint should wait for a warm
	// cache rather than get an empty page.
	if !s.cfg.Store.SyncedAll() {
		writeJSON(w, r, s.cfg.Logger, http.StatusServiceUnavailable, map[string]any{
			"status": "syncing",
			"detail": "informer caches have not completed their initial list",
		})
		return
	}
	writeJSON(w, r, s.cfg.Logger, http.StatusOK, map[string]any{"status": "ok"})
}

// EncodeTopic renders a topic's current snapshot for the SSE stream. It reports
// false for a topic with no view, which is now only DorguEvents: they are
// watched and cached, and nothing renders them yet.
func (s *Server) EncodeTopic(topic store.Topic) ([]byte, bool) {
	var payload any
	switch topic {
	case store.TopicApps:
		payload = s.cfg.Snapshots.Apps()
	case store.TopicIncidents:
		payload = s.cfg.Snapshots.Incidents()
	case store.TopicRemediations:
		payload = s.cfg.Snapshots.Remediations()
	case store.TopicCluster:
		payload = s.cfg.Snapshots.Cluster()
	case store.TopicMemory:
		payload = s.cfg.Snapshots.Memory()
	case store.TopicOverview:
		payload = s.cfg.Snapshots.Overview(view.DefaultOverviewWindow)
	case store.TopicMeta:
		payload = s.meta()
	default:
		return nil, false
	}

	data, err := json.Marshal(payload)
	if err != nil {
		s.cfg.Logger.Error("encoding snapshot for the live stream",
			"topic", topic, "error", err)
		return nil, false
	}
	return data, true
}

// Publish builds a topic's snapshot and pushes it to every connected client.
// It is the flush callback for the coalescer.
func (s *Server) Publish(topic store.Topic) {
	data, ok := s.EncodeTopic(topic)
	if !ok {
		return
	}
	s.cfg.Broker.Publish(string(topic), data)
}

// StreamTopics are the topics a connecting client is sent a snapshot of, in the
// order they are sent.
//
// Meta first, deliberately. It carries which cluster this is and which views
// this build renders, so the shell is correct before any view's data lands and
// nothing has to re-render when it does.
//
// Only the topics with a view. DorguEvents is watched and cached and has none
// yet, so it is published to the broker and no client is told about it.
var StreamTopics = []store.Topic{
	store.TopicMeta,
	store.TopicApps,
	store.TopicIncidents,
	store.TopicRemediations,
	store.TopicCluster,
	store.TopicMemory,
	store.TopicOverview,
}

func writeJSON(w http.ResponseWriter, r *http.Request, logger *slog.Logger, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// The cache is authoritative and changes constantly, and the SSE stream is
	// the update path. A cached REST response would be the one way a user could
	// end up looking at stale data and reaching for reload.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already sent, so this cannot become an error
		// response. It is logged rather than swallowed.
		logger.Warn("writing response body", "path", r.URL.Path, "error", err)
	}
}
