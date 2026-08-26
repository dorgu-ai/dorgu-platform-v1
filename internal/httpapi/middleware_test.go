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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func requestWithHost(host string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Host = host
	return req
}

// An unauthenticated localhost server needs this. Without it, any page the user
// visits can point a hostname it controls at 127.0.0.1 and read this API
// cross-origin, because the browser treats the response as same-origin with the
// attacker's page.
func TestHostGuardAllowsOnlyLocalhostByDefault(t *testing.T) {
	handler := guardHost(nil, okHandler())

	allowed := []string{
		"localhost:7171",
		"127.0.0.1:7171",
		"127.0.0.1",
		"[::1]:7171",
		"LOCALHOST:7171",
	}
	for _, host := range allowed {
		t.Run("allow "+host, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, requestWithHost(host))
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}

	rejected := []string{
		"dorgu.attacker.example:7171",
		"evil.example",
		"192.168.1.10:7171",
		"127.0.0.1.attacker.example",
		"",
	}
	for _, host := range rejected {
		t.Run("reject "+host, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, requestWithHost(host))
			require.Equal(t, http.StatusForbidden, rec.Code)
			assert.Contains(t, rec.Body.String(), "unexpected Host header")
		})
	}
}

func TestHostGuardHonoursAnExplicitAllowlist(t *testing.T) {
	handler := guardHost([]string{"dashboard.internal"}, okHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithHost("dashboard.internal:7171"))
	assert.Equal(t, http.StatusOK, rec.Code)

	// An explicit allowlist replaces the default rather than extending it: a
	// deliberate non-loopback bind should not silently keep accepting localhost.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithHost("localhost:7171"))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHostGuardAppliesToTheStreamToo(t *testing.T) {
	h := newHarness(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil)
	req.Host = "evil.example"
	rec := httptest.NewRecorder()

	h.server.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSecurityHeadersAreSetOnEveryResponse(t *testing.T) {
	h := newHarness(t, spaFS())

	for _, path := range []string{"/api/v1/apps", "/api/v1/meta", "/healthz", "/"} {
		t.Run(path, func(t *testing.T) {
			rec := h.get(t, path)
			assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
			assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
			assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
			assert.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
		})
	}
}

func TestContentSecurityPolicyGrantsNothingToRemoteOrigins(t *testing.T) {
	h := newHarness(t, spaFS())
	policy := h.get(t, "/").Header().Get("Content-Security-Policy")

	assert.Contains(t, policy, "default-src 'self'")
	assert.Contains(t, policy, "connect-src 'self'", "the SSE stream is an origin request")
	assert.Contains(t, policy, "frame-ancestors 'none'")
	assert.Contains(t, policy, "object-src 'none'")
	assert.NotContains(t, policy, "script-src 'self' 'unsafe-inline'",
		"scripts get no inline grant; everything is bundled and served from this origin")
	assert.NotContains(t, policy, "https://", "no CDN, no font host, no analytics")
}

// The API is same-origin only, which is the correct posture for a server with no
// authentication. A CORS header would undo the Host guard above.
func TestNoCORSHeadersAreEverSet(t *testing.T) {
	h := newHarness(t, nil)
	rec := h.get(t, "/api/v1/apps")

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
}
