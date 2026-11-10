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
	// DefaultHost is loopback. Nothing outside the machine can reach it, and
	// the session token (see Options.Token) is what separates the person who
	// started the dashboard from any other local process or browser tab.
	// Changing the host is changing the security model, which is why
	// Options.Host says so and Validate warns.
	DefaultHost = "127.0.0.1"

	// DefaultPort is 0: the kernel picks a free port every run. A fixed port is
	// one a hostile page can guess and probe for, and two dashboards pointed at
	// two clusters cannot share one. The URL is printed and opened for you, so
	// nothing is lost; pass a port to get a stable one.
	DefaultPort = 0

	// DefaultVersion is reported when no build stamped a version in.
	DefaultVersion = "dev"
)

// Options configures a dashboard. The zero value is usable: it reads the
// caller's default kubeconfig and serves on 127.0.0.1 on a random free port,
// behind a freshly generated session token.
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

	// Host is the bind address. Anything other than a loopback address puts the
	// dashboard on the network, protected only by the session token.
	Host string
	// Port is the bind port. Zero, the default, asks the kernel for a free one;
	// read the actual URL from OnReady.
	Port int
	// Token is the session token every API request must present. Empty, the
	// default, generates a fresh random one, which is what every caller should
	// want. Set it only to make a run reproducible in tests.
	Token string
	// AllowedHosts overrides the Host header allowlist. Only needed when
	// binding to a non-loopback address on purpose.
	AllowedHosts []string

	// IncidentLimit caps the incident feed. Zero uses the view default;
	// negative means no cap. A capped feed reports the cap in its payload.
	IncidentLimit int
	// RemediationLimit caps the remediation list, the same way. Zero uses the
	// view default; negative means no cap.
	RemediationLimit int

	// Resync is the informer resync period. Zero uses the informer default.
	Resync time.Duration
	// Heartbeat is the SSE keep-alive interval. Zero uses the server default.
	Heartbeat time.Duration
	// PushWindow is how long changes are gathered before a snapshot is pushed.
	// Zero uses the coalescer default.
	PushWindow time.Duration
	// MetricsInterval is how often node usage is re-read from metrics-server.
	// Zero uses the poller default.
	//
	// It is the one interval in this configuration, because it is the one thing
	// the dashboard polls: the metrics API serves no watch verb, so there is
	// nothing to subscribe to.
	MetricsInterval time.Duration

	// Version is reported by the meta endpoint and shown in the UI footer.
	Version string
	// Logger receives startup, watch and decode failures. Nil logs warnings and
	// above to stderr in text form, which is what a CLI subcommand wants.
	Logger *slog.Logger

	// OnReady is called once the listener is up, with the URL to open. The URL
	// carries the session token in its query: the first request exchanges it
	// for a cookie and the server redirects it out of the address bar. It is
	// how the CLI prints or opens the address without this package deciding to
	// do either. It is a secret for the length of the run; do not log it.
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
// The warning is not a formality. A dashboard on 0.0.0.0 puts a live view of
// the cluster, read with the caller's own credentials, on the network behind
// nothing but a token that travels in clear text over HTTP, and the person who
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
		logger.Warn("WARNING: binding to a non-loopback address exposes your cluster to the network",
			"host", host,
			"detail", "anyone who can reach this port and obtains the session token reads your cluster "+
				"with your kubeconfig's permissions, and the token travels in clear text over plain HTTP",
			"hint", "drop --host to bind 127.0.0.1, or put a TLS-terminating proxy in front of this")
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
