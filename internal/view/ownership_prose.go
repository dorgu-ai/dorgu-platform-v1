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
	"fmt"
	"strings"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// What a workload's owner means for what Dorgu will do, in words.
//
// Ported from the CLI's remediation_workload.go so a refusal the CLI prints and
// a refusal this screen shows send the reader to the same place with the same
// reasoning. If the two ever disagree about who owns a Deployment or where its
// desired state lives, the reader learns that Dorgu does not know, which is
// worse than either answer.
//
// The sentences are deliberately about consequence rather than policy. "Dorgu
// will not patch this" reads as a limitation; "a direct patch would make your
// next helm upgrade fail on a field-manager conflict" reads as competence, and
// it is the same fact.

// ownerName names the owner, preferring the specific detail the operator
// recorded over the generic category.
func ownerName(workload *dorguv1.WorkloadRef) string {
	if workload == nil {
		return "an owner Dorgu could not identify"
	}
	if detail := strings.TrimSpace(workload.ManagedByDetail); detail != "" {
		return detail
	}
	switch workload.ManagedBy {
	case dorguv1.ManagedByHelm:
		return "a Helm release"
	case dorguv1.ManagedByArgoCD:
		return "an ArgoCD application"
	case dorguv1.ManagedByFlux:
		return "a Flux controller"
	case dorguv1.ManagedByKustomize:
		return "a kustomize overlay"
	default:
		return "an owner Dorgu could not identify"
	}
}

// whyNotPatched is the one plain line explaining the refusal.
func whyNotPatched(workload *dorguv1.WorkloadRef) string {
	owner := ownerName(workload)
	managedBy := ""
	if workload != nil {
		managedBy = workload.ManagedBy
	}

	switch managedBy {
	case dorguv1.ManagedByHelm:
		return fmt.Sprintf("A direct patch would claim the fields it sets away from %s, and your next "+
			"helm upgrade would then fail with a field-manager conflict.", owner)
	case dorguv1.ManagedByArgoCD:
		return fmt.Sprintf("A direct patch would be reverted on the next sync by %s, or rejected "+
			"outright under server-side apply.", owner)
	case dorguv1.ManagedByFlux:
		return fmt.Sprintf("A direct patch would be reverted the next time %s reconciles this "+
			"Deployment.", owner)
	case dorguv1.ManagedByKustomize:
		return fmt.Sprintf("A direct patch would be overwritten the next time %s is applied.", owner)
	case dorguv1.ManagedByUnmanaged:
		return ""
	default:
		if !workloadObserved(workload) {
			return "Dorgu has no record of the live workload behind this remediation, so it cannot tell " +
				"what owns it. No record is treated as owned."
		}
		return "Dorgu could not identify what manages this Deployment, and an unseen owner is still an " +
			"owner: a patch that collides with one breaks their next deploy. Unknown is treated as owned."
	}
}

// ownerChangeLocation names where an owned workload's desired state lives, and
// how a change there reaches the cluster.
//
// It hedges where Dorgu genuinely does not know: a chart's values key is
// chart-specific and Dorgu has not read the chart. Naming a key it has not seen
// would be the confident wrong answer this product exists to avoid.
func ownerChangeLocation(workload *dorguv1.WorkloadRef) string {
	owner := ownerName(workload)
	managedBy := ""
	if workload != nil {
		managedBy = workload.ManagedBy
	}

	switch managedBy {
	case dorguv1.ManagedByHelm:
		return fmt.Sprintf("the values for %s (the key is chart-specific, commonly under `resources`), "+
			"then run your usual helm upgrade", owner)
	case dorguv1.ManagedByArgoCD:
		return fmt.Sprintf("the Git manifests for %s, then commit and let ArgoCD sync", owner)
	case dorguv1.ManagedByFlux:
		return fmt.Sprintf("the Git source reconciled by %s, then commit and let Flux reconcile it", owner)
	case dorguv1.ManagedByKustomize:
		return "your kustomize overlay for this Deployment, then re-apply the overlay"
	case dorguv1.ManagedByUnmanaged:
		return ""
	default:
		if !workloadObserved(workload) {
			return "whatever manages this application"
		}
		return fmt.Sprintf("whatever manages Deployment %s", workload.Name)
	}
}

// workloadObserved reports whether the operator actually resolved a live
// workload. An unresolved ref still exists, with ManagedBy "unknown", but it
// names nothing.
func workloadObserved(workload *dorguv1.WorkloadRef) bool {
	return workload != nil && workload.Name != "" && workload.Namespace != ""
}

// ownerFallbackInstruction is the one instruction to give when the plan carries
// no owner-shaped step of its own: the concrete keys and values, and where to set
// them, so a refusal is never a dead end.
func ownerFallbackInstruction(workload *dorguv1.WorkloadRef, intent resourceIntent) string {
	where := ownerChangeLocation(workload)
	if where == "" {
		where = "wherever this workload's desired state lives"
	}
	fields := changedFieldList(intent)
	if fields == "" {
		return fmt.Sprintf("Make this change in %s.", where)
	}
	return fmt.Sprintf("Set %s in %s.", fields, where)
}

// changedFieldList renders the changed keys in the fixed key order, e.g.
// "resources.limits.memory: 512Mi, resources.limits.cpu: 100m".
//
// It reads the intent, which is built from the post-guardrail patches, so a value
// a guardrail refused is not in it and cannot be handed to somebody about to type
// it into a values file.
func changedFieldList(intent resourceIntent) string {
	if intent.isEmpty() {
		return ""
	}
	parts := make([]string, 0, len(resourceKeyOrder))
	for _, key := range resourceKeyOrder {
		if value := proposedResourceValue(intent, key.kind, key.name); value != "" {
			parts = append(parts, fmt.Sprintf("resources.%s.%s: %s", key.kind, key.name, value))
		}
	}
	return strings.Join(parts, ", ")
}
