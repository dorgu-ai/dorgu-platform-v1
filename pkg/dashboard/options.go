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
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"
)

// Defaults for Options. Every one of them is a decision, not a placeholder.
const (
	// DefaultHost is loopback. The dashboard has no authentication, by the
	// Aug 22 decision, and the reason that decision is defensible is that
	// nothing outside the machine can reach it. Changing this is changing the
	// security model, which is why Options.Host says so and Validate warns.
	DefaultHost = "127.0.0.1"

	// DefaultPort is a fixed high port so the URL is the same every run and can
	// be bookmarked. Port 0 asks the kernel for a free one, which is what tests
	// and a second concurrent dashboard want.
	DefaultPort = 7171

	// DefaultVersion is reported when no build stamped a version in.
	DefaultVersion = "dev"
)

// Options configures a dashboard. The zero value is usable: it reads the
// caller's default kubeconfig and serves on 127.0.0.1:7171.
type Options struct {
	// Kubeconfig is an explicit kubeconfig path. Empty uses the standard
	// loading rules, which is $KUBECONFIG if set and ~/.kube/config otherwise.
	Kubeconfig string
	// Context overrides the kubeconfig current context.
	Context string
	// Namespace scopes every view and every informer. Empty watches all
	// namespaces, which is what a cluster-admin kubeconfig can do; set it when
	// the caller's RBAC is namespace-scoped, because a cluster-wide watch will
	// otherwise be refused and the views will stay empty.
	Namespace string

	// Host is the bind address. Anything other than a loopback address exposes
	// an unauthenticated view of the cluster to the network.
	Host string
	// Port is the bind port. Zero asks the kernel for a free one; read the
	// actual URL from OnReady.
	Port int
	// AllowedHosts overrides the Host header allowlist. Only needed when
	// binding to a non-loopback address on purpose.
	AllowedHosts []string

	// IncidentLimit caps the incident feed. Zero uses the view default;
	// negative means no cap. A capped feed reports the cap in its payload.
	IncidentLimit int

	// Resync is the informer resync period. Zero uses the informer default.
	Resync time.Duration
	// Heartbeat is the SSE keep-alive interval. Zero uses the server default.
	Heartbeat time.Duration
	// PushWindow is how long changes are gathered before a snapshot is pushed.
	// Zero uses the coalescer default.
	PushWindow time.Duration

	// Version is reported by the meta endpoint and shown in the UI footer.
	Version string
	// Logger receives startup, watch and decode failures. Nil logs warnings and
	// above to stderr in text form, which is what a CLI subcommand wants.
	Logger *slog.Logger

	// OnReady is called once the listener is up, with the URL to open. It is
	// how the CLI prints or opens the address without this package deciding to
	// do either.
	OnReady func(url string)
}

// withDefaults returns a copy with every unset field filled in. It does not
// mutate the receiver: callers hold onto their Options and a run must not
// rewrite them.
func (o Options) withDefaults() Options {
	filled := o
	if filled.Host == "" {
		filled.Host = DefaultHost
	}
	if filled.Version == "" {
		filled.Version = DefaultVersion
	}
	if filled.Logger == nil {
		filled.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))
	}
	if filled.OnReady == nil {
		filled.OnReady = func(string) {}
	}
	return filled
}

// Validate reports a configuration that cannot work, and warns about one that
// works but changes the security posture.
//
// The warning is not a formality. A dashboard on 0.0.0.0 with no auth hands
// every reader on the network a live view of the cluster, and the person who
// typed --host deserves to be told that in words rather than discover it.
func (o Options) Validate(logger *slog.Logger) error {
	if o.Port < 0 || o.Port > 65535 {
		return fmt.Errorf("port %d is out of range 0-65535", o.Port)
	}

	host := o.Host
	if host == "" {
		host = DefaultHost
	}
	if isLoopback(host) {
		return nil
	}

	if logger != nil {
		logger.Warn("binding to a non-loopback address exposes an unauthenticated view of your cluster",
			"host", host,
			"detail", "the dashboard has no authentication by design; loopback is what keeps that safe",
			"hint", "drop --host to bind 127.0.0.1, or put an authenticating proxy in front of this")
	}
	if len(o.AllowedHosts) == 0 {
		return fmt.Errorf(
			"host %q is not a loopback address, so the Host header allowlist must be set explicitly "+
				"(AllowedHosts / --allow-host); refusing to serve to unknown hostnames", host)
	}
	return nil
}

// isLoopback reports whether a bind address only accepts local connections.
// The empty string and "0.0.0.0" are not loopback: they bind every interface.
func isLoopback(host string) bool {
	switch host {
	case "localhost":
		return true
	case "", "0.0.0.0", "::":
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
