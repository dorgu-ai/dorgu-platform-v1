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

package view

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// Turning a patch into a diff a person can read.
//
// A step carries a JSON merge patch and, when the operator recorded one, the
// snapshot of the fields it will overwrite. Rendering those as two blocks of
// YAML and asking the reader to spot the difference is what a terminal has to
// do; a browser can put the field, the old value and the new value on one line.
//
// Both diffs in this view are built from the PATCH, never from a guardrail's
// requested value. A field a guardrail refused is removed from the patch
// entirely, so it cannot appear in a diff: the payload cannot advertise a change
// that will not happen, and that is a property of where the data comes from
// rather than a rule this code has to remember to apply.

// diffPatch renders one step's patch as field-level changes against its
// pre-patch snapshot.
//
// Ordering is by path, so two renders of the same plan read the same way.
func diffPatch(patch, prePatch *apiextensionsv1.JSON) []ResourceChange {
	after := flattenJSON(patch)
	if len(after) == 0 {
		return nil
	}
	before := flattenJSON(prePatch)

	paths := make([]string, 0, len(after))
	for path := range after {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	changes := make([]ResourceChange, 0, len(paths))
	for _, path := range paths {
		newValue := after[path]
		oldValue, existed := before[path]

		change := ResourceChange{Path: path, Before: oldValue, After: newValue}
		if !existed {
			// No snapshot for this field. That is genuinely two different
			// situations and neither can be told from the other here: the
			// operator recorded no pre-patch state at all, or it recorded one
			// and this field was not in it. Saying "not set" is the reading that
			// cannot overstate what is known, and Added is what carries the
			// weight in the UI.
			change.Before = AbsentResourceValue
			change.Added = true
		}
		change.Changed = change.After != oldValue || !existed
		changes = append(changes, change)
	}
	return changes
}

// flattenJSON turns a nested JSON object into dotted paths with scalar values.
//
// Arrays are indexed rather than skipped, because a plan that sets
// spec.env[0].value is setting something the reader has to see. A nil or
// unparseable payload flattens to nothing: this is a display path, and a patch
// this build cannot read is better shown as no rows than as a wrong row.
func flattenJSON(raw *apiextensionsv1.JSON) map[string]string {
	if raw == nil || len(raw.Raw) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw.Raw, &decoded); err != nil {
		return nil
	}

	out := map[string]string{}
	flattenValue("", decoded, out)
	return out
}

