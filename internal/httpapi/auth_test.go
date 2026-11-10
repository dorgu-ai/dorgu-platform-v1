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
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeProbe is a stand-in for a write route that does not exist yet. Every
// route today is a GET; these tests put a mutating route behind the same chain
// so a future write endpoint inherits checks that are already proven to hold
// for every method.
type writeProbe struct {
	hits int
}

func (p *writeProbe) routes() http.Handler {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) {
		p.hits++
		w.WriteHeader(http.StatusNoContent)
	}
	mux.HandleFunc("GET /api/v1/read", ok)
	mux.HandleFunc("POST /api/v1/write", ok)
	mux.HandleFunc("PUT /api/v1/write", ok)
	mux.HandleFunc("PATCH /api/v1/write", ok)
	mux.HandleFunc("DELETE /api/v1/write", ok)
	mux.HandleFunc("GET /api/v1/stream", ok)
	mux.HandleFunc("GET /page", ok)
	mux.HandleFunc("GET /healthz", ok)
	return mux
}

const probeHost = "127.0.0.1:41234"

func probeChain(p *writeProbe, allowedHosts ...string) http.Handler {
	return protect(Config{Token: testToken, AllowedHosts: allowedHosts}, p.routes())
}

type probeRequest struct {
	method string
	path   string
	host   string
	header map[string]string
	cookie string
}

func (pr probeRequest) build() *http.Request {
	host := pr.host
	if host == "" {
		host = probeHost
	}
	req := httptest.NewRequest(pr.method, "http://"+host+pr.path, nil)
	req.Host = host
	for k, v := range pr.header {
		req.Header.Set(k, v)
	}
	if pr.cookie != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: pr.cookie})
	}
	return req
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

var apiRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/read"},
	{http.MethodGet, "/api/v1/stream"},
	{http.MethodPost, "/api/v1/write"},
	{http.MethodPut, "/api/v1/write"},
	{http.MethodPatch, "/api/v1/write"},
	{http.MethodDelete, "/api/v1/write"},
}

// Every check, against every method and every kind of route, and in each case
// the handler behind the chain must not run.
func TestEveryCheckBlocksEveryRouteBeforeItsHandlerRuns(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*probeRequest)
		status int
	}{
		{"missing token", func(*probeRequest) {}, http.StatusUnauthorized},
		{"bad bearer token", func(r *probeRequest) { r.header = bearer("wrong") }, http.StatusUnauthorized},
		{"bad cookie token", func(r *probeRequest) { r.cookie = "wrong" }, http.StatusUnauthorized},
		{"empty bearer token", func(r *probeRequest) { r.header = map[string]string{"Authorization": "Bearer "} }, http.StatusUnauthorized},
		{"basic auth is not a token", func(r *probeRequest) {
			r.header = map[string]string{"Authorization": "Basic " + testToken}
		}, http.StatusUnauthorized},
		{"token in the query of an API call is not accepted", func(r *probeRequest) {
			r.path += "?token=" + testToken
		}, http.StatusUnauthorized},
		{"bad Host with a valid token", func(r *probeRequest) {
			r.header = bearer(testToken)
			r.host = "evil.example"
		}, http.StatusForbidden},
		{"rebinding hostname with a valid cookie", func(r *probeRequest) {
			r.cookie = testToken
			r.host = "127.0.0.1.attacker.example:41234"
		}, http.StatusForbidden},
		{"cross-site Origin with a valid token", func(r *probeRequest) {
			r.header = map[string]string{"Authorization": "Bearer " + testToken, "Origin": "https://evil.example"}
		}, http.StatusForbidden},
		{"Origin on another port of localhost", func(r *probeRequest) {
			r.header = map[string]string{"Authorization": "Bearer " + testToken, "Origin": "http://127.0.0.1:9999"}
		}, http.StatusForbidden},
		{"Origin null with a valid cookie", func(r *probeRequest) {
			r.cookie = testToken
			r.header = map[string]string{"Origin": "null"}
		}, http.StatusForbidden},
		{"Origin with a path", func(r *probeRequest) {
			r.header = map[string]string{"Authorization": "Bearer " + testToken, "Origin": "http://" + probeHost + "/x"}
		}, http.StatusForbidden},
		{"Origin that is not a URL", func(r *probeRequest) {
			r.header = map[string]string{"Authorization": "Bearer " + testToken, "Origin": "%%%"}
		}, http.StatusForbidden},
		{"Origin with userinfo", func(r *probeRequest) {
			r.header = map[string]string{"Authorization": "Bearer " + testToken, "Origin": "http://a@" + probeHost}
		}, http.StatusForbidden},
		{"Origin with a non-http scheme", func(r *probeRequest) {
			r.header = map[string]string{"Authorization": "Bearer " + testToken, "Origin": "file://" + probeHost}
		}, http.StatusForbidden},
	}

	for _, route := range apiRoutes {
		for _, tc := range cases {
			t.Run(route.method+" "+route.path+" "+tc.name, func(t *testing.T) {
				probe := &writeProbe{}
				pr := probeRequest{method: route.method, path: route.path}
				tc.mutate(&pr)

				rec := httptest.NewRecorder()
				probeChain(probe).ServeHTTP(rec, pr.build())

				assert.Equal(t, tc.status, rec.Code)
				assert.Zero(t, probe.hits, "the handler behind the chain must not run")
				assertNoCORS(t, rec)
			})
		}
	}
}

