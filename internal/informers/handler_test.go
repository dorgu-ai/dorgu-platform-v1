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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// The CRD Go types are imported from the operator, so this conversion is the
// whole of the mapping. There is no hand-written field list to fall out of date
// when a CRD gains a field, which is exactly the hand-syncing the plan rejected.
func TestTypedHandlerConvertsUnstructuredIntoTheOperatorsOwnTypes(t *testing.T) {
	raw := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "dorgu.io/v1",
		"kind":       "IncidentMemory",
		"metadata":   map[string]any{"name": "checkout-oom", "namespace": "apps"},
		"spec": map[string]any{
			"personaRef": map[string]any{
				"kind": "ApplicationPersona", "name": "checkout", "namespace": "apps",
			},
			"category":    "resource",
			"severity":    "critical",
			"attribution": "persona",
			"detection": map[string]any{
				"signal":            "OOMKilled",
				"source":            "pod-failure-detector",
				"firstSeen":         "2026-08-27T03:00:00Z",
				"lastSeen":          "2026-08-27T03:05:00Z",
				"affectedResources": []any{},
			},
			"rootCause": map[string]any{
				"summary":    "Memory limit is below the working set",
				"confidence": "0.85",
				"provider":   "ai-enhanced",
			},
		},
		"status": map[string]any{"phase": "Detected", "occurrenceCount": int64(3)},
	}}

	var got *dorguv1.IncidentMemory
	handler := typedHandler[dorguv1.IncidentMemory](
		func(i *dorguv1.IncidentMemory) { got = i },
		func(string, string) { t.Fatal("no delete expected") },
		func(err error) { t.Fatalf("conversion failed: %v", err) },
	)
	handler.OnAdd(raw, false)

	require.NotNil(t, got)
	assert.Equal(t, "checkout-oom", got.Name)
	assert.Equal(t, "apps", got.Namespace)
	assert.Equal(t, "critical", got.Spec.Severity)
	assert.Equal(t, "OOMKilled", got.Spec.Detection.Signal)
	require.NotNil(t, got.Spec.RootCause)
	assert.Equal(t, "0.85", got.Spec.RootCause.Confidence)
	assert.Equal(t, "ai-enhanced", got.Spec.RootCause.Provider)
	assert.Equal(t, "Detected", got.Status.Phase)
	assert.Equal(t, int32(3), got.Status.OccurrenceCount)
	assert.False(t, got.Spec.Detection.FirstSeen.IsZero(), "timestamps must survive conversion")
}

func TestTypedHandlerTreatsUpdateAsAPut(t *testing.T) {
	raw := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "dorgu.io/v1",
		"kind":       "ApplicationPersona",
		"metadata":   map[string]any{"name": "checkout", "namespace": "apps"},
		"spec":       map[string]any{"name": "checkout", "type": "api"},
	}}

	puts := 0
	handler := typedHandler[dorguv1.ApplicationPersona](
		func(*dorguv1.ApplicationPersona) { puts++ },
		func(string, string) {},
		func(err error) { t.Fatal(err) },
	)

	handler.OnAdd(raw, false)
	handler.OnUpdate(raw, raw)
	assert.Equal(t, 2, puts)
}

// A field the CRD does not declare must not fail the conversion: it is how an
// older dashboard keeps working against a newer operator.
func TestUnknownFieldsAreIgnoredRatherThanFatal(t *testing.T) {
	raw := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "dorgu.io/v1",
		"kind":       "ApplicationPersona",
		"metadata":   map[string]any{"name": "checkout", "namespace": "apps"},
		"spec": map[string]any{
			"name": "checkout", "type": "api",
			"somethingFromTheFuture": "value",
		},
	}}

	var got *dorguv1.ApplicationPersona
	handler := typedHandler[dorguv1.ApplicationPersona](
		func(p *dorguv1.ApplicationPersona) { got = p },
		func(string, string) {},
		func(err error) { t.Fatalf("an unknown field must not be fatal: %v", err) },
	)
	handler.OnAdd(raw, false)

	require.NotNil(t, got)
	assert.Equal(t, "checkout", got.Spec.Name)
}

// A wrongly typed field is reported and the object dropped, rather than a zero
// value being written into the cache as though it were real data.
func TestAnUnconvertibleObjectIsReportedNotSilentlyDropped(t *testing.T) {
	raw := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "dorgu.io/v1",
		"kind":       "ApplicationPersona",
		"metadata":   map[string]any{"name": "checkout"},
		"spec":       map[string]any{"name": []any{"not", "a", "string"}},
	}}

	var reported error
	handler := typedHandler[dorguv1.ApplicationPersona](
		func(*dorguv1.ApplicationPersona) { t.Fatal("a broken object must not be stored") },
		func(string, string) {},
		func(err error) { reported = err },
	)
	handler.OnAdd(raw, false)

	require.Error(t, reported)
	assert.Contains(t, reported.Error(), "ApplicationPersona")
}

