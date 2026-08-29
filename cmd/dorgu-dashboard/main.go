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

// Command dorgu-dashboard serves the Dorgu dashboard as a standalone binary.
//
// It is a thin wrapper: flags in, pkg/dashboard.Run out. The `dorgu` CLI imports
// that same package for `dorgu dashboard`, so there is one implementation and
// this command cannot drift from it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dorgu-ai/dorgu-platform-v1/pkg/dashboard"
)

// version is stamped by the build. See the Makefile.
var version = dashboard.DefaultVersion

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		kubeconfig    = flag.String("kubeconfig", "", "path to kubeconfig (default: $KUBECONFIG, then ~/.kube/config)")
		kubeContext   = flag.String("context", "", "kubeconfig context to use (default: the current context)")
		namespace     = flag.String("namespace", "", "limit every view to one namespace (default: all namespaces)")
		host          = flag.String("host", dashboard.DefaultHost, "bind address; anything other than loopback exposes an unauthenticated view of your cluster")
		port          = flag.Int("port", dashboard.DefaultPort, "bind port; 0 asks the kernel for a free one")
		allowHost     = flag.String("allow-host", "", "comma-separated Host header allowlist, required when --host is not loopback")
		incidentLimit = flag.Int("incident-limit", 0, "cap the incident feed; 0 uses the default, -1 means no cap")
		planLimit     = flag.Int("remediation-limit", 0, "cap the remediation list; 0 uses the default, -1 means no cap")
		metricsEvery  = flag.Duration("metrics-interval", 0, "how often to read node usage from metrics-server; 0 uses the default")
		logLevel      = flag.String("log-level", "info", "log level: debug, info, warn or error")
		showVersion   = flag.Bool("version", false, "print the version and exit")
	)

	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	level, err := parseLevel(*logLevel)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = dashboard.Run(ctx, dashboard.Options{
		Kubeconfig:       *kubeconfig,
		Context:          *kubeContext,
		Namespace:        *namespace,
		Host:             *host,
		Port:             *port,
		AllowedHosts:     splitList(*allowHost),
		IncidentLimit:    *incidentLimit,
		RemediationLimit: *planLimit,
		MetricsInterval:  *metricsEvery,
		Version:          version,
		Logger:           logger,
		OnReady: func(url string) {
			// stdout, not the logger: this is the one line a person is waiting
			// for, and it should survive --log-level=error and be pipeable.
			fmt.Printf("Dorgu dashboard: %s\n", url)
			fmt.Println("Reading your cluster with your kubeconfig. Press Ctrl-C to stop.")
		},
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprint(out, `dorgu-dashboard serves a local, read-only view of what Dorgu sees in your cluster.

It uses your kubeconfig, holds no credentials, needs no permissions of its own,
and binds to localhost. It can do exactly what you can do and nothing more.

Usage:
  dorgu-dashboard [flags]

Flags:
`)
	flag.PrintDefaults()
	fmt.Fprint(out, `
Examples:
  dorgu-dashboard
  dorgu-dashboard --namespace apps
  dorgu-dashboard --context staging --port 0
`)
}

func parseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q; use debug, info, warn or error", name)
	}
}

// splitList turns a comma-separated flag into a slice, dropping empties so a
// trailing comma is not a hostname.
func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
