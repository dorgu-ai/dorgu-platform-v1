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

// Package informers watches the cluster and writes what it sees into the store.
//
// Watch, never poll. The API server pushes changes over long-lived watches, so
// the dashboard's latency is one network hop from the change happening, and the
// cost of an extra browser tab is zero API calls. Polling would have made both
// worse in exchange for nothing.
//
// There is exactly one exception in this process and it is not here: node usage
// from metrics-server, in internal/metrics. The metrics.k8s.io API serves get
// and list and no watch verb, so there is nothing to watch. Everything a
// Kubernetes API can stream, this package streams.
package informers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

// Informer names, used as the store's sync keys and in log lines.
const (
	NameDeployments = "deployments"
	NamePods        = "pods"
	NameNodes       = "nodes"
)

// DefaultResync is how often the informers re-deliver their whole cache.
//
// Watches are the real update path and a resync corrects nothing that a watch
// would miss; it exists as a safety net against a bug in this process that
// dropped an event. Ten minutes is cheap because a resync reads the local cache
// and issues no API calls.
const DefaultResync = 10 * time.Minute

// DefaultSyncTimeout bounds the wait for the initial list.
//
// It exists because an informer that can never sync would otherwise hang startup
// forever with nothing serving: a watch refused by RBAC retries indefinitely,
// and client-go is right to retry, but a dashboard that never opens its port is
// a worse answer than one that opens it and says which view is not ready. Thirty
// seconds is generous for a cold list on a large cluster.
const DefaultSyncTimeout = 30 * time.Second

// Options configures the informer set.
type Options struct {
	// Namespace scopes every informer. Empty watches all namespaces.
	Namespace string
	// Resync is the informer resync period. Zero uses DefaultResync.
	Resync time.Duration
	// Resources is what discovery found: which dorgu.io resources the API
	// server serves and how each one is scoped. An absent resource gets no
	// informer, because a watch against a CRD that is not installed only ever
	// produces errors.
	Resources kube.Resources
	// SyncTimeout bounds how long Start waits for the initial list. Zero uses
	// DefaultSyncTimeout.
	SyncTimeout time.Duration
	// Logger receives conversion and watch failures. Required.
	Logger *slog.Logger
}

// Set is the running collection of informers.
type Set struct {
	core coreinformers.SharedInformerFactory
	// scoped watches namespaced resources, honouring Options.Namespace.
	scoped dynamicinformer.DynamicSharedInformerFactory
	// clusterWide watches cluster-scoped resources. ClusterPersona is one, and
	// a namespaced list against it fails outright, so it needs its own factory
	// rather than the namespace filter.
	clusterWide dynamicinformer.DynamicSharedInformerFactory

	logger      *slog.Logger
	store       *store.Store
	syncTimeout time.Duration

	// names are the informers actually registered, which is what cache sync is
	// waited on. It excludes CRDs that are not installed.
	names []string
	// shared holds every registered informer so watch error handlers can be
	// attached before start.
	shared []cache.SharedIndexInformer
}

// New builds the informer set and registers its event handlers.
//
// Handlers write straight into the store. There is no queue between them,
// because there is no work to rate-limit: the store write is a map assignment
// and the notification it triggers is coalesced downstream.
func New(clients kube.Clients, cache *store.Store, opts Options) (*Set, error) {
	if opts.Logger == nil {
		return nil, fmt.Errorf("informers: logger is required")
	}
	if clients.Kubernetes == nil || clients.Dynamic == nil {
		return nil, fmt.Errorf("informers: a Kubernetes client and a dynamic client are required")
	}
	resync := opts.Resync
	if resync == 0 {
		resync = DefaultResync
	}
	namespace := opts.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceAll
	}

	syncTimeout := opts.SyncTimeout
	if syncTimeout == 0 {
		syncTimeout = DefaultSyncTimeout
	}

	set := &Set{
		core: coreinformers.NewSharedInformerFactoryWithOptions(clients.Kubernetes, resync,
			coreinformers.WithNamespace(namespace),
			coreinformers.WithTransform(trimForCache),
		),
		scoped: dynamicinformer.NewFilteredDynamicSharedInformerFactory(
			clients.Dynamic, resync, namespace, nil),
		clusterWide: dynamicinformer.NewFilteredDynamicSharedInformerFactory(
			clients.Dynamic, resync, metav1.NamespaceAll, nil),
		logger:      opts.Logger,
		store:       cache,
		syncTimeout: syncTimeout,
	}

	set.registerNative()
	set.registerDorgu(opts.Resources)
	return set, nil
}

