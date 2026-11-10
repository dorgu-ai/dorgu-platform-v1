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
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// This file is the session layer: a per-run random token, exchanged for an
// HttpOnly cookie, required on every /api route.
//
// # Why a token on a loopback server
//
// Loopback keeps the network out. It does not keep out the user's own browser,
// which will happily send a request to 127.0.0.1 on behalf of any page it has
// open, and it does not keep out another local user or process. The Host check
// stops DNS rebinding and the Origin check stops cross-site requests, but both
// are about where a request claims to come from. The token is what proves the
// caller was handed the URL this process printed, which is the only thing that
// will matter once a route can write.
//
// # How it flows
//
//  1. Run generates a token and opens http://127.0.0.1:PORT/?token=T.
//  2. The first request carries it in the query. The server answers 303 to the
//     same URL without the token and sets the cookie, so the token never sits in
//     the address bar, in history or in a Referer header.
//  3. Every /api request must carry the cookie, or an Authorization: Bearer
//     header for a non-browser client such as curl. Nothing else is accepted on
//     /api, in particular not a query parameter, which would end up in logs.
//
// The embedded SPA and /healthz are served without it: the SPA is the same
// public bundle in every binary and holds no cluster data, and a probe has to
// be able to ask whether the process is up.

const (
	// SessionCookieName is the cookie the token is exchanged for.
	SessionCookieName = "dorgu_session"
	// TokenQueryParam is the one query parameter the exchange reads.
	TokenQueryParam = "token"

	tokenBytes = 32
)

// NewToken returns a fresh random session token: 256 bits, URL-safe.
func NewToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating the session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// tokenMatches compares in constant time. Hashing first makes the comparison
// length-independent, so a wrong-length guess leaks nothing about the length of
// the real token either.
func tokenMatches(want [sha256.Size]byte, got string) bool {
	sum := sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(want[:], sum[:]) == 1
}

// isAPIPath reports whether a path is under the authenticated API.
func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

// isSafeMethod is the set that cannot change state by definition.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// bearerToken extracts the token from an Authorization: Bearer header.
func bearerToken(r *http.Request) string {
	const prefix = "bearer "
	header := r.Header.Get("Authorization")
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}

// requireSession enforces the token.
//
// It wraps the whole mux, not individual routes, so a route added later is
// covered without anyone remembering to ask. Order of checks:
//
//   - a request with ?token= on a non-API path is the exchange: a valid token
//     sets the cookie and redirects with the token removed, an invalid one is a
//     401 and sets nothing;
//   - /api requires a valid cookie or bearer header, 401 otherwise;
//   - a state-changing request authenticated by the cookie must carry an Origin
//     header. Browsers always send one on POST, PUT, PATCH and DELETE, so its
//     absence means the request did not come from a page of ours. A bearer
//     header is not an ambient credential and needs no such check.
func requireSession(token string, next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAPIPath(r.URL.Path) {
			if r.URL.Query().Has(TokenQueryParam) {
				exchangeToken(w, r, want, token, r.URL.Query().Get(TokenQueryParam))
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		viaCookie := false
		authenticated := false
		if c, err := r.Cookie(SessionCookieName); err == nil && tokenMatches(want, c.Value) {
			authenticated, viaCookie = true, true
		}
		if !authenticated {
			if bearer := bearerToken(r); bearer != "" && tokenMatches(want, bearer) {
				authenticated = true
			}
		}
		if !authenticated {
			writeAuthError(w, http.StatusUnauthorized,
				"missing or invalid session token; open the URL that dorgu dashboard printed")
			return
		}

		if viaCookie && !isSafeMethod(r.Method) && r.Header.Get("Origin") == "" {
			writeAuthError(w, http.StatusForbidden,
				"a state-changing request must carry an Origin header")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// exchangeToken trades a query-string token for the session cookie and strips
// it from the URL.
func exchangeToken(w http.ResponseWriter, r *http.Request, want [sha256.Size]byte, token, supplied string) {
	if !tokenMatches(want, supplied) {
		writeAuthError(w, http.StatusUnauthorized,
			"invalid session token; open the URL that dorgu dashboard printed")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:  SessionCookieName,
		Value: token,
		Path:  "/",
		// HttpOnly keeps the token away from script, so an XSS in the SPA
		// cannot read it. SameSite=Strict keeps the browser from attaching it
		// to a request a different site initiated. Secure is not set: the
		// dashboard serves plain HTTP on loopback, and browsers refuse to store
		// a Secure cookie from an http origin in some configurations.
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	stripped := *r.URL
	query := stripped.Query()
	query.Del(TokenQueryParam)
	stripped.RawQuery = query.Encode()
	target := stripped.RequestURI()
	if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		// A path that would be read as a scheme-relative URL is never a place
		// to redirect to.
		target = "/"
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "{\"error\":%q}\n", message)
}

// guardOrigin rejects a request whose Origin header is not this server.
//
// It applies to every method and every path, and only to requests that carry
// the header: browsers send Origin on every cross-origin request and on every
// same-origin state-changing one, and a request without it is either a
// same-origin read or a non-browser client, which the token covers.
//
// Same-origin means the Origin's host and port equal the Host the request was
// addressed to. Host has already been through the allowlist, so this is
// stricter than either check alone. "null" is what a sandboxed frame or a
// redirect chain sends and is never accepted.
//
// No CORS headers are set anywhere, so a cross-origin read is blocked by the
// browser even if this check were bypassed. This is the server-side half.
func guardOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin, present := r.Header["Origin"]
		if present {
			if err := checkOrigin(origin, r.Host); err != nil {
				http.Error(w, "Request rejected: "+err.Error()+".\n", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

var errOrigin = errors.New("unexpected Origin header")

func checkOrigin(values []string, host string) error {
	if len(values) != 1 {
		return errOrigin
	}
	parsed, err := url.Parse(values[0])
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errOrigin
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" {
		return errOrigin
	}
	if !strings.EqualFold(parsed.Host, host) {
		return errOrigin
	}
	return nil
}