func TestAValidTokenReachesEveryRoute(t *testing.T) {
	for _, route := range apiRoutes {
		t.Run(route.method+" "+route.path+" bearer", func(t *testing.T) {
			probe := &writeProbe{}
			rec := httptest.NewRecorder()
			probeChain(probe).ServeHTTP(rec, probeRequest{
				method: route.method, path: route.path, header: bearer(testToken),
			}.build())
			assert.Equal(t, http.StatusNoContent, rec.Code)
			assert.Equal(t, 1, probe.hits)
		})
		t.Run(route.method+" "+route.path+" cookie with same-origin Origin", func(t *testing.T) {
			probe := &writeProbe{}
			rec := httptest.NewRecorder()
			probeChain(probe).ServeHTTP(rec, probeRequest{
				method: route.method, path: route.path, cookie: testToken,
				header: map[string]string{"Origin": "http://" + probeHost},
			}.build())
			assert.Equal(t, http.StatusNoContent, rec.Code)
			assert.Equal(t, 1, probe.hits)
		})
	}
}

func TestBearerSchemeIsCaseInsensitive(t *testing.T) {
	probe := &writeProbe{}
	rec := httptest.NewRecorder()
	probeChain(probe).ServeHTTP(rec, probeRequest{
		method: http.MethodGet, path: "/api/v1/read",
		header: map[string]string{"Authorization": "bEaReR " + testToken},
	}.build())
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// A cookie is an ambient credential: the browser attaches it to whatever the
// page asks for. A write authenticated by one must prove it came from a page.
func TestACookieAuthenticatedWriteWithoutAnOriginIsRefused(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			probe := &writeProbe{}
			rec := httptest.NewRecorder()
			probeChain(probe).ServeHTTP(rec, probeRequest{
				method: method, path: "/api/v1/write", cookie: testToken,
			}.build())
			assert.Equal(t, http.StatusForbidden, rec.Code)
			assert.Zero(t, probe.hits)
		})
	}
}

// A read authenticated by cookie needs no Origin: same-origin GETs do not carry
// one.
func TestACookieAuthenticatedReadWithoutAnOriginIsAllowed(t *testing.T) {
	probe := &writeProbe{}
	rec := httptest.NewRecorder()
	probeChain(probe).ServeHTTP(rec, probeRequest{
		method: http.MethodGet, path: "/api/v1/stream", cookie: testToken,
	}.build())
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestAnExplicitAllowlistStillNeedsTheTokenAndASameHostOrigin(t *testing.T) {
	probe := &writeProbe{}
	chain := probeChain(probe, "dash.internal")
	host := "dash.internal:8443"

	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, probeRequest{method: http.MethodGet, path: "/api/v1/read", host: host}.build())
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = httptest.NewRecorder()
	chain.ServeHTTP(rec, probeRequest{
		method: http.MethodGet, path: "/api/v1/read", host: host, header: bearer(testToken),
	}.build())
	assert.Equal(t, http.StatusNoContent, rec.Code)

	rec = httptest.NewRecorder()
	chain.ServeHTTP(rec, probeRequest{
		method: http.MethodPost, path: "/api/v1/write", host: host, cookie: testToken,
		header: map[string]string{"Origin": "https://dash.internal:8443"},
	}.build())
	assert.Equal(t, http.StatusNoContent, rec.Code, "https behind a TLS proxy is a same-host Origin")
}