// registerNative wires the three built-in kinds the views need.
//
// Deployments carry the ownership evidence (labels, annotations, managedFields)
// and the resource block. Pods carry the container states that make a crash loop
// visible before the operator has written anything about it, and the resource
// requests the Cluster view's saturation figure is the sum of.
//
// Nodes are the allocatable pool that saturation is measured against. They are
// watched rather than read from ClusterPersona.status on purpose: that field is
// written on a reconcile interval by whatever operator version is installed, and
// the version that computed it wrongly is still in clusters. The dashboard is
// the surface where a wrong number reads as authoritative, so it does the
// arithmetic itself, from the same two lists the CLI uses.
//
// The namespace filter on the core factory does not reach the Node informer:
// client-go's NewFilteredNodeInformer takes no namespace, because a Node is
// cluster-scoped. So a --namespace run still watches Nodes, and a kubeconfig
// that may not list them fails the watch rather than silently listing none.
func (s *Set) registerNative() {
	deployments := s.core.Apps().V1().Deployments().Informer()
	s.add(NameDeployments, deployments, nativeHandler[appsv1.Deployment](
		s.store.PutDeployment, s.store.DeleteDeployment, s.errorFor(NameDeployments)))

	pods := s.core.Core().V1().Pods().Informer()
	s.add(NamePods, pods, nativeHandler[corev1.Pod](
		s.store.PutPod, s.store.DeletePod, s.errorFor(NamePods)))

	nodes := s.core.Core().V1().Nodes().Informer()
	s.add(NameNodes, nodes, nativeHandler[corev1.Node](
		s.store.PutNode, s.store.DeleteNode, s.errorFor(NameNodes)))
}

// registerDorgu wires an informer per installed dorgu.io CRD and records which
// ones were missing, so a view can say "the operator is not installed" instead
// of showing an empty table.
func (s *Set) registerDorgu(resources kube.Resources) {
	for _, resource := range kube.DorguResources {
		present := resources.Present(resource)
		s.store.SetCRDPresent(resource, present)
		if !present {
			s.logger.Warn("dorgu.io resource is not served by this cluster; not watching it",
				"resource", resource,
				"hint", "install the dorgu operator to populate this view")
			continue
		}

		// The scope decides which factory watches it. A cluster-scoped resource
		// listed with a namespace is answered with "the server could not find
		// the requested resource", so getting this wrong does not degrade the
		// view, it stops the informer syncing at all.
		factory := s.clusterWide
		if resources.Namespaced(resource) {
			factory = s.scoped
		}
		s.add(resource, factory.ForResource(kube.GVR(resource)).Informer(), s.handlerFor(resource))
	}
}

func (s *Set) handlerFor(resource string) cache.ResourceEventHandler {
	onError := s.errorFor(resource)
	switch resource {
	case kube.ResourceApplicationPersonas:
		return typedHandler[dorguv1.ApplicationPersona](
			s.store.PutAppPersona, s.store.DeleteAppPersona, onError)
	case kube.ResourceClusterPersonas:
		return typedHandler[dorguv1.ClusterPersona](
			s.store.PutClusterPersona, s.store.DeleteClusterPersona, onError)
	case kube.ResourceIncidentMemories:
		return typedHandler[dorguv1.IncidentMemory](
			s.store.PutIncident, s.store.DeleteIncident, onError)
	case kube.ResourceRemediationActions:
		return typedHandler[dorguv1.RemediationAction](
			s.store.PutRemediation, s.store.DeleteRemediation, onError)
	case kube.ResourceDorguEvents:
		return typedHandler[dorguv1.DorguEvent](
			s.store.PutDorguEvent, s.store.DeleteDorguEvent, onError)
	default:
		// Unreachable while this switch covers kube.DorguResources. Kept as a
		// loud failure rather than a silent skip so adding a sixth CRD without
		// a handler cannot ship as "the view is just empty".
		return cache.ResourceEventHandlerFuncs{
			AddFunc: func(any) {
				s.logger.Error("no handler registered for dorgu.io resource", "resource", resource)
			},
		}
	}
}

func (s *Set) add(name string, informer cache.SharedIndexInformer, handler cache.ResourceEventHandler) {
	if _, err := informer.AddEventHandler(handler); err != nil {
		// AddEventHandler only errors on an already-stopped informer, which
		// cannot happen here because nothing has been started yet.
		s.logger.Error("registering informer handler", "informer", name, "error", err)
		return
	}
	s.names = append(s.names, name)
	s.shared = append(s.shared, informer)
	s.store.SetSynced(name, false)
}

