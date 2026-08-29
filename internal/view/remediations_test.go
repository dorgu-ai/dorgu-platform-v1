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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

func rawJSON(text string) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(text)}
}

// remediation builds a RemediationAction with the shape the operator writes.
func remediation(
	namespace, name string,
	mutate ...func(*dorguv1.RemediationAction),
) *dorguv1.RemediationAction {
	r := &dorguv1.RemediationAction{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         namespace,
			Name:              name,
			CreationTimestamp: metav1.NewTime(baseTime),
		},
		Spec: dorguv1.RemediationActionSpec{
			IncidentRef: dorguv1.IncidentReference{Name: "oom-checkout", Namespace: namespace},
			PersonaRef: dorguv1.PersonaReference{
				Kind: "ApplicationPersona", Name: "checkout", Namespace: namespace,
			},
			TrustLevel:  2,
			Confidence:  "0.85",
			Explanation: "The container was OOMKilled three times in ten minutes.",
			Action:      dorguv1.RemediationActionDetail{Type: dorguv1.ActionTypePersonaUpdate},
		},
		Status: dorguv1.RemediationActionStatus{Phase: RemediationPending},
	}
	for _, m := range mutate {
		m(r)
	}
	return r
}

// helmOwned is the brownfield default: something else owns the workload's
// desired state, so Dorgu explains rather than writes.
func helmOwned(r *dorguv1.RemediationAction) {
	r.Spec.WorkloadRef = &dorguv1.WorkloadRef{
		Kind:            "Deployment",
		Name:            "checkout-podinfo",
		Namespace:       r.Namespace,
		Container:       "podinfo",
		ManagedBy:       dorguv1.ManagedByHelm,
		ManagedByDetail: `Helm release "checkout" in namespace apps`,
		ObservedResources: &dorguv1.ObservedResources{
			Limits: &dorguv1.ResourceValues{Memory: "256Mi"},
		},
	}
}

func unmanaged(r *dorguv1.RemediationAction) {
	r.Spec.WorkloadRef = &dorguv1.WorkloadRef{
		Kind:      "Deployment",
		Name:      "reports",
		Namespace: r.Namespace,
		Container: "reports",
		ManagedBy: dorguv1.ManagedByUnmanaged,
		ObservedResources: &dorguv1.ObservedResources{
			Limits: &dorguv1.ResourceValues{Memory: "256Mi"},
		},
	}
}

func buildOne(action *dorguv1.RemediationAction) Remediation {
	payload := BuildRemediations(RemediationsInput{Remediations: []*dorguv1.RemediationAction{action}})
	return payload.Remediations[0]
}

// ---------------------------------------------------------------------------
// The invariant: a value a guardrail refused is never shown as a change
// ---------------------------------------------------------------------------

