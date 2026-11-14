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

// Command fixture-server runs the real dashboard server over a fixture cluster.
//
// No Kubernetes cluster is involved. The store is filled by internal/fixtures
// and everything above it is the production code path: the same Snapshots, the
// same handlers, the same token and Host and Origin checks, the same SSE stream,
// and the SPA embedded from internal/webui/dist. That is what lets the Playwright
// tests and the CI screenshots exercise the whole product on a bare runner.
//
//	make web && go run ./hack/fixture-server -scenario full -port 4173
//
// then open http://127.0.0.1:4173/?token=fixture-token.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/fixtures"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/httpapi"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/sse"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/webui"
)

// fixtureToken is public on purpose: this server serves invented data on
// loopback and exists for tests.
const fixtureToken = "fixture-token"

func main() {
	scenario := flag.String("scenario", "full", fmt.Sprintf("which fixture cluster to serve: %v", fixtures.Scenarios))
	port := flag.Int("port", 4173, "port to listen on, on 127.0.0.1")
	flag.Parse()

	if err := run(*scenario, *port); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// build assembles the server over a fixture store. The assets are an argument so
// a test can serve a stub SPA without a frontend build.
func build(scenario fixtures.Scenario, assets fs.FS, now time.Time) (*httpapi.Server, *sse.Coalescer, error) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var srv *httpapi.Server
	coalescer := sse.NewCoalescer(sse.DefaultWindow, func(topic string) { srv.Publish(store.Topic(topic)) })
	cache := store.New(func(topic store.Topic) { coalescer.Mark(string(topic)) })
	fixtures.Populate(cache, scenario, now)

	srv, err := httpapi.New(httpapi.Config{
		Store:     cache,
		Snapshots: httpapi.NewSnapshots(cache, httpapi.SnapshotOptions{Context: "fixture-cluster"}),
		Broker:    sse.NewBroker(),
		Logger:    logger,
		Assets:    assets,
		Cluster:   httpapi.ClusterInfo{Context: "fixture-cluster", Server: "https://fixture.invalid"},
		Identity: httpapi.IdentityInfo{
			Available: true, Username: "alex@example.com", Groups: []string{"platform-team", "system:authenticated"},
		},
		Version: "fixture",
		Token:   fixtureToken,
	})
	return srv, coalescer, err
}

func run(scenarioName string, port int) error {
	scenario, err := fixtures.Parse(scenarioName)
	if err != nil {
		return err
	}
	assets, built := webui.Assets()
	if !built {
		return errors.New("no SPA embedded: run `make web` first")
	}
	srv, coalescer, err := build(scenario, assets, time.Now())
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("cannot listen: %w", err)
	}
	fmt.Printf("fixture dashboard (%s): http://%s/?token=%s\n", scenario, listener.Addr(), fixtureToken)
	return serve(ctx, listener, srv.Handler(), coalescer)
}

// serve runs the server on the listener until ctx is cancelled.
func serve(ctx context.Context, listener net.Listener, handler http.Handler, coalescer *sse.Coalescer) error {
	go coalescer.Run(ctx)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	errs := make(chan error, 1)
	go func() { errs <- server.Serve(listener) }()
	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
