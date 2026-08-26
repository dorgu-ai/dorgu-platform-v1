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

// Package dashboard runs the Dorgu dashboard.
//
// It is the package the `dorgu` CLI imports so that `dorgu dashboard` is one
// command with no second thing to install, and it is what cmd/dorgu-dashboard
// wraps for the standalone binary and container image. Both surfaces call Run,
// so there is one startup path and no chance of the CLI and the binary behaving
// differently.
//
// # The trust posture, in code
//
// This package creates no credentials. It loads the caller's kubeconfig, binds
// loopback by default, and exposes read-only endpoints. It can do exactly what
// the person running it can do, and nothing more.
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/httpapi"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/informers"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/sse"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/webui"
)

// shutdownGrace is how long in-flight requests get to finish on shutdown.
//
// SSE streams are long-lived and will not finish on their own, so the grace
// period is short: their context is cancelled with the server's and they return
// immediately. This exists for the REST handlers.
const shutdownGrace = 3 * time.Second

// Run starts the dashboard and blocks until ctx is cancelled or serving fails.
//
// The startup order is deliberate:
//
//  1. connect, so an unreachable cluster fails before a port is taken
//  2. discover CRDs, so a watch is never started against a resource the API
//     server does not serve
//  3. warm the informer caches, so the first page render is the truth rather
//     than an empty table
//  4. bind and serve
//
// Step 3 is why this can take a moment on a large cluster, and why it is worth
// it: the alternative is showing "no apps" to someone who has two hundred.
func Run(ctx context.Context, opts Options) error {
	cfg := opts.withDefaults()
	logger := cfg.Logger

	if err := cfg.Validate(logger); err != nil {
		return err
	}

	conn, err := kube.Connect(kube.ConnectOptions{
		Kubeconfig: cfg.Kubeconfig,
		Context:    cfg.Context,
	})
	if err != nil {
		return fmt.Errorf("connecting to your cluster: %w", err)
	}
	logger.Info("connected", "context", conn.Context, "server", conn.Server, "inCluster", conn.InCluster)

	clients, err := kube.NewClients(conn)
	if err != nil {
		return err
	}

	resources, err := kube.DiscoverDorguResources(clients.Discovery)
	if err != nil {
		return fmt.Errorf("checking which dorgu.io resources your cluster serves: %w", err)
	}
	if missing := resources.Missing(); len(missing) > 0 {
		logger.Warn("some dorgu.io CRDs are not installed; the views that need them will say so",
			"missing", missing,
			"hint", "install the dorgu operator to populate them")
	}

	// The coalescer flushes through the server, and the store notifies the
	// coalescer, so one of the three has to be wired after the fact. It is safe:
	// the coalescer only calls flush from Run, which starts after the server
	// exists, and Mark before then merely records a dirty topic.
	window := cfg.PushWindow
	if window == 0 {
		window = sse.DefaultWindow
	}
	var srv *httpapi.Server
	coalescer := sse.NewCoalescer(window, func(topic string) {
		srv.Publish(store.Topic(topic))
	})

	cache := store.New(func(topic store.Topic) { coalescer.Mark(string(topic)) })
	broker := sse.NewBroker()

	assets, built := webui.Assets()
	if !built {
		logger.Warn("no SPA embedded in this binary; the API works and the root path explains why",
			"hint", "run make web && make build")
	}

	srv, err = httpapi.New(httpapi.Config{
		Store: cache,
		Snapshots: httpapi.NewSnapshots(cache, httpapi.SnapshotOptions{
			Namespace:     cfg.Namespace,
			IncidentLimit: cfg.IncidentLimit,
		}),
		Broker: broker,
		Logger: logger,
		Assets: assets,
		Cluster: httpapi.ClusterInfo{
			Context:   conn.Context,
			Server:    conn.Server,
			Namespace: cfg.Namespace,
			InCluster: conn.InCluster,
		},
		Version:      cfg.Version,
		Heartbeat:    cfg.Heartbeat,
		AllowedHosts: cfg.AllowedHosts,
	})
	if err != nil {
		return err
	}

	set, err := informers.New(clients, cache, informers.Options{
		Namespace: cfg.Namespace,
		Resync:    cfg.Resync,
		Resources: resources,
		Logger:    logger,
	})
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go coalescer.Run(runCtx)

	logger.Info("warming caches", "informers", set.Watched())
	if err := set.Start(runCtx); err != nil {
		return fmt.Errorf("watching your cluster: %w", err)
	}

	return serve(runCtx, cfg, srv.Handler(), logger)
}

// serve binds the listener and runs the HTTP server until ctx is cancelled.
//
// The listener is created before Serve so a port already in use fails with a
// message naming the address, rather than after the caller has been told the
// dashboard is starting.
func serve(ctx context.Context, cfg Options, handler http.Handler, logger logAdapter) error {
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", address, err)
	}

	server := &http.Server{
		Handler: handler,
		// The SSE stream never finishes, so there is no write timeout to set.
		// A header timeout still applies and is the one that protects against a
		// client opening a connection and sending nothing.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	url := "http://" + net.JoinHostPort(displayHost(cfg.Host), strconv.Itoa(port(listener))) + "/"
	logger.Info("dashboard ready", "url", url)
	cfg.OnReady(url)

	errs := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errs <- err
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("shutting down: %w", err)
		}
		return nil
	}
}

// displayHost turns a bind address into something a person can click. A wildcard
// bind is reported as localhost, because that is the address that will actually
// work from the machine the message is being read on.
func displayHost(host string) string {
	switch host {
	case "", "0.0.0.0", "::":
		return "localhost"
	default:
		return host
	}
}

// port reads the port actually bound, which is what makes Port 0 usable.
func port(listener net.Listener) int {
	if addr, ok := listener.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

// logAdapter is the slice of *slog.Logger serve uses, declared where it is used
// so that serve can be exercised with a stub.
type logAdapter interface {
	Info(msg string, args ...any)
}
