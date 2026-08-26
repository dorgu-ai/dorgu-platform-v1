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
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// typedHandler adapts a dynamic informer's unstructured events into typed
// api/v1 objects.
//
// The CRD Go types are imported from the operator rather than restated, so this
// conversion is the whole of the mapping: there is no hand-written field list
// to fall out of date when a CRD gains a field. A dynamic informer is used
// instead of a generated clientset because kubebuilder projects like the
// operator do not publish one, and re-deriving one here would be the
// hand-syncing the plan rejected Rust for.
func typedHandler[T any](
	put func(*T),
	del func(namespace, name string),
	onError func(error),
) cache.ResourceEventHandler {
	convert := func(obj any) {
		typed, err := toTyped[T](obj)
		if err != nil {
			onError(err)
			return
		}
		put(typed)
	}

	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { convert(obj) },
		UpdateFunc: func(_, obj any) { convert(obj) },
		DeleteFunc: func(obj any) {
			namespace, name, err := namespacedName(obj)
			if err != nil {
				onError(err)
				return
			}
			del(namespace, name)
		},
	}
}

// nativeHandler wires an already-typed informer, which needs no conversion.
func nativeHandler[T any](
	put func(*T),
	del func(namespace, name string),
	onError func(error),
) cache.ResourceEventHandler {
	accept := func(obj any) {
		typed, ok := obj.(*T)
		if !ok {
			onError(fmt.Errorf("informer delivered %T, want %T", obj, new(T)))
			return
		}
		put(typed)
	}

	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { accept(obj) },
		UpdateFunc: func(_, obj any) { accept(obj) },
		DeleteFunc: func(obj any) {
			namespace, name, err := namespacedName(obj)
			if err != nil {
				onError(err)
				return
			}
			del(namespace, name)
		},
	}
}

// toTyped converts an informer object into T.
func toTyped[T any](obj any) (*T, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		if typed, isTyped := obj.(*T); isTyped {
			return typed, nil
		}
		return nil, fmt.Errorf("informer delivered %T, want *unstructured.Unstructured", obj)
	}

	out := new(T)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, out); err != nil {
		return nil, fmt.Errorf("converting %s %s/%s: %w",
			u.GetKind(), u.GetNamespace(), u.GetName(), err)
	}
	return out, nil
}

// namespacedName reads the identity of a deleted object.
//
// A delete can arrive as a DeletedFinalStateUnknown tombstone when the watch was
// interrupted and the informer noticed the object was gone during a relist. The
// tombstone carries the last known object, so unwrapping it is the difference
// between removing the row and leaving a deleted app on screen forever.
func namespacedName(obj any) (string, string, error) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return "", "", fmt.Errorf("reading metadata of deleted %T: %w", obj, err)
	}
	return accessor.GetNamespace(), accessor.GetName(), nil
}
