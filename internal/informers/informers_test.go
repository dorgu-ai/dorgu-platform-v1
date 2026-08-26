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

package informers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/kube"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// listKinds tells the fake dynamic client the list kind for each watched
// resource. Without it the fake cannot serve a LIST and every informer times out
// waiting to sync.
func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		kube.GVR(kube.ResourceApplicationPersonas): "ApplicationPersonaList",
		kube.GVR(kube.ResourceClusterPersonas):     "ClusterPersonaList",
		kube.GVR(kube.ResourceIncidentMemories):    "IncidentMemoryList",
		kube.GVR(kube.ResourceRemediationActions):  "RemediationActionList",
		kube.GVR(kube.ResourceDorguEvents):         "DorguEventList",
	}
}

// allPresent is every dorgu.io resource installed, with the real scope of each.
func allPresent() kube.Resources {
	out := kube.Resources{}
	for _, r := range kube.DorguResources {
		out[r] = kube.ResourceInfo{
			Present:    true,
			Namespaced: r != kube.ResourceClusterPersonas,
		}
	}
	return out
}

// onlyPresent installs the named resources and nothing else.
func onlyPresent(names ...string) kube.Resources {
	out := kube.Resources{}
	for _, name := range names {
		out[name] = kube.ResourceInfo{
			Present:    true,
			Namespaced: name != kube.ResourceClusterPersonas,
		}
	}
	return out
}

func unstructuredPersona(namespace, name, appName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "dorgu.io/v1",
		"kind":       "ApplicationPersona",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec":       map[string]any{"name": appName, "type": "api"},
	}}
}

func unstructuredIncident(namespace, name, signal string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "dorgu.io/v1",
		"kind":       "IncidentMemory",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec": map[string]any{
			"personaRef": map[string]any{
				"kind": "ApplicationPersona", "name": "checkout", "namespace": namespace,
			},
			"category":  "resource",
			"severity":  "critical",
			"detection": map[string]any{"signal": signal, "source": "pod-failure-detector"},
		},
	}}
}

type fixture struct {
	set     *Set
	store   *store.Store
	typed   *kubefake.Clientset
	dynamic *dynamicfake.FakeDynamicClient
}

func newFixture(t *testing.T, present kube.Resources, objects ...runtime.Object) *fixture {
	t.Helper()

	var native []runtime.Object
	var custom []runtime.Object
	for _, obj := range objects {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			custom = append(custom, u)
			continue
		}
		native = append(native, obj)
	}

	typed := kubefake.NewClientset(native...)
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), listKinds(), custom...)

	cache := store.New(nil)
	set, err := New(kube.Clients{Kubernetes: typed, Dynamic: dyn}, cache, Options{
		Resources: present,
		Logger:    quietLogger(),
		Resync:    time.Hour,
	})
	require.NoError(t, err)

	return &fixture{set: set, store: cache, typed: typed, dynamic: dyn}
}

func (f *fixture) start(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, f.set.Start(ctx))
	return ctx
}

// ---------------------------------------------------------------------------
// construction
// ---------------------------------------------------------------------------