func TestNativeHandlerPassesTypedObjectsStraightThrough(t *testing.T) {
	original := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: "apps"},
	}

	var got *appsv1.Deployment
	handler := nativeHandler[appsv1.Deployment](
		func(d *appsv1.Deployment) { got = d },
		func(string, string) {},
		func(err error) { t.Fatal(err) },
	)
	handler.OnAdd(original, false)

	assert.Same(t, original, got, "no copy here: the store makes one")
}

func TestNativeHandlerReportsTheWrongType(t *testing.T) {
	var reported error
	handler := nativeHandler[appsv1.Deployment](
		func(*appsv1.Deployment) { t.Fatal("must not accept a Pod as a Deployment") },
		func(string, string) {},
		func(err error) { reported = err },
	)
	handler.OnAdd(&corev1.Pod{}, false)

	require.Error(t, reported)
	assert.Contains(t, reported.Error(), "want *v1.Deployment")
}

func TestDeleteReportsTheNamespacedName(t *testing.T) {
	var namespace, name string
	handler := nativeHandler[appsv1.Deployment](
		func(*appsv1.Deployment) {},
		func(ns, n string) { namespace, name = ns, n },
		func(err error) { t.Fatal(err) },
	)

	handler.OnDelete(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: "apps"},
	})
	assert.Equal(t, "apps", namespace)
	assert.Equal(t, "checkout", name)
}

// A tombstone arrives when the watch was interrupted and the informer noticed
// the object was gone during a relist. Unwrapping it is the difference between
// removing the row and leaving a deleted app on screen forever.
func TestATombstoneDeleteStillRemovesTheRow(t *testing.T) {
	var namespace, name string
	handler := typedHandler[dorguv1.ApplicationPersona](
		func(*dorguv1.ApplicationPersona) {},
		func(ns, n string) { namespace, name = ns, n },
		func(err error) { t.Fatal(err) },
	)

	handler.OnDelete(cache.DeletedFinalStateUnknown{
		Key: "apps/checkout",
		Obj: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "dorgu.io/v1",
			"kind":       "ApplicationPersona",
			"metadata":   map[string]any{"name": "checkout", "namespace": "apps"},
		}},
	})

	assert.Equal(t, "apps", namespace)
	assert.Equal(t, "checkout", name)
}

func TestADeleteWithNoReadableMetadataIsReported(t *testing.T) {
	var reported error
	handler := nativeHandler[appsv1.Deployment](
		func(*appsv1.Deployment) {},
		func(string, string) { t.Fatal("nothing to delete") },
		func(err error) { reported = err },
	)

	handler.OnDelete("not an object at all")
	require.Error(t, reported)
	assert.Contains(t, reported.Error(), "reading metadata of deleted")
}

// Pods are the most numerous object watched, and managedFields on a Pod is
// routinely larger than the parts of the spec anyone looks at.
func TestTrimPodDropsWhatNoViewReads(t *testing.T) {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:          "checkout-abc",
			Namespace:     "apps",
			Labels:        map[string]string{"app": "checkout"},
			Annotations:   map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{...}"},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubelet"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	out, err := trimPod(p)
	require.NoError(t, err)

	trimmed, ok := out.(*corev1.Pod)
	require.True(t, ok)
	assert.Nil(t, trimmed.ManagedFields)
	assert.Nil(t, trimmed.Annotations)
	assert.Equal(t, map[string]string{"app": "checkout"}, trimmed.Labels,
		"labels are how a Pod is tied to its Deployment")
	assert.Equal(t, corev1.PodRunning, trimmed.Status.Phase)
}

// Ownership detection needs managedFields on Deployments, so the transform must
// leave anything that is not a Pod completely alone.
func TestTrimPodLeavesDeploymentManagedFieldsIntact(t *testing.T) {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:          "checkout",
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "helm"}},
			Annotations:   map[string]string{"meta.helm.sh/release-name": "checkout"},
		},
	}

	out, err := trimPod(d)
	require.NoError(t, err)
	assert.Same(t, d, out)
	assert.Len(t, d.ManagedFields, 1, "ownership evidence must survive")
	assert.NotEmpty(t, d.Annotations)
}
