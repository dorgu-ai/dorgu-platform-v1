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

package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
)

// startServed runs the real handler behind the real serve and returns the URL
// OnReady was given, which is exactly what the CLI opens in a browser.
func startServed(t *testing.T) string {
	t.Helper()
	ready := make(chan string, 1)
	opts := Options{Host: "127.0.0.1", Port: 0, OnReady: func(u string) { ready <- u }}.withDefaults()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	handler, err := testHandler()
	require.NoError(t, err)
	go func() { _ = serve(ctx, opts, handler, testToken, quietLogger()) }()

	select {
	case u := <-ready:
		return u
	case <-time.After(5 * time.Second):
		t.Fatal("OnReady was never called")
		return ""
	}
}

// The whole browser story over a real socket: open the printed URL, get
// redirected to a clean one, end up holding an HttpOnly cookie, and read the API.
func TestOpeningThePrintedURLEstablishesASessionAndStripsTheToken(t *testing.T) {
	printed := startServed(t)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	var finalURL *url.URL
	client := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		finalURL = req.URL
		return nil
	}}

	resp, err := client.Get(printed)
	require.NoError(t, err)
	resp.Body.Close()

	require.NotNil(t, finalURL, "the token exchange must redirect")
	assert.Empty(t, finalURL.RawQuery, "the token must be gone from the address bar")
	assert.Equal(t, "/", finalURL.Path)

	base, err := url.Parse(printed)
	require.NoError(t, err)
	cookies := jar.Cookies(base)
	require.Len(t, cookies, 1)
	assert.Equal(t, "dorgu_session", cookies[0].Name)

	api, err := client.Get(base.Scheme + "://" + base.Host + "/api/v1/meta")
	require.NoError(t, err)
	defer api.Body.Close()
	assert.Equal(t, http.StatusOK, api.StatusCode)
	var meta map[string]any
	require.NoError(t, json.NewDecoder(api.Body).Decode(&meta))
	assert.Contains(t, meta, "identity")
}

func TestTheAPIOverARealSocketRefusesACallerWithNoSession(t *testing.T) {
	printed := startServed(t)
	base, err := url.Parse(printed)
	require.NoError(t, err)

	for _, path := range []string{"/api/v1/meta", "/api/v1/stream", "/api/v1/apps"} {
		resp, err := http.Get(base.Scheme + "://" + base.Host + path)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, path)
	}
}

func TestSessionURL(t *testing.T) {
	tests := []struct{ name, base, token, want string }{
		{"adds the token", "http://127.0.0.1:9/", "abc", "http://127.0.0.1:9/?token=abc"},
		{"escapes it", "http://127.0.0.1:9/", "a&b=c", "http://127.0.0.1:9/?token=a%26b%3Dc"},
		{"no token leaves the URL alone", "http://127.0.0.1:9/", "", "http://127.0.0.1:9/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sessionURL(tc.base, tc.token))
		})
	}
}

type recordingLogger struct{ lines []string }

func (r *recordingLogger) Info(msg string, args ...any) {
	r.lines = append(r.lines, msg)
	for _, a := range args {
		if s, ok := a.(string); ok {
			r.lines = append(r.lines, s)
		}
	}
}

// Logs get pasted into issues; the token must never be in one.
func TestTheTokenIsNotLogged(t *testing.T) {
	logger := &recordingLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts := Options{Host: "127.0.0.1", OnReady: func(string) {}}.withDefaults()

	require.NoError(t, serve(ctx, opts, pingHandler(), "very-secret-token", logger))

	for _, line := range logger.lines {
		assert.NotContains(t, line, "very-secret-token")
	}
}

func TestResolveIdentityReportsTheCaller(t *testing.T) {
	client := fake.NewClientset()
	client.PrependReactor("create", "selfsubjectreviews",
		func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, &authenticationv1.SelfSubjectReview{
				Status: authenticationv1.SelfSubjectReviewStatus{UserInfo: authenticationv1.UserInfo{
					Username: "alice", Groups: []string{"system:authenticated"},
				}},
			}, nil
		})

	got := resolveIdentity(context.Background(), kube.Clients{Kubernetes: client}, quietLogger())

	assert.True(t, got.Available)
	assert.Equal(t, "alice", got.Username)
	assert.Equal(t, []string{"system:authenticated"}, got.Groups)
	assert.Empty(t, got.Reason)
}

func TestResolveIdentityDegradesToAReasonAndWarns(t *testing.T) {
	client := fake.NewClientset()
	client.PrependReactor("create", "selfsubjectreviews",
		func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("the server could not find the requested resource")
		})
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	got := resolveIdentity(context.Background(), kube.Clients{Kubernetes: client}, logger)

	assert.False(t, got.Available)
	assert.Contains(t, got.Reason, "1.28")
	assert.Contains(t, buf.String(), "could not read the authenticated identity")
}
