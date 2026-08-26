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

// Package kube connects to the cluster the user is already pointed at.
//
// The dashboard holds no credentials of its own. It loads the same kubeconfig
// kubectl would load, honours the same KUBECONFIG variable and the same current
// context, and therefore has exactly the permissions the person running it has.
// There is no service account, no token file and no stored secret anywhere in
// this package, which is what makes the trust claim in the README checkable
// rather than a promise.
package kube

import (
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Connection is a resolved cluster connection plus the facts about it a user
// needs in order to confirm they are looking at the cluster they meant.
//
// Showing the context and server in the UI is not decoration. A dashboard that
// does not say which cluster it is describing is a dashboard you cannot trust
// when you have four of them.
type Connection struct {
	// Config is the rest config every client is built from.
	Config *rest.Config
	// Context is the kubeconfig context name in use, empty when the connection
	// came from in-cluster credentials.
	Context string
	// Server is the API server URL.
	Server string
	// InCluster reports that the connection came from a mounted service
	// account rather than a kubeconfig.
	InCluster bool
}

// ConnectOptions selects which cluster to talk to.
type ConnectOptions struct {
	// Kubeconfig is an explicit kubeconfig path. Empty uses the standard
	// loading rules: $KUBECONFIG if set, otherwise ~/.kube/config.
	Kubeconfig string
	// Context overrides the current context. Empty uses the current context.
	Context string
	// QPS and Burst bound the client's request rate. The informers list once
	// and then watch, so these only ever bite during startup on a cluster with
	// a lot of objects, where the default of 5 QPS makes the first paint slow
	// for no reason.
	QPS   float32
	Burst int
}

// Default client rate limits. Raised well above client-go's defaults because
// this process issues a handful of list calls at startup and then nothing: the
// default 5 QPS exists to protect the API server from a controller in a hot
// loop, which this is not.
const (
	DefaultQPS   = 50
	DefaultBurst = 100
)

// Connect resolves a cluster connection, preferring an explicit kubeconfig and
// falling back to in-cluster credentials.
//
// Every failure names the file it tried, because "unable to load
// configuration" with no path in it is the least useful error a Kubernetes tool
// can produce.
func Connect(opts ConnectOptions) (*Connection, error) {
	loading := clientcmd.NewDefaultClientConfigLoadingRules()
	if opts.Kubeconfig != "" {
		loading.ExplicitPath = opts.Kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if opts.Context != "" {
		overrides.CurrentContext = opts.Context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, overrides)

	config, kubeconfigErr := clientConfig.ClientConfig()
	if kubeconfigErr == nil {
		contextName, err := currentContext(clientConfig, opts.Context)
		if err != nil {
			return nil, err
		}
		applyRateLimits(config, opts)
		return &Connection{Config: config, Context: contextName, Server: config.Host}, nil
	}

	// No usable kubeconfig. In-cluster is the architected-for-later case from
	// the plan, and supporting it here costs nothing; it is never the reason a
	// local run works, because a local run has no service account mounted.
	inCluster, inClusterErr := rest.InClusterConfig()
	if inClusterErr != nil {
		return nil, fmt.Errorf("no usable Kubernetes configuration: %w (in-cluster fallback also failed: %v)",
			kubeconfigErr, inClusterErr)
	}
	applyRateLimits(inCluster, opts)
	return &Connection{Config: inCluster, Server: inCluster.Host, InCluster: true}, nil
}

func currentContext(clientConfig clientcmd.ClientConfig, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	raw, err := clientConfig.RawConfig()
	if err != nil {
		return "", fmt.Errorf("reading kubeconfig contexts: %w", err)
	}
	return raw.CurrentContext, nil
}

// Clients is the set of typed and dynamic clients the informers need.
//
// It is returned as interfaces rather than concrete types so the informer set
// can be driven by client-go's fakes in tests. That is not a courtesy to the
// tests: the informer wiring is where a watch on the wrong resource or a handler
// on the wrong kind hides, and neither is visible without running it.
type Clients struct {
	Kubernetes kubernetes.Interface
	Dynamic    dynamic.Interface
	Discovery  discovery.DiscoveryInterface
}

// NewClients builds every client from a resolved connection.
func NewClients(conn *Connection) (Clients, error) {
	if conn == nil || conn.Config == nil {
		return Clients{}, fmt.Errorf("kube: a resolved connection is required")
	}

	typed, err := kubernetes.NewForConfig(conn.Config)
	if err != nil {
		return Clients{}, fmt.Errorf("building Kubernetes client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(conn.Config)
	if err != nil {
		return Clients{}, fmt.Errorf("building dynamic client: %w", err)
	}
	disco, err := discovery.NewDiscoveryClientForConfig(conn.Config)
	if err != nil {
		return Clients{}, fmt.Errorf("building discovery client: %w", err)
	}

	return Clients{Kubernetes: typed, Dynamic: dyn, Discovery: disco}, nil
}

func applyRateLimits(config *rest.Config, opts ConnectOptions) {
	config.QPS = opts.QPS
	if config.QPS <= 0 {
		config.QPS = DefaultQPS
	}
	config.Burst = opts.Burst
	if config.Burst <= 0 {
		config.Burst = DefaultBurst
	}
	config.UserAgent = rest.DefaultKubernetesUserAgent() + " dorgu-dashboard"
}