// The clean-room shape, with the reported numbers.
//
// The plan asked for 4Gi against a live 256Mi, which is 16x and over the 2x
// ceiling, so the blast-radius guardrail clamped it to 512Mi. It also asked to
// set a CPU limit the container does not have, which the absent-field rule
// rejected outright.
//
// Two things have to be true at once, and they pull in opposite directions:
// the refused 4Gi must appear nowhere as something that will happen, and it must
// appear in the verdict, because the verdict is the only place a reader learns it
// was asked for at all.
func TestARefusedValueAppearsInTheVerdictAndInNoDiff(t *testing.T) {
	action := remediation("apps", "fix-oom-checkout", helmOwned, func(r *dorguv1.RemediationAction) {
		r.Spec.PlanSource = dorguv1.PlanSourceAIAnthropic
		r.Spec.Steps = []dorguv1.RemediationStep{{
			Order:          1,
			ID:             "raise-memory",
			Type:           dorguv1.StepTypePersonaUpdate,
			Risk:           "low",
			AutoExecutable: true,
			Description:    "Set spec.resources.limits.memory to 512Mi on the ApplicationPersona.",
			Rationale:      "Raising the memory limit to 4Gi is well within a 2x ceiling.",
			// The patch is the post-guardrail object: 512Mi, and no cpu key.
			Patch: rawJSON(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
			Safety: []dorguv1.StepSafety{
				{
					Rule:      dorguv1.SafetyRuleBlastRadius,
					Verdict:   dorguv1.SafetyVerdictClamped,
					Field:     "spec.resources.limits.memory",
					Baseline:  "256Mi",
					Requested: "4Gi",
					Permitted: "512Mi",
					Ratio:     "16.0x",
					MaxRatio:  "2.0x",
					Message:   "Blast-radius guardrail: the plan asked to set memory to 4Gi.",
				},
				{
					Rule:      dorguv1.SafetyRuleAbsentField,
					Verdict:   dorguv1.SafetyVerdictRejected,
					Field:     "spec.resources.limits.cpu",
					Requested: "500m",
					Message:   `Left out: container "podinfo" does not set cpu today.`,
				},
			},
		}}
	})

	row := buildOne(action)
	require.Len(t, row.Steps, 1)
	step := row.Steps[0]

	// The verdict carries both refused values, verbatim, because it is the only
	// record that they were asked for.
	require.Len(t, step.Safety, 2)
	assert.Equal(t, "4Gi", step.Safety[0].Requested)
	assert.Equal(t, "512Mi", step.Safety[0].Permitted)
	assert.Equal(t, "500m", step.Safety[1].Requested)
	assert.Empty(t, step.Safety[1].Permitted,
		"a rejected field applies nothing, and the empty value is what says so")

	// No diff anywhere may offer either refused value.
	for _, change := range step.PatchChanges {
		assert.NotEqual(t, "4Gi", change.After, "the clamped value must not appear as a change")
	}
	for _, change := range row.WorkloadChanges {
		assert.NotEqual(t, "4Gi", change.After)
		assert.NotEqual(t, "500m", change.After)
	}

	// The rejected CPU key is absent from the workload diff entirely: the
	// container does not set it and the plan will not either, so there is
	// nothing to show.
	paths := changePaths(row.WorkloadChanges)
	assert.NotContains(t, paths, "resources.limits.cpu",
		"a rejected field is not in the patch, so it cannot be in the diff")

	// And the change that WILL happen is shown, against the live value.
	memory := findChange(t, row.WorkloadChanges, "resources.limits.memory")
	assert.Equal(t, "256Mi", memory.Before)
	assert.Equal(t, "512Mi", memory.After)
	assert.True(t, memory.Changed)
	assert.False(t, memory.Added)

	assert.Equal(t, dorguv1.SafetyVerdictRejected, row.StrongestVerdict,
		"rejected outranks clamped: it is the one where nothing happens")
	assert.Equal(t, 2, row.GuardrailCount)
}

// The operator appends every safety message to the description of a step it ruled
// on. A client rendering both would print the same sentence twice, three lines
// apart, which is what the structured field was introduced to stop.
func TestSafetyMessagesAreTakenBackOutOfTheDescription(t *testing.T) {
	const message = "Blast-radius guardrail: the plan asked for 4Gi, applying 512Mi."
	entries := []dorguv1.StepSafety{{
		Rule: dorguv1.SafetyRuleBlastRadius, Verdict: dorguv1.SafetyVerdictClamped,
		Message: message,
	}}

	got := descriptionWithoutSafetyMessages("Set the memory limit to 512Mi. "+message, entries)
	assert.Equal(t, "Set the memory limit to 512Mi.", got)
	assert.NotContains(t, got, "Blast-radius")
}

// A description that is nothing but the message is left alone. Printing it twice
// is worse than printing it once; printing an empty step is worse than both.
func TestADescriptionThatIsOnlyTheMessageSurvives(t *testing.T) {
	const message = "Left out: the container does not set cpu today."
	entries := []dorguv1.StepSafety{{Message: message}}
	assert.Equal(t, message, descriptionWithoutSafetyMessages(message, entries))
}

// A verdict from an operator newer than this build is still Dorgu's verdict, so
// it is reported rather than dropped.
func TestAnUnrankedVerdictIsStillReported(t *testing.T) {
	steps := []Step{{Safety: []dorguv1.StepSafety{{Verdict: "quarantined"}}}}
	assert.Equal(t, "quarantined", strongestVerdict(steps))
}

func TestNoGuardrailMeansNoVerdict(t *testing.T) {
	assert.Empty(t, strongestVerdict([]Step{{}}))
	assert.Equal(t, 0, countGuardrails([]Step{{}}))
}

// ---------------------------------------------------------------------------
// The workload diff, which is the one that matters
// ---------------------------------------------------------------------------

// F-05. A plan that introduces a resource key the container has never had is a
// change of a different kind from one that moves a number, and comparing persona
// to persona made it invisible. The before-state is the observed workload.
func TestAnIntroducedResourceKeyIsCalledOut(t *testing.T) {
	action := remediation("apps", "add-cpu", unmanaged, func(r *dorguv1.RemediationAction) {
		r.Spec.Steps = []dorguv1.RemediationStep{{
			Order: 1, ID: "s1", Type: dorguv1.StepTypePersonaUpdate, AutoExecutable: true,
			Description: "Set resources.limits.cpu.",
			Patch:       rawJSON(`{"spec":{"resources":{"limits":{"cpu":"500m","memory":"512Mi"}}}}`),
		}}
	})

	row := buildOne(action)

	cpu := findChange(t, row.WorkloadChanges, "resources.limits.cpu")
	assert.Equal(t, AbsentResourceValue, cpu.Before)
	assert.Equal(t, "500m", cpu.After)
	assert.True(t, cpu.Added, "this container has no CPU limit today, so the plan introduces one")

	memory := findChange(t, row.WorkloadChanges, "resources.limits.memory")
	assert.False(t, memory.Added, "memory is already set, so raising it adds nothing")
	assert.True(t, memory.Changed)
}

// Without an observation there is no basis for claiming a key is new. "Dorgu
// never looked" and "Dorgu looked and found nothing" are different facts.
func TestAnUnobservedWorkloadDoesNotClaimAKeyIsNew(t *testing.T) {
	action := remediation("apps", "blind", func(r *dorguv1.RemediationAction) {
		// A ref with no observed resources: the operator could not read the
		// container. ManagedBy defaults to unknown, which is treated as owned.
		r.Spec.WorkloadRef = &dorguv1.WorkloadRef{
			Kind: "Deployment", Name: "checkout", Namespace: "apps",
			ManagedBy: dorguv1.ManagedByUnknown,
		}
		r.Spec.Steps = []dorguv1.RemediationStep{{
			Order: 1, ID: "s1", Type: dorguv1.StepTypePersonaUpdate, AutoExecutable: true,
			Description: "Set the memory limit.",
			Patch:       rawJSON(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
		}}
	})

	row := buildOne(action)
	memory := findChange(t, row.WorkloadChanges, "resources.limits.memory")
	assert.Equal(t, UnobservedResourceValue, memory.Before,
		"unknown, not 'not set': Dorgu never read this container")
	assert.False(t, memory.Added, "a key cannot be called new against a workload nobody read")
}

// ---------------------------------------------------------------------------
// The step patch diff
// ---------------------------------------------------------------------------

func TestPatchChangesFlattenToDottedPaths(t *testing.T) {
	changes := diffPatch(
		rawJSON(`{"spec":{"resources":{"limits":{"memory":"512Mi"}},"replicas":3}}`),
		rawJSON(`{"spec":{"resources":{"limits":{"memory":"256Mi"}},"replicas":3}}`),
	)

	memory := findChange(t, changes, "spec.resources.limits.memory")
	assert.Equal(t, "256Mi", memory.Before)
	assert.Equal(t, "512Mi", memory.After)
	assert.True(t, memory.Changed)

	replicas := findChange(t, changes, "spec.replicas")
	assert.Equal(t, "3", replicas.Before, "a number renders as it was written, not as 3.000000")
	assert.Equal(t, "3", replicas.After)
	assert.False(t, replicas.Changed, "a field the patch restates unchanged is shown as unchanged")
}

// Without a pre-patch snapshot every field reads as new, which is the honest
// reading: nothing in the object says what the value was.
func TestPatchWithNoSnapshotReportsEveryFieldAsAdded(t *testing.T) {
	changes := diffPatch(rawJSON(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`), nil)
	require.Len(t, changes, 1)
	assert.True(t, changes[0].Added)
	assert.Equal(t, AbsentResourceValue, changes[0].Before)
}

// A patch this build cannot parse produces no rows rather than a wrong row.
func TestAnUnparseablePatchProducesNoDiff(t *testing.T) {
	assert.Empty(t, diffPatch(rawJSON(`{not json`), nil))
	assert.Empty(t, diffPatch(nil, nil))
}

// A patch that sets nothing gets no rows. A row whose field path is the empty
// string reads as a rendering fault rather than as a change.
func TestAnEmptyPatchProducesNoDiffRows(t *testing.T) {
	assert.Empty(t, diffPatch(rawJSON(`{}`), nil))
	assert.Empty(t, diffPatch(rawJSON(`[]`), nil))
	assert.Empty(t, diffPatch(rawJSON(`"just a string"`), nil))

	// Nested, an empty object is a field being set to {} and is worth showing.
	changes := diffPatch(rawJSON(`{"spec":{"resources":{}}}`), nil)
	require.Len(t, changes, 1)
	assert.Equal(t, "spec.resources", changes[0].Path)
	assert.Equal(t, "{}", changes[0].After)
}

// A patch setting a field to null is removing it, which is a change worth seeing.
func TestANullValueRendersAsTheWord(t *testing.T) {
	changes := diffPatch(rawJSON(`{"spec":{"resources":null}}`), nil)
	require.Len(t, changes, 1)
	assert.Equal(t, "null", changes[0].After)
}

// ---------------------------------------------------------------------------
// Appliability, and the ownership that overrides it
// ---------------------------------------------------------------------------

// The gate that lifted. Clean-room run #4 measured nine AI plans and zero that
// could change a workload, because the planner described the fix as
// workload-apply steps the CRD forbids from being auto-executable and gave the
// one persona-update step no patch.
func TestAPlanWithNoPatchIsAdvisoryNotAppliable(t *testing.T) {
	action := remediation("apps", "advice-only", helmOwned, func(r *dorguv1.RemediationAction) {
		r.Spec.Action.Type = dorguv1.ActionTypeNotification
		r.Spec.Steps = []dorguv1.RemediationStep{{
			Order: 1, ID: "s1", Type: dorguv1.StepTypeWorkloadApply,
			Description: "Raise the memory limit in your Helm values.",
		}}
	})

	row := buildOne(action)
	assert.False(t, row.Appliable)
	assert.Empty(t, row.WorkloadChanges, "there is no change to diff")
	assert.NotEmpty(t, row.OwnerInstructions, "advice still has to be actionable")
}

// An appliable plan against an owned workload will be declined at approve time.
// Saying so on the row is the difference between a guard that reads as
// competence and one that reads as breakage discovered later.
func TestAnAppliablePlanOnAnOwnedWorkloadSaysWhyItWillNotBeApplied(t *testing.T) {
	action := remediation("apps", "fix-oom", helmOwned, func(r *dorguv1.RemediationAction) {
		r.Spec.Steps = []dorguv1.RemediationStep{{
			Order: 1, ID: "s1", Type: dorguv1.StepTypePersonaUpdate, AutoExecutable: true,
			Description: "Raise the memory limit.",
			Patch:       rawJSON(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
		}}
	})

	row := buildOne(action)
	require.True(t, row.Appliable)
	assert.Contains(t, row.AppliableBlockedBy, "field-manager conflict",
		"the reader should finish the sentence knowing what would have gone wrong")
	require.NotNil(t, row.Workload)
	assert.True(t, row.Workload.Owned)
	assert.Contains(t, row.Workload.OwnerName, "Helm release")
	assert.Contains(t, row.Workload.ChangeLocation, "helm upgrade")
}

// An unmanaged workload is the one Dorgu may patch, so there is nothing to block
// and nobody to hand instructions to.
func TestAnUnmanagedWorkloadIsNotBlockedAndNeedsNoOwnerInstructions(t *testing.T) {
	action := remediation("apps", "fix-oom", unmanaged, func(r *dorguv1.RemediationAction) {
		r.Spec.Steps = []dorguv1.RemediationStep{{
			Order: 1, ID: "s1", Type: dorguv1.StepTypePersonaUpdate, AutoExecutable: true,
			Description: "Raise the memory limit.",
			Patch:       rawJSON(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
		}}
	})

	row := buildOne(action)
	assert.True(t, row.Appliable)
	assert.Empty(t, row.AppliableBlockedBy)
	assert.Empty(t, row.OwnerInstructions)
	require.NotNil(t, row.Workload)
	assert.False(t, row.Workload.Owned)
	assert.Empty(t, row.Workload.WhyNotPatched)
}

// A plan with no workloadRef at all is treated as owned: no observation is not
// evidence that patching is safe.
func TestNoWorkloadRefIsTreatedAsOwned(t *testing.T) {
	action := remediation("apps", "no-ref", func(r *dorguv1.RemediationAction) {
		r.Spec.Steps = []dorguv1.RemediationStep{{
			Order: 1, ID: "s1", Type: dorguv1.StepTypePersonaUpdate, AutoExecutable: true,
			Description: "Raise the memory limit.",
			Patch:       rawJSON(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
		}}
	})

	row := buildOne(action)
	assert.Nil(t, row.Workload, "there is no workload to describe")
	assert.NotEmpty(t, row.AppliableBlockedBy, "absent evidence is still treated as owned")
	assert.NotEmpty(t, row.OwnerInstructions)
	assert.Contains(t, row.OwnerInstructions[0].Description, "resources.limits.memory: 512Mi",
		"a refusal that hands over nothing is a dead end")
}

// ---------------------------------------------------------------------------
// Step commands
// ---------------------------------------------------------------------------

func TestStepCommandsAreFilteredByShapeAndByOwnership(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		owned        bool
		wantOffered  string
		wantWithheld bool
	}{
		{"read-only on an owned workload is offered", "kubectl logs deploy/checkout -n apps", true,
			"kubectl logs deploy/checkout -n apps", false},
		{"a write on an owned workload is withheld", "kubectl patch deploy/checkout -n apps --type merge -p x", true,
			"", true},
		{"a write on an unmanaged workload is offered", "kubectl scale deploy/reports --replicas=3", false,
			"kubectl scale deploy/reports --replicas=3", false},
		{"a global flag cannot hide the verb", "kubectl -n apps patch deploy/checkout", true, "", true},
		{"rollout status reads", "kubectl rollout status deploy/checkout -n apps", true,
			"kubectl rollout status deploy/checkout -n apps", false},
		{"rollout restart writes", "kubectl rollout restart deploy/checkout -n apps", true, "", true},
		{"a bare rollout cannot be classified", "kubectl rollout deploy/checkout", true, "", true},
		{"an unrecognised verb is refused on an owned workload", "kubectl frobnicate deploy/checkout", true,
			"", true},
		{"a shell metacharacter is refused outright", "kubectl get pods; rm -rf /", false, "", true},
		{"a non-kubectl command is refused outright", "helm upgrade checkout ./chart", false, "", true},
		{"no command means nothing to say", "", true, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offered, withheld := runnableStepCommand(tt.command, tt.owned)
			assert.Equal(t, tt.wantOffered, offered)
			assert.Equal(t, tt.wantWithheld, withheld != "",
				"a command that was carried and is not offered has to say why")
		})
	}
}

// The step carries the command; the row explains the absence. A reader who saw it
// in `kubectl get -o yaml` would otherwise think this screen lost it.
func TestAWithheldCommandIsExplainedOnTheStep(t *testing.T) {
	action := remediation("apps", "restart", helmOwned, func(r *dorguv1.RemediationAction) {
		r.Spec.Steps = []dorguv1.RemediationStep{{
			Order: 1, ID: "s1", Type: dorguv1.StepTypeRestart,
			Description: "Restart the Deployment.",
			Command:     "kubectl rollout restart deploy/checkout-podinfo -n apps",
		}}
	})

	row := buildOne(action)
	require.Len(t, row.Steps, 1)
	assert.Empty(t, row.Steps[0].Command)
	assert.Contains(t, row.Steps[0].CommandWithheld, "take the fields it sets away from the owner")
}

// ---------------------------------------------------------------------------
// Steps, risk and mode
// ---------------------------------------------------------------------------

// The single cheapest place in the product to guess wrong. A step with no risk
// assessed is not a low-risk step.
func TestAnUnrecordedRiskIsUnknownNotLow(t *testing.T) {
	action := remediation("apps", "no-risk", unmanaged, func(r *dorguv1.RemediationAction) {
		r.Spec.Steps = []dorguv1.RemediationStep{{
			Order: 1, ID: "s1", Type: dorguv1.StepTypeManual, Description: "Check the logs.",
		}}
	})

	row := buildOne(action)
	assert.Equal(t, RiskUnknown, row.Steps[0].Risk)
	assert.Equal(t, StepModeAdvisory, row.Steps[0].Mode)
}

func TestStepsAreOrderedByOrderNotByArrivalOrder(t *testing.T) {
	action := remediation("apps", "ordered", unmanaged, func(r *dorguv1.RemediationAction) {
		r.Spec.Steps = []dorguv1.RemediationStep{
			{Order: 3, ID: "c", Type: dorguv1.StepTypeManual, Description: "Third."},
			{Order: 1, ID: "a", Type: dorguv1.StepTypeManual, Description: "First."},
			{Order: 2, ID: "b", Type: dorguv1.StepTypeManual, Description: "Second."},
		}
	})

	row := buildOne(action)
	require.Len(t, row.Steps, 3)
	assert.Equal(t, []string{"a", "b", "c"},
		[]string{row.Steps[0].ID, row.Steps[1].ID, row.Steps[2].ID})
}

// A legacy object with only the single Action still renders as a plan. The
// operator's own EffectiveSteps does the projection, so this surface cannot
// disagree with the others about what an old object says.
func TestALegacySingleActionObjectRendersAsOneStep(t *testing.T) {
	action := remediation("apps", "legacy", unmanaged, func(r *dorguv1.RemediationAction) {
		r.Spec.Action = dorguv1.RemediationActionDetail{
			Type:  dorguv1.ActionTypePersonaUpdate,
			Patch: rawJSON(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
		}
	})

	row := buildOne(action)
	require.Len(t, row.Steps, 1)
	assert.Equal(t, dorguv1.StepTypePersonaUpdate, row.Steps[0].Type)
	assert.True(t, row.Steps[0].AutoExecutable)
	assert.True(t, row.Appliable)
	assert.NotEmpty(t, row.WorkloadChanges, "a legacy plan still has a diff")
}

// The operator's per-step status is attached to the step it belongs to.
func TestStepStatusIsMatchedByOrder(t *testing.T) {
	action := remediation("apps", "applied", unmanaged, func(r *dorguv1.RemediationAction) {
		r.Spec.Steps = []dorguv1.RemediationStep{
			{Order: 1, ID: "a", Type: dorguv1.StepTypeManual, Description: "One."},
			{Order: 2, ID: "b", Type: dorguv1.StepTypeManual, Description: "Two."},
		}
		r.Status.StepStatuses = []dorguv1.StepStatus{{Order: 2, Phase: "Applied"}}
	})

	row := buildOne(action)
	assert.Nil(t, row.Steps[0].Status)
	require.NotNil(t, row.Steps[1].Status)
	assert.Equal(t, "Applied", row.Steps[1].Status.Phase)
}

// ---------------------------------------------------------------------------
// The list itself
// ---------------------------------------------------------------------------

// This is a decision queue, not a feed. A Pending plan from an hour ago is more
// urgent than a Completed one from a minute ago, which is the opposite of how the
// incident list is ordered and deliberately so.
func TestPendingPlansSortAboveEverythingElse(t *testing.T) {
	later := metav1.NewTime(baseTime.Add(3600000000000))

	payload := BuildRemediations(RemediationsInput{
		Remediations: []*dorguv1.RemediationAction{
			remediation("apps", "completed-recently", func(r *dorguv1.RemediationAction) {
				r.Status.Phase = RemediationCompleted
				r.CreationTimestamp = later
			}),
			remediation("apps", "pending-older"),
		},
	})

	require.Len(t, payload.Remediations, 2)
	assert.Equal(t, "pending-older", payload.Remediations[0].Name,
		"what needs deciding comes first")
	assert.Equal(t, 1, payload.Summary.Pending)
	assert.Equal(t, 1, payload.Summary.Completed)
}

// An object the operator has not reconciled is a proposal awaiting a decision.
// Rendering an empty phase as an empty cell would hide a plan from its queue.
func TestAnEmptyPhaseReadsAsPending(t *testing.T) {
	action := remediation("apps", "fresh", func(r *dorguv1.RemediationAction) {
		r.Status.Phase = ""
	})
	assert.Equal(t, RemediationPending, buildOne(action).Phase)
}

func TestSummaryCountsWhatTheHeaderShows(t *testing.T) {
	payload := BuildRemediations(RemediationsInput{
		Remediations: []*dorguv1.RemediationAction{
			remediation("apps", "ai-guarded-owned", helmOwned, func(r *dorguv1.RemediationAction) {
				r.Spec.PlanSource = dorguv1.PlanSourceAIAnthropic
				r.Spec.Steps = []dorguv1.RemediationStep{{
					Order: 1, ID: "s1", Type: dorguv1.StepTypePersonaUpdate, AutoExecutable: true,
					Description: "Raise memory.",
					Patch:       rawJSON(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
					Safety: []dorguv1.StepSafety{{
						Rule: dorguv1.SafetyRuleBlastRadius, Verdict: dorguv1.SafetyVerdictClamped,
						Field: "spec.resources.limits.memory", Message: "clamped",
					}},
				}}
			}),
			remediation("apps", "rule-advisory", unmanaged, func(r *dorguv1.RemediationAction) {
				r.Spec.PlanSource = dorguv1.PlanSourceRuleBased
				r.Spec.Action.Type = dorguv1.ActionTypeNotification
				r.Spec.Steps = []dorguv1.RemediationStep{{
					Order: 1, ID: "s1", Type: dorguv1.StepTypeManual, Description: "Check the logs.",
				}}
			}),
		},
	})

	got := payload.Summary
	assert.Equal(t, 2, got.Total)
	assert.Equal(t, 1, got.Appliable)
	assert.Equal(t, 1, got.Advisory)
	assert.Equal(t, 1, got.Guarded)
	assert.Equal(t, 1, got.Owned)
	assert.Equal(t, 1, got.AIPlanned)
}

func TestNamespaceScopeFiltersTheList(t *testing.T) {
	payload := BuildRemediations(RemediationsInput{
		Remediations: []*dorguv1.RemediationAction{
			remediation("apps", "here"),
			remediation("other", "elsewhere"),
		},
		Namespace: "apps",
	})
	require.Len(t, payload.Remediations, 1)
	assert.Equal(t, "here", payload.Remediations[0].Name)
}

func TestTruncationIsReportedRatherThanSwallowed(t *testing.T) {
	var actions []*dorguv1.RemediationAction
	for i := range 5 {
		actions = append(actions, remediation("apps", string(rune('a'+i))))
	}

	payload := BuildRemediations(RemediationsInput{Remediations: actions, Limit: 2})
	require.NotNil(t, payload.Truncation)
	assert.Equal(t, 2, payload.Truncation.Shown)
	assert.Equal(t, 5, payload.Truncation.Total)
	assert.Len(t, payload.Remediations, 2)
}

func TestEmptyListEncodesAsAListNotNull(t *testing.T) {
	payload := BuildRemediations(RemediationsInput{})
	assert.NotNil(t, payload.Remediations)
	assert.Empty(t, payload.Remediations)
}

// ---------------------------------------------------------------------------
// Read-only
// ---------------------------------------------------------------------------

// The affordance this view offers instead of an approve button. Pointing at the
// tool that can act is honest in a way a disabled control is not.
func TestEveryRowCarriesTheCLICommandThatCanActOnIt(t *testing.T) {
	row := buildOne(remediation("apps", "fix-oom-checkout"))
	assert.Equal(t, "dorgu remediation diff fix-oom-checkout -n apps", row.DiffCommand)
}

// ---------------------------------------------------------------------------
// Referenced objects
// ---------------------------------------------------------------------------

// A link is only worth offering when there is something to link to.
func TestExistenceOfReferencedObjectsIsReported(t *testing.T) {
	action := remediation("apps", "fix-oom")

	without := buildOne(action)
	assert.False(t, without.Persona.Exists)
	assert.False(t, without.Incident.Exists)

	payload := BuildRemediations(RemediationsInput{
		Remediations: []*dorguv1.RemediationAction{action},
		Personas: []*dorguv1.ApplicationPersona{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "checkout"},
		}},
		Incidents: []*dorguv1.IncidentMemory{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "oom-checkout"},
		}},
	})
	assert.True(t, payload.Remediations[0].Persona.Exists)
	assert.True(t, payload.Remediations[0].Incident.Exists)
}

// Older operators wrote the root cause into planSummary and the same sentence
// into explanation, so one paragraph printed twice under two headings.
func TestARedundantExplanationIsDropped(t *testing.T) {
	const summary = "The container was OOMKilled three times in ten minutes."
	assert.Empty(t, nonRedundantExplanation("Root cause: "+summary, summary))
	assert.Equal(t, "Something else entirely.",
		nonRedundantExplanation("Something else entirely.", summary))
	assert.Equal(t, "Only an explanation.", nonRedundantExplanation("Only an explanation.", ""))
}

// A missing approval block is an object that requires approval, not one that
// does not. The CRD defaults Required to true.
func TestAMissingApprovalBlockStillRequiresApproval(t *testing.T) {
	assert.True(t, buildOne(remediation("apps", "fix-oom")).ApprovalRequired)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func changePaths(changes []ResourceChange) []string {
	out := make([]string, 0, len(changes))
	for _, change := range changes {
		out = append(out, change.Path)
	}
	return out
}

func findChange(t *testing.T, changes []ResourceChange, path string) ResourceChange {
	t.Helper()
	for _, change := range changes {
		if change.Path == path {
			return change
		}
	}
	t.Fatalf("no change for %q in %v", path, changePaths(changes))
	return ResourceChange{}
}
