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

// Package metrics reads live node usage from metrics-server.
//
// # Why this polls when nothing else in the process does
//
// The rest of the dashboard watches. This cannot: the metrics.k8s.io API serves
// get and list and no watch verb, because it is a window onto a sampling
// pipeline rather than a store of objects. There is no stream to subscribe to,
// so the only way to have a used figure at all is to ask on an interval.
//
// # Why a used figure is worth the exception
//
// Requested and used answer different questions and used to share one line. On
// the clean-room cluster they were 25% and 1%, which is the difference between
// "nothing more will schedule" and "what is running is barely working". Dropping
// used would leave the Cluster view stating a number that reads like utilisation
// and is not.
//
// # Why a missing answer is a first-class result
//
// metrics-server is not installed by default on any managed Kubernetes
// offering, so no answer is the ordinary case, not a failure. Every failure is
// turned into a reason string and recorded, because the one thing this must
// never do is report zero usage for a cluster nobody measured.
package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

// NodesPath is the metrics-server endpoint for per-node usage.
//
// It is read as raw JSON rather than through the k8s.io/metrics module because
// the shape needed here is two quantity strings per node. Taking the module for
// that would add a dependency whose version has to track the API server's, in
// exchange for types this file declares in eight lines.
const NodesPath = "/apis/metrics.k8s.io/v1beta1/nodes"

// DefaultInterval is how often usage is re-read.
//
// metrics-server's own default scrape period is 15 seconds and it serves the
// last sample it took, so asking more often than that returns the same numbers
// with more requests. Thirty seconds means a figure is at worst about two
// samples old, which is the right resolution for a number the UI labels with
// its age anyway.
const DefaultInterval = 30 * time.Second

// DefaultTimeout bounds one read.
//
// Short on purpose. A cluster with no metrics-server answers immediately with a
// 404 or a service-unavailable, and the case this guards is the other one: an
// APIService registered against a backend that is not answering, where the
// request hangs. Blocking the poll loop on that would stop the reason string
// from being refreshed, which is the one thing the UI needs from a failure.
const DefaultTimeout = 10 * time.Second

// Options configures the poller. Client, Store and Logger are required.
type Options struct {
	// Client is used only for its REST client, to reach an aggregated API that
	// the typed clientset has no method for.
	Client kubernetes.Interface
	// Store receives every result, success or failure.
	Store *store.Store
	// Logger receives the first failure and any change in failure reason.
	Logger *slog.Logger

	// Interval overrides DefaultInterval.
	Interval time.Duration
	// Timeout overrides DefaultTimeout.
	Timeout time.Duration
}

// Poller reads node usage on an interval and writes it to the store.
type Poller struct {
	opts Options

	// lastReason is the previous failure reason, held so a cluster with no
	// metrics-server logs once rather than every interval for as long as the
	// dashboard is open.
	lastReason string
}

// NewPoller validates the options and returns a poller.
func NewPoller(opts Options) (*Poller, error) {
	switch {
	case opts.Client == nil:
		return nil, fmt.Errorf("metrics: a Kubernetes client is required")
	case opts.Store == nil:
		return nil, fmt.Errorf("metrics: a store is required")
	case opts.Logger == nil:
		return nil, fmt.Errorf("metrics: a logger is required")
	}
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	return &Poller{opts: opts}, nil
}

// Run polls until ctx is cancelled.
//
// The first read happens immediately rather than after one interval, so the
// Cluster view has a used figure on the first paint instead of thirty seconds
// of "not measured yet" on a cluster that can measure.
func (p *Poller) Run(ctx context.Context) {
	p.once(ctx)

	ticker := time.NewTicker(p.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.once(ctx)
		}
	}
}

// once performs one read and records the outcome.
func (p *Poller) once(ctx context.Context) {
	readCtx, cancel := context.WithTimeout(ctx, p.opts.Timeout)
	defer cancel()

	usage, err := p.read(readCtx)
	if err != nil {
		// A cancelled parent is shutdown, not a metrics failure, and
		// overwriting a good reading with "context canceled" on the way out
		// would be the last thing the UI saw.
		if ctx.Err() != nil {
			return
		}
		reason := err.Error()
		p.opts.Store.SetNodeUsage(store.UnavailableNodeUsage(reason))
		if reason != p.lastReason {
			p.opts.Logger.Info("no live usage figures; the Cluster view will say why",
				"reason", reason,
				"effect", "requested is still reported; used renders as n/a with this reason",
				"hint", "install metrics-server to populate it")
			p.lastReason = reason
		}
		return
	}

	if p.lastReason != "" {
		p.opts.Logger.Info("live usage figures are available again")
		p.lastReason = ""
	}
	p.opts.Store.SetNodeUsage(usage)
}

// read fetches and sums per-node usage.
func (p *Poller) read(ctx context.Context) (store.NodeUsage, error) {
	raw, err := p.opts.Client.CoreV1().RESTClient().Get().AbsPath(NodesPath).DoRaw(ctx)
	if err != nil {
		return store.NodeUsage{}, fmt.Errorf("metrics-server did not answer: %w", err)
	}
	return ParseNodeUsage(raw, time.Now())
}

// nodeMetricsList is the part of a metrics.k8s.io NodeMetricsList this reads.
type nodeMetricsList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Usage struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	} `json:"items"`
}

// ParseNodeUsage sums per-node usage out of a NodeMetricsList.
//
// An empty list is an error rather than a zero reading. metrics-server answering
// with no nodes means it is running but has not sampled anything yet, and
// reporting that as 0% used is the exact false statement this package exists to
// avoid.
func ParseNodeUsage(raw []byte, readAt time.Time) (store.NodeUsage, error) {
	var list nodeMetricsList
	if err := json.Unmarshal(raw, &list); err != nil {
		return store.NodeUsage{}, fmt.Errorf("metrics-server sent a reply this build could not parse: %w", err)
	}
	if len(list.Items) == 0 {
		return store.NodeUsage{}, fmt.Errorf("metrics-server answered but reported no nodes")
	}

	var cpu, memory resource.Quantity
	for _, node := range list.Items {
		addQuantity(&cpu, node.Usage.CPU)
		addQuantity(&memory, node.Usage.Memory)
	}
	return store.NewNodeUsage(cpu, memory, readAt), nil
}

// addQuantity adds a quantity string to a running total, ignoring what it cannot
// parse.
//
// Treating an unparseable field as zero mirrors the CLI deliberately. Refusing
// to report usage at all because one field on one node is malformed would trade
// a slightly low number for no number, and the figure is labelled with its age
// and its source either way.
func addQuantity(total *resource.Quantity, value string) {
	if value == "" {
		return
	}
	parsed, err := resource.ParseQuantity(value)
	if err != nil {
		return
	}
	total.Add(parsed)
}