// Start runs every informer and blocks until each has completed its initial
// list, or ctx is cancelled.
//
// Waiting matters. Serving a page before the caches are warm would show an empty
// Apps table on a cluster full of apps, and the payload's readiness flag exists
// precisely so that state is never mistaken for an answer.
func (s *Set) Start(ctx context.Context) error {
	for i, informer := range s.shared {
		name := s.names[i]
		if err := informer.SetWatchErrorHandler(s.watchErrorHandler(name)); err != nil {
			s.logger.Warn("could not install watch error handler", "informer", name, "error", err)
		}
	}

	s.core.Start(ctx.Done())
	s.scoped.Start(ctx.Done())
	s.clusterWide.Start(ctx.Done())

	syncCtx, cancel := context.WithTimeout(ctx, s.syncTimeout)
	defer cancel()

	for i, informer := range s.shared {
		if cache.WaitForCacheSync(syncCtx.Done(), informer.HasSynced) {
			s.store.SetSynced(s.names[i], true)
			s.logger.Debug("informer synced", "informer", s.names[i])
			continue
		}

		if ctx.Err() != nil {
			// Shutdown, not a sync failure.
			return fmt.Errorf("cancelled while syncing informer %s: %w", s.names[i], ctx.Err())
		}

		// The timeout fired. Serving anyway beats hanging with a closed port,
		// but "not synced" on its own would leave the view on a loading
		// skeleton for as long as the process runs: the screen would say it was
		// reading a cluster it had already given up on reading. So the reason is
		// recorded on the store and rendered in the view. The likeliest cause on
		// a real cluster is RBAC, and a namespace-scoped kubeconfig cannot list
		// Nodes at all, which the Cluster view now says in words.
		reason := fmt.Sprintf("the initial list did not complete within %s, "+
			"which usually means your kubeconfig user may not list or watch this resource",
			s.syncTimeout)
		s.store.SetSyncFailed(s.names[i], reason)
		s.logger.Error("informer did not complete its initial list; serving without it",
			"informer", s.names[i],
			"timeout", s.syncTimeout,
			"effect", "the views that need it say so on screen instead of showing an empty table",
			"hint", "check that your kubeconfig user may list and watch this resource")
	}
	return nil
}

// Watched lists the informers that were registered.
func (s *Set) Watched() []string {
	out := make([]string, len(s.names))
	copy(out, s.names)
	return out
}

// watchErrorHandler reports a broken watch rather than retrying in silence.
//
// client-go retries on its own, and it is right to, but a watch that keeps
// failing means the UI is quietly going stale. The user hears about it: a
// forbidden watch is the single most likely failure on a real cluster, because
// the dashboard has exactly the caller's RBAC and a namespace-scoped user cannot
// list cluster-wide.
func (s *Set) watchErrorHandler(name string) cache.WatchErrorHandler {
	return func(_ *cache.Reflector, err error) {
		if err == nil {
			return
		}
		s.logger.Error("watch failed; this view will go stale until it recovers",
			"informer", name,
			"error", err,
			"hint", "check that your kubeconfig user may list and watch this resource")
	}
}

func (s *Set) errorFor(name string) func(error) {
	return func(err error) {
		s.logger.Error("dropping an object the informer could not decode",
			"informer", name, "error", err)
	}
}

// trimForCache drops the parts of an object no view reads, before it enters the
// cache.
//
// Pods are by far the most numerous object watched and managedFields on a Pod is
// routinely larger than the parts of the spec anyone looks at. A Node's
// managedFields and its annotations are similarly large and similarly unread:
// the Cluster view needs allocatable, capacity, conditions, taints and labels,
// and labels are kept because that is where the role comes from.
//
// Ownership detection needs managedFields on Deployments, which this
// deliberately leaves untouched. Anything else passes through unchanged, so a
// kind added later is trimmed only when someone decides what it does not need.
func trimForCache(obj any) (any, error) {
	switch typed := obj.(type) {
	case *corev1.Pod:
		typed.ManagedFields = nil
		typed.Annotations = nil
		return typed, nil
	case *corev1.Node:
		typed.ManagedFields = nil
		typed.Annotations = nil
		return typed, nil
	default:
		return obj, nil
	}
}

// compile-time assertion that trimForCache matches the transform signature.
var _ cache.TransformFunc = trimForCache

var _ runtime.Object = (*corev1.Pod)(nil)
