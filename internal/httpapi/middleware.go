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
	"net"
	"net/http"
	"strings"
)

// DefaultAllowedHosts is the Host header allowlist for a loopback bind.
var DefaultAllowedHosts = []string{"localhost", "127.0.0.1", "[::1]", "::1"}

// guardHost rejects requests whose Host header is not in the allowlist.
//
// This is the DNS rebinding defence, and an unauthenticated localhost server
// needs it. Without it, any web page the user visits can point a hostname it
// controls at 127.0.0.1 and then read this API cross-origin, because the browser
// considers the response same-origin with the attacker's page. The Host header is
// the one part of that request the attacker cannot forge into looking local.
//
// It is not a substitute for auth and is not presented as one. The Aug 22
// decision was that an unauthenticated in-cluster dashboard is indefensible;
// this is what keeps the local one from being trivially reachable from a browser
// tab the user did not open.
func guardHost(allowed []string, next http.Handler) http.Handler {
	if len(allowed) == 0 {
		allowed = DefaultAllowedHosts
	}
	set := make(map[string]bool, len(allowed))
	for _, h := range allowed {
		set[strings.ToLower(h)] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, set) {
			http.Error(w,
				"Request rejected: unexpected Host header. The dashboard only answers "+
					"requests addressed to localhost.\n",
				http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hostAllowed(host string, allowed map[string]bool) bool {
	if host == "" {
		// HTTP/1.1 requires a Host header and Go's server rejects a request
		// without one, so this is only reachable from a hand-built request.
		return false
	}
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	return allowed[strings.ToLower(name)]
}

// securityHeaders sets the headers that matter for a page which renders cluster
// contents.
//
// The content security policy is strict because it can be: everything the SPA
// loads is embedded in this binary and served from this origin, so there is no
// CDN, no analytics and no font host to carve an exception for. 'unsafe-inline'
// is granted for styles only, which is what a Tailwind build plus the runtime
// style injection React uses actually needs; scripts get no such grant.
//
// connect-src stays 'self' rather than 'none' because the SSE stream is an
// origin request. No CORS headers are set anywhere: the API is same-origin only,
// which is the correct posture for a server with no authentication.
func securityHeaders(next http.Handler) http.Handler {
	const policy = "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; " +
		"font-src 'self' data:; " +
		"connect-src 'self'; " +
		"object-src 'none'; " +
		"base-uri 'none'; " +
		"form-action 'none'; " +
		"frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", policy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