func TestTheErrorBodyNamesTheFixAndLeaksNoToken(t *testing.T) {
	probe := &writeProbe{}
	rec := httptest.NewRecorder()
	probeChain(probe).ServeHTTP(rec, probeRequest{
		method: http.MethodGet, path: "/api/v1/read", header: bearer("guess"),
	}.build())
	assert.Contains(t, rec.Body.String(), "open the URL that dorgu dashboard printed")
	assert.NotContains(t, rec.Body.String(), testToken)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

// ---------------------------------------------------------------------------
// the exchange
// ---------------------------------------------------------------------------

func TestTheTokenIsExchangedForAnHttpOnlyStrictCookieAndStrippedFromTheURL(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantLocation string
	}{
		{"root", "/?token=" + testToken, "/"},
		{"deep link", "/page?token=" + testToken, "/page"},
		{"other params survive", "/page?a=1&token=" + testToken + "&b=2", "/page?a=1&b=2"},
		{"scheme-relative path cannot become an open redirect", "//evil.example/x?token=" + testToken, "/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := &writeProbe{}
			rec := httptest.NewRecorder()
			probeChain(probe).ServeHTTP(rec, probeRequest{method: http.MethodGet, path: tc.path}.build())

			require.Equal(t, http.StatusSeeOther, rec.Code)
			assert.Equal(t, tc.wantLocation, rec.Header().Get("Location"))
			assert.NotContains(t, rec.Header().Get("Location"), "token")
			assert.Zero(t, probe.hits, "the exchange answers itself")

			cookies := rec.Result().Cookies()
			require.Len(t, cookies, 1)
			c := cookies[0]
			assert.Equal(t, SessionCookieName, c.Name)
			assert.Equal(t, testToken, c.Value)
			assert.True(t, c.HttpOnly, "script must not be able to read the session")
			assert.Equal(t, http.SameSiteStrictMode, c.SameSite)
			assert.Equal(t, "/", c.Path)
			assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		})
	}
}

func TestABadTokenInTheURLSetsNoCookieAndIsRefused(t *testing.T) {
	for _, path := range []string{"/?token=wrong", "/?token=", "/page?token", "/healthz?token=wrong"} {
		t.Run(path, func(t *testing.T) {
			probe := &writeProbe{}
			rec := httptest.NewRecorder()
			probeChain(probe).ServeHTTP(rec, probeRequest{method: http.MethodGet, path: path}.build())
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Empty(t, rec.Result().Cookies())
			assert.Zero(t, probe.hits)
		})
	}
}

func TestTheExchangeStillRunsTheHostAndOriginChecks(t *testing.T) {
	probe := &writeProbe{}
	rec := httptest.NewRecorder()
	probeChain(probe).ServeHTTP(rec, probeRequest{
		method: http.MethodGet, path: "/?token=" + testToken, host: "evil.example",
	}.build())
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, rec.Result().Cookies(), "a rebinding page must not be handed a session")

	rec = httptest.NewRecorder()
	probeChain(probe).ServeHTTP(rec, probeRequest{
		method: http.MethodGet, path: "/?token=" + testToken,
		header: map[string]string{"Origin": "https://evil.example"},
	}.build())
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
}

func TestThePageAndTheProbeNeedNoToken(t *testing.T) {
	for _, path := range []string{"/page", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			probe := &writeProbe{}
			rec := httptest.NewRecorder()
			probeChain(probe).ServeHTTP(rec, probeRequest{method: http.MethodGet, path: path}.build())
			assert.Equal(t, http.StatusNoContent, rec.Code)
			assert.Equal(t, 1, probe.hits)
		})
	}
}

// ---------------------------------------------------------------------------
// through the real server
// ---------------------------------------------------------------------------