func TestNewRequiresALoggerAndClients(t *testing.T) {
	cache := store.New(nil)
	clients := kube.Clients{Kubernetes: kubefake.NewClientset(), Dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}

	_, err := New(clients, cache, Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")

	_, err = New(kube.Clients{}, cache, Options{Logger: quietLogger()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dynamic client are required")
}

// A watch against a CRD that is not installed only ever produces errors, so the
// informer is never created and the absence is recorded instead.
func TestAbsentCRDsGetNoInformerAndAreRecordedAsMissing(t *testing.T) {
	f := newFixture(t, onlyPresent(kube.ResourceApplicationPersonas))

	watched := f.set.Watched()
	assert.Contains(t, watched, NameDeployments)
	assert.Contains(t, watched, NamePods)
	assert.Contains(t, watched, kube.ResourceApplicationPersonas)
	assert.NotContains(t, watched, kube.ResourceIncidentMemories)

	assert.True(t, f.store.CRDPresent(kube.ResourceApplicationPersonas))
	assert.False(t, f.store.CRDPresent(kube.ResourceIncidentMemories))
}

func TestEveryInstalledResourceGetsAnInformer(t *testing.T) {
	f := newFixture(t, allPresent())

	watched := f.set.Watched()
	assert.Len(t, watched, len(kube.DorguResources)+2, "the five CRDs plus Deployments and Pods")
	for _, r := range kube.DorguResources {
		assert.Contains(t, watched, r)
	}
}

func TestWatchedReturnsACopy(t *testing.T) {
	f := newFixture(t, allPresent())

	first := f.set.Watched()
	first[0] = "scribbled-on"
	assert.NotEqual(t, "scribbled-on", f.set.Watched()[0])
}

// ---------------------------------------------------------------------------
// the pipeline, end to end
// ---------------------------------------------------------------------------

// Serving a page before the caches are warm would show an empty Apps table on a
// cluster full of apps.
func TestStartHydratesTheStoreAndMarksEveryInformerSynced(t *testing.T) {
	replicas := int32(2)
	f := newFixture(t, allPresent(),
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout-abc"}},
		unstructuredPersona("apps", "checkout", "checkout"),
		unstructuredIncident("apps", "checkout-oom", "OOMKilled"),
	)
	f.start(t)

	assert.True(t, f.store.SyncedAll())
	for _, name := range f.set.Watched() {
		assert.True(t, f.store.Synced(name), name)
	}

	require.Len(t, f.store.Deployments(), 1)
	assert.Equal(t, "checkout", f.store.Deployments()[0].Name)
	require.Len(t, f.store.Pods(), 1)

	personas := f.store.AppPersonas()
	require.Len(t, personas, 1)
	assert.Equal(t, "checkout", personas[0].Spec.Name)
	assert.Equal(t, "api", personas[0].Spec.Type)

	incidents := f.store.Incidents()
	require.Len(t, incidents, 1)
	assert.Equal(t, "OOMKilled", incidents[0].Spec.Detection.Signal)
	assert.Equal(t, "critical", incidents[0].Spec.Severity)
}

// A change after sync has to reach the store on its own, with nothing polling.
func TestALiveCreateReachesTheStore(t *testing.T) {
	f := newFixture(t, allPresent())
	ctx := f.start(t)

	_, err := f.dynamic.Resource(kube.GVR(kube.ResourceIncidentMemories)).
		Namespace("apps").
		Create(ctx, unstructuredIncident("apps", "checkout-oom", "CrashLoopBackOff"), metav1.CreateOptions{})
	require.NoError(t, err)

	require.Eventually(t, func() bool { return len(f.store.Incidents()) == 1 },
		10*time.Second, 20*time.Millisecond, "the watch never delivered the new incident")
	assert.Equal(t, "CrashLoopBackOff", f.store.Incidents()[0].Spec.Detection.Signal)
}

func TestALiveDeleteRemovesItFromTheStore(t *testing.T) {
	f := newFixture(t, allPresent(), unstructuredPersona("apps", "checkout", "checkout"))
	ctx := f.start(t)
	require.Len(t, f.store.AppPersonas(), 1)

	require.NoError(t, f.dynamic.Resource(kube.GVR(kube.ResourceApplicationPersonas)).
		Namespace("apps").Delete(ctx, "checkout", metav1.DeleteOptions{}))

	assert.Eventually(t, func() bool { return len(f.store.AppPersonas()) == 0 },
		10*time.Second, 20*time.Millisecond, "a deleted app stayed on screen")
}

func TestALiveDeploymentUpdateReachesTheStore(t *testing.T) {
	replicas := int32(2)
	f := newFixture(t, allPresent(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 2},
	})
	ctx := f.start(t)

	updated := f.store.Deployments()[0]
	updated.Status.ReadyReplicas = 0
	_, err := f.typed.AppsV1().Deployments("apps").Update(ctx, updated, metav1.UpdateOptions{})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		stored := f.store.Deployments()
		return len(stored) == 1 && stored[0].Status.ReadyReplicas == 0
	}, 10*time.Second, 20*time.Millisecond, "a Deployment going unhealthy never arrived")
}

// The store notifies its callback per change, and that is what feeds the SSE
// coalescer. A quiet informer would mean a browser that never updates.
func TestInformerChangesNotifyTheStoreCallback(t *testing.T) {
	notified := make(chan store.Topic, 128)
	cache := store.New(func(topic store.Topic) {
		select {
		case notified <- topic:
		default:
		}
	})

	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), listKinds())
	set, err := New(
		kube.Clients{Kubernetes: kubefake.NewClientset(), Dynamic: dyn},
		cache,
		Options{Resources: allPresent(), Logger: quietLogger(), Resync: time.Hour},
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, set.Start(ctx))

	// Drain the notifications the CRD-presence and sync bookkeeping produced.
	for len(notified) > 0 {
		<-notified
	}

	_, err = dyn.Resource(kube.GVR(kube.ResourceIncidentMemories)).Namespace("apps").
		Create(ctx, unstructuredIncident("apps", "checkout-oom", "OOMKilled"), metav1.CreateOptions{})
	require.NoError(t, err)

	deadline := time.After(10 * time.Second)
	seen := map[store.Topic]bool{}
	for !seen[store.TopicIncidents] || !seen[store.TopicApps] {
		select {
		case topic := <-notified:
			seen[topic] = true
		case <-deadline:
			t.Fatalf("expected apps and incidents notifications; saw %v", seen)
		}
	}
}