func flattenValue(prefix string, value any, out map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			// An empty object at the root is a patch that sets nothing, and it
			// gets no row: a row whose field path is the empty string reads as a
			// rendering fault. Nested, it is a field being set to {} and is a
			// change worth showing.
			if prefix != "" {
				out[prefix] = "{}"
			}
			return
		}
		for key, nested := range typed {
			flattenValue(joinPath(prefix, key), nested, out)
		}
	case []any:
		if len(typed) == 0 {
			if prefix != "" {
				out[prefix] = "[]"
			}
			return
		}
		for i, nested := range typed {
			flattenValue(prefix+"["+strconv.Itoa(i)+"]", nested, out)
		}
	default:
		// A bare scalar at the root is not a merge patch and names no field, so
		// it gets no row for the same reason.
		if prefix != "" {
			out[prefix] = scalarString(typed)
		}
	}
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// scalarString renders a JSON scalar the way it was written.
//
// A number goes through strconv rather than fmt's %v so 512 does not render as
// 512.000000 or 5.12e+02, and a JSON null renders as the word rather than as an
// empty string: a patch setting a field to null is removing it, which is a
// change worth seeing.
func scalarString(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// resourceIntent is the container resource change a plan's patches add up to:
// only the keys they actually set, so nothing broader than the proposal is ever
// shown as changing.
type resourceIntent struct {
	limits   map[string]string
	requests map[string]string
}

func (r resourceIntent) isEmpty() bool {
	return len(r.limits) == 0 && len(r.requests) == 0
}

// extractResourceIntent reads the container resource change out of a
// persona-update patch.
//
// It mirrors the CLI's extractResourceChange, including accepting a patch
// written with or without the "spec" wrapper, because both shapes are in
// clusters.
func extractResourceIntent(patch *apiextensionsv1.JSON) resourceIntent {
	if patch == nil || len(patch.Raw) == 0 {
		return resourceIntent{}
	}
	var decoded map[string]any
	if err := json.Unmarshal(patch.Raw, &decoded); err != nil {
		return resourceIntent{}
	}
	if spec, ok := decoded["spec"].(map[string]any); ok {
		decoded = spec
	}
	resources, ok := decoded["resources"].(map[string]any)
	if !ok {
		return resourceIntent{}
	}
	return resourceIntent{
		limits:   stringMap(resources["limits"]),
		requests: stringMap(resources["requests"]),
	}
}

// stringMap keeps only the string-valued entries of a decoded JSON object.
//
// A resource quantity is always a string in this API. A non-string value is a
// malformed patch, and dropping the one field beats discarding the whole plan.
func stringMap(value any) map[string]string {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(object))
	for key, nested := range object {
		if text, ok := nested.(string); ok {
			out[key] = text
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// mergeResourceIntent returns a new intent with b merged onto a, b winning on
// conflict. Neither input is mutated.
func mergeResourceIntent(a, b resourceIntent) resourceIntent {
	out := resourceIntent{limits: map[string]string{}, requests: map[string]string{}}
	for _, source := range []resourceIntent{a, b} {
		for key, value := range source.limits {
			out.limits[key] = value
		}
		for key, value := range source.requests {
			out.requests[key] = value
		}
	}
	return out
}

// resourceKeyOrder is the fixed render order for the container diff, so two
// renders of the same plan read the same way.
var resourceKeyOrder = []struct {
	kind string
	name string
}{
	{"limits", "cpu"},
	{"limits", "memory"},
	{"requests", "cpu"},
	{"requests", "memory"},
}

// diffWorkloadResources renders what a plan does to the running container.
//
// This is the diff that matters, and it is grounded in the observed workload
// rather than in the persona. Comparing persona to persona is what made a
// remediation that introduced a CPU limit the container had never had render as
// no change at all: the persona was not the thing being changed, so it could not
// be the thing diffed.
func diffWorkloadResources(workload *dorguv1.WorkloadRef, intent resourceIntent) []ResourceChange {
	if intent.isEmpty() {
		return nil
	}

	observed := workload != nil && workload.ObservedResources != nil
	absent := AbsentResourceValue
	if !observed {
		absent = UnobservedResourceValue
	}

	var changes []ResourceChange
	for _, key := range resourceKeyOrder {
		before := observedResourceValue(workload, key.kind, key.name)
		after := proposedResourceValue(intent, key.kind, key.name)
		if before == "" && after == "" {
			continue
		}

		change := ResourceChange{
			Path:   "resources." + key.kind + "." + key.name,
			Before: before,
			After:  after,
		}
		if before == "" {
			change.Before = absent
			// Added only where Dorgu actually looked and found nothing. Without
			// an observation there is no basis for claiming a key is new.
			change.Added = after != "" && observed
		}
		change.Changed = after != "" && after != before
		changes = append(changes, change)
	}
	return changes
}

// observedResourceValue reads one key off the live observation, returning "" when
// the workload does not set it.
func observedResourceValue(workload *dorguv1.WorkloadRef, kind, name string) string {
	if workload == nil || workload.ObservedResources == nil {
		return ""
	}
	var values *dorguv1.ResourceValues
	switch kind {
	case "limits":
		values = workload.ObservedResources.Limits
	case "requests":
		values = workload.ObservedResources.Requests
	}
	if values == nil {
		return ""
	}
	if name == "cpu" {
		return values.CPU
	}
	return values.Memory
}

// proposedResourceValue reads one key out of the plan's intent, returning "" when
// the plan does not touch it.
func proposedResourceValue(intent resourceIntent, kind, name string) string {
	if kind == "limits" {
		return intent.limits[name]
	}
	return intent.requests[name]
}