func TestTheRealServerRefusesEveryAPIRouteWithoutAToken(t *testing.T) {
	h := newHarness(t, spaFS())
	for _, path := range []string{
		"/api/v1/apps", "/api/v1/incidents", "/api/v1/remediations",
		"/api/v1/cluster", "/api/v1/meta", "/api/v1/stream", "/api/v1/nope",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://localhost:7171"+path, nil)
			rec := httptest.NewRecorder()
			h.server.Handler().ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

// SSE is the route that is easiest to forget: it is long-lived, so a missing
// check would hand over a live feed rather than one response.
func TestTheStreamWithoutATokenIsRefusedOverARealConnection(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()
	ts := httptest.NewServer(h.server.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tests := []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"no credentials", func(*http.Request) {}, http.StatusUnauthorized},
		{"wrong bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }, http.StatusUnauthorized},
		{"wrong cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "nope"})
		}, http.StatusUnauthorized},
		{"token in the query", func(r *http.Request) {
			r.URL.RawQuery = url.Values{TokenQueryParam: {testToken}}.Encode()
		}, http.StatusUnauthorized},
		{"valid cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})
		}, http.StatusOK},
		{"valid bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+testToken) }, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/stream", nil)
			require.NoError(t, err)
			tc.mutate(req)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, tc.status, resp.StatusCode)
			if tc.status != http.StatusOK {
				assert.NotEqual(t, "text/event-stream", resp.Header.Get("Content-Type"))
			}
		})
	}
}

func TestNoResponseCarriesCORSHeaders(t *testing.T) {
	h := newHarness(t, spaFS())
	h.ready()
	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "http://localhost:7171/api/v1/meta", nil),
		httptest.NewRequest(http.MethodGet, "http://localhost:7171/", nil),
		httptest.NewRequest(http.MethodOptions, "http://localhost:7171/api/v1/meta", nil),
	}
	for _, req := range requests {
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("Origin", "http://localhost:7171")
		req.Header.Set("Access-Control-Request-Method", "GET")
		rec := httptest.NewRecorder()
		h.server.Handler().ServeHTTP(rec, req)
		assertNoCORS(t, rec)
	}
}

func assertNoCORS(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for name := range rec.Header() {
		assert.False(t, strings.HasPrefix(strings.ToLower(name), "access-control-"),
			"unexpected CORS header %s", name)
	}
}

func TestNewTokenIsRandomAndLongEnough(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		token, err := NewToken()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(token), 43, "256 bits is 43 base64url characters")
		assert.NotContains(t, token, "=")
		assert.False(t, seen[token], "tokens must not repeat")
		seen[token] = true
	}
}

func TestTokenMatchesIsExact(t *testing.T) {
	want := sha256Sum(testToken)
	assert.True(t, tokenMatches(want, testToken))
	for _, got := range []string{"", testToken + "x", testToken[:len(testToken)-1], strings.ToUpper(testToken)} {
		assert.False(t, tokenMatches(want, got), got)
	}
}

func sha256Sum(s string) [32]byte { return sha256.Sum256([]byte(s)) }

func TestMetaCarriesTheAuthenticatedIdentity(t *testing.T) {
	h := newHarness(t, nil, func(c *Config) {
		c.Identity = IdentityInfo{Available: true, Username: "alice", Groups: []string{"dev"}}
	})
	h.ready()

	var meta Meta
	require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/meta").Body.Bytes(), &meta))

	assert.True(t, meta.Identity.Available)
	assert.Equal(t, "alice", meta.Identity.Username)
	assert.Equal(t, []string{"dev"}, meta.Identity.Groups)
}

func TestMetaSaysWhenTheIdentityIsUnavailable(t *testing.T) {
	h := newHarness(t, nil, func(c *Config) {
		c.Identity = IdentityInfo{Reason: "needs Kubernetes 1.28"}
	})

	var meta Meta
	require.NoError(t, json.Unmarshal(h.get(t, "/api/v1/meta").Body.Bytes(), &meta))

	assert.False(t, meta.Identity.Available)
	assert.Equal(t, "needs Kubernetes 1.28", meta.Identity.Reason)
}