func TestStartFailsWhenTheContextIsCancelled(t *testing.T) {
	f := newFixture(t, allPresent())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := f.set.Start(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled while syncing",
		"a shutdown is reported as a shutdown, not as a sync failure")
}

// A watch that can never succeed must not hang startup with a closed port.
// client-go retries a refused watch indefinitely and is right to, but a
// dashboard that never opens its port is a worse answer than one that opens and
// reports the view as not ready.
func TestAnInformerThatNeverSyncsDoesNotHangStartup(t *testing.T) {
	cache := store.New(nil)

	// LIST on incidentmemories is refused, which is what a namespace-scoped
	// kubeconfig user gets for a cluster-wide watch and the likeliest cause of
	// this state on a real cluster.
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), listKinds())
	dyn.PrependReactor("list", kube.ResourceIncidentMemories,
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(
				kube.GVR(kube.ResourceIncidentMemories).GroupResource(), "",
				errors.New("incidentmemories is forbidden at the cluster scope"))
		})
	set, err := New(
		kube.Clients{Kubernetes: kubefake.NewClientset(), Dynamic: dyn},
		cache,
		Options{
			Resources: onlyPresent(
				kube.ResourceApplicationPersonas, kube.ResourceIncidentMemories),
			Logger:      quietLogger(),
			Resync:      time.Hour,
			SyncTimeout: 300 * time.Millisecond,
		},
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	start := time.Now()
	require.NoError(t, set.Start(ctx), "startup must succeed so the server can serve")
	assert.Less(t, time.Since(start), 10*time.Second, "the sync wait has to be bounded")

	assert.True(t, cache.Synced(NameDeployments))
	assert.True(t, cache.Synced(kube.ResourceApplicationPersonas))
	assert.False(t, cache.Synced(kube.ResourceIncidentMemories),
		"the unsynced informer stays marked unsynced, so its view reports itself not ready")
	assert.False(t, cache.SyncedAll())
}

// ClusterPersona is cluster-scoped. Watching it through the namespace-filtered
// factory makes its LIST fail outright, which used to hang startup entirely
// whenever --namespace was passed.
func TestNamespaceScopeDoesNotBreakTheClusterScopedCRD(t *testing.T) {
	cache := store.New(nil)
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), listKinds())

	set, err := New(
		kube.Clients{Kubernetes: kubefake.NewClientset(), Dynamic: dyn},
		cache,
		Options{
			Namespace:   "apps",
			Resources:   allPresent(),
			Logger:      quietLogger(),
			Resync:      time.Hour,
			SyncTimeout: 10 * time.Second,
		},
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, set.Start(ctx))

	assert.True(t, cache.Synced(kube.ResourceClusterPersonas),
		"a cluster-scoped resource must be watched cluster-wide even under a namespace scope")
	assert.True(t, cache.SyncedAll())
}

// The dashboard has exactly the caller's RBAC, so a namespace-scoped user is a
// real case and the informers have to be scopeable to match.
func TestNamespaceScopeIsAppliedToTheInformers(t *testing.T) {
	replicas := int32(1)
	typed := kubefake.NewClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: "staging", Name: "checkout"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	cache := store.New(nil)
	set, err := New(
		kube.Clients{
			Kubernetes: typed,
			Dynamic:    dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds()),
		},
		cache,
		Options{Namespace: "apps", Resources: kube.Resources{}, Logger: quietLogger(), Resync: time.Hour},
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, set.Start(ctx))

	stored := cache.Deployments()
	require.Len(t, stored, 1)
	assert.Equal(t, "apps", stored[0].Namespace)
}

// Pods enter the cache through the transform, so the trimming has to hold for
// real informer traffic and not only for a direct call.
func TestPodsEnterTheCacheTrimmed(t *testing.T) {
	f := newFixture(t, kube.Resources{}, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:     "apps",
			Name:          "checkout-abc",
			Labels:        map[string]string{"app": "checkout"},
			Annotations:   map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{...}"},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubelet"}},
		},
	})
	f.start(t)

	pods := f.store.Pods()
	require.Len(t, pods, 1)
	assert.Nil(t, pods[0].ManagedFields)
	assert.Nil(t, pods[0].Annotations)
	assert.Equal(t, map[string]string{"app": "checkout"}, pods[0].Labels)
}
