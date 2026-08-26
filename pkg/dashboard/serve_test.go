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
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func pingHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
}

// Port 0 is what a second concurrent dashboard and every test wants, and it only
// works if the actual bound port is reported back.
func TestServeReportsTheActualPortWhenAskedForAFreeOne(t *testing.T) {
	ready := make(chan string, 1)
	opts := Options{
		Host:    "127.0.0.1",
		Port:    0,
		OnReady: func(url string) { ready <- url },
	}.withDefaults()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errs := make(chan error, 1)
	go func() { errs <- serve(ctx, opts, pingHandler(), quietLogger()) }()

	var url string
	select {
	case url = <-ready:
	case err := <-errs:
		t.Fatalf("serve returned early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("OnReady was never called")
	}

	assert.True(t, strings.HasPrefix(url, "http://127.0.0.1:"), "got %q", url)
	assert.False(t, strings.HasSuffix(url, ":0/"), "the reported port must be the bound one")

	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(body))

	cancel()
	select {
	case err := <-errs:
		assert.NoError(t, err, "a cancelled context is a clean shutdown, not a failure")
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not shut down")
	}
}

// A port already in use has to fail with the address in the message, before the
// caller has been told the dashboard is starting.
func TestServeNamesTheAddressItCouldNotBind(t *testing.T) {
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer blocker.Close()

	taken := blocker.Addr().(*net.TCPAddr).Port
	called := false
	opts := Options{
		Host:    "127.0.0.1",
		Port:    taken,
		OnReady: func(string) { called = true },
	}.withDefaults()

	err = serve(context.Background(), opts, pingHandler(), quietLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot listen on 127.0.0.1:")
	assert.False(t, called, "OnReady must not fire for a bind that failed")
}

func TestServeStopsWhenTheContextIsAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := Options{Host: "127.0.0.1", Port: 0}.withDefaults()
	assert.NoError(t, serve(ctx, opts, pingHandler(), quietLogger()))
}

// The Host guard lives in the HTTP layer, and this asserts it is actually
// mounted on the path Run wires up rather than only in that package's own tests.
func TestServedHandlerRejectsAForeignHostHeader(t *testing.T) {
	ready := make(chan string, 1)
	opts := Options{Host: "127.0.0.1", Port: 0, OnReady: func(u string) { ready <- u }}.withDefaults()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler, err := testHandler()
	require.NoError(t, err)
	go func() { _ = serve(ctx, opts, handler, quietLogger()) }()

	url := <-ready
	req, err := http.NewRequest(http.MethodGet, url+"api/v1/meta", nil)
	require.NoError(t, err)
	req.Host = "dorgu.attacker.example"

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
