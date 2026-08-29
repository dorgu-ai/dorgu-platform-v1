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
	"sort"
	"strings"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// RemediationsInput is everything the Remediations view is built from.
type RemediationsInput struct {
	Remediations []*dorguv1.RemediationAction

	// Personas and Incidents are read only to answer whether the objects a plan
	// references actually exist, which is what decides whether the UI offers a
	// link to them.
	Personas  []*dorguv1.ApplicationPersona
	Incidents []*dorguv1.IncidentMemory

	// Namespace scopes the list. Empty means every namespace.
	Namespace string
	// Limit caps the rows. Zero means DefaultRemediationLimit; negative means no
	// cap.
	Limit int

	Readiness Readiness
}

// BuildRemediations projects RemediationActions into the Remediations view.
//
// Ordering puts the plans awaiting a decision first, then everything else
// newest-first. That is not the incident feed's rule, and the difference is the
// point: an incident list is a record of what happened, and this is a queue of
// things a person has to decide about. A Pending plan from an hour ago is more
// urgent than a Completed one from a minute ago.
func BuildRemediations(in RemediationsInput) RemediationsPayload {
	personaExists := personaNameSet(in.Personas)
	incidentExists := incidentNameSet(in.Incidents)

	rows := make([]Remediation, 0, len(in.Remediations))
	for _, action := range in.Remediations {
		if action == nil {
			continue
		}
		if in.Namespace != "" && action.Namespace != in.Namespace {
			continue
		}
		rows = append(rows, projectRemediation(action, personaExists, incidentExists))
	}

	sortRemediations(rows)
	summary := summariseRemediations(rows)

	limit := in.Limit
	if limit == 0 {
		limit = DefaultRemediationLimit
	}
	var truncation *TruncationNotice
	if limit > 0 && len(rows) > limit {
		truncation = &TruncationNotice{Shown: limit, Total: len(rows), Limit: limit}
		rows = rows[:limit]
	}

	return RemediationsPayload{
		Remediations: rows,
		Summary:      summary,
		Readiness:    in.Readiness,
		Truncation:   truncation,
	}
}

func projectRemediation(
	action *dorguv1.RemediationAction,
	personaExists, incidentExists map[string]bool,
) Remediation {
	spec := action.Spec
	workload := spec.WorkloadRef
	owned := workload.IsOwned()

	steps, intent := projectSteps(action, owned)

	row := Remediation{
		ID:         action.Namespace + "/" + action.Name,
		Name:       action.Name,
		Namespace:  action.Namespace,
		Phase:      phaseOrPending(action.Status.Phase),
		PlanSource: spec.PlanSource,
		AIPlanned:  spec.PlanSource == dorguv1.PlanSourceAIAnthropic,
		Confidence: spec.Confidence,
		TrustLevel: spec.TrustLevel,

		PlanSummary: strings.TrimSpace(spec.PlanSummary),
		Explanation: nonRedundantExplanation(spec.Explanation, spec.PlanSummary),

		Incident: RemediationIncident{
			Name:      spec.IncidentRef.Name,
			Namespace: spec.IncidentRef.Namespace,
			Exists:    incidentExists[spec.IncidentRef.Namespace+"/"+spec.IncidentRef.Name],
		},
		Persona: RemediationPersona{
			Kind:      spec.PersonaRef.Kind,
			Name:      spec.PersonaRef.Name,
			Namespace: spec.PersonaRef.Namespace,
			Exists:    personaExists[spec.PersonaRef.Namespace+"/"+spec.PersonaRef.Name],
		},

		Workload:        projectRemediationWorkload(workload),
		Steps:           steps,
		WorkloadChanges: diffWorkloadResources(workload, intent),

		Appliable:        action.HasAutoApplicableChange(),
		StrongestVerdict: strongestVerdict(steps),
		GuardrailCount:   countGuardrails(steps),

		DiffCommand: diffCommandFor(action),

		ApprovedBy:         action.Status.ApprovedBy,
		ApprovedAt:         action.Status.ApprovedAt,
		AppliedAt:          action.Status.AppliedAt,
		VerificationResult: action.Status.VerificationResult,
		Conditions:         action.Status.Conditions,
		CreatedAt:          action.CreationTimestamp,
	}

	// An appliable plan against a workload somebody else owns will be declined
	// at approve time. Saying so on the row is the difference between a guard
	// that reads as competence and one that reads as breakage discovered later.
	if row.Appliable && owned {
		row.AppliableBlockedBy = whyNotPatched(workload)
	}
	// Owner instructions are for the reader who has to make the change
	// somewhere Dorgu will never see. There is nothing to hand over when Dorgu
	// can do it itself.
	if owned {
		row.OwnerInstructions = ownerInstructions(steps, workload, intent)
	}

	if approval := spec.Approval; approval != nil {
		row.ApprovalRequired = approval.Required
		row.ApprovalDeadline = approval.Deadline
	} else {
		// The CRD defaults Required to true, and an object written without the
		// block is one that requires approval rather than one that does not.
		row.ApprovalRequired = true
	}

	if rollback := spec.Rollback; rollback != nil {
		row.Rollback = &RemediationRollback{
			Enabled:    rollback.Enabled,
			MaxRetries: rollback.MaxRetries,
		}
		if rollback.HealthCheckAfter != nil {
			row.Rollback.HealthCheckAfter = rollback.HealthCheckAfter.Duration.String()
		}
	}

	return row
}

// phaseOrPending defaults an empty phase to Pending.
//
// A RemediationAction the operator has not reconciled yet is a proposal awaiting
// a decision, which is what Pending means. Rendering it as an empty cell would
// hide a plan from the queue it belongs in.
func phaseOrPending(phase string) string {
	if phase == "" {
		return RemediationPending
	}
	return phase
}

// projectSteps renders the ordered plan and returns the container resource
// change its patches add up to.
//
// EffectiveSteps is the operator's own projection, so a legacy single-Action
// object becomes one step here exactly as it does everywhere else. Reimplementing
// that fallback is how two surfaces end up disagreeing about what an old object
// says.
func projectSteps(action *dorguv1.RemediationAction, owned bool) ([]Step, resourceIntent) {
	effective := action.EffectiveSteps()
	sort.SliceStable(effective, func(a, b int) bool { return effective[a].Order < effective[b].Order })

	statuses := stepStatusesByOrder(action.Status.StepStatuses)

	steps := make([]Step, 0, len(effective))
	intent := resourceIntent{}
	for i := range effective {
		source := effective[i]

		step := Step{
			Order:          source.Order,
			ID:             source.ID,
			Type:           source.Type,
			Risk:           riskOrUnknown(source.Risk),
			Mode:           stepMode(source.AutoExecutable),
			AutoExecutable: source.AutoExecutable,
			Description:    descriptionWithoutSafetyMessages(source.Description, source.Safety),
			Rationale:      strings.TrimSpace(source.Rationale),
			Safety:         source.Safety,
			PatchChanges:   diffPatch(source.Patch, source.PrePatchState),
		}
		step.Command, step.CommandWithheld = runnableStepCommand(source.Command, owned)
		if status, found := statuses[source.Order]; found {
			step.Status = status
		}

		if source.Type == dorguv1.StepTypePersonaUpdate {
			intent = mergeResourceIntent(intent, extractResourceIntent(source.Patch))
		}
		steps = append(steps, step)
	}
	return steps, intent
}

func stepStatusesByOrder(statuses []dorguv1.StepStatus) map[int32]*StepStatus {
	out := make(map[int32]*StepStatus, len(statuses))
	for i := range statuses {
		status := statuses[i]
		out[status.Order] = &StepStatus{
			Phase:              status.Phase,
			AppliedAt:          status.AppliedAt,
			VerificationResult: status.VerificationResult,
		}
	}
	return out
}

// riskOrUnknown reports an unrecorded risk as unknown rather than as low.
//
// This is the single cheapest place in the product to guess wrong. A step with
// no risk assessed is not a safe step, and a low-risk badge over an unassessed
// change to a running workload is the kind of confident wrong answer that costs
// the trust the rest of this screen is built to earn.
func riskOrUnknown(risk string) string {
	if strings.TrimSpace(risk) == "" {
		return RiskUnknown
	}
	return risk
}

func stepMode(autoExecutable bool) string {
	if autoExecutable {
		return StepModeAuto
	}
	return StepModeAdvisory
}

// projectRemediationWorkload renders the live workload and what its ownership
// means, or nil when the plan records no workload at all.
func projectRemediationWorkload(workload *dorguv1.WorkloadRef) *RemediationWorkload {
	if workload == nil {
		return nil
	}

	out := &RemediationWorkload{
		Kind:            workload.Kind,
		Name:            workload.Name,
		Namespace:       workload.Namespace,
		Container:       workload.Container,
		ManagedBy:       workload.ManagedBy,
		ManagedByDetail: workload.ManagedByDetail,
		Owned:           workload.IsOwned(),
		Observed:        workloadObserved(workload),
		ObservedImage:   workload.ObservedImage,

		ObservedResources: workload.ObservedResources,
		ObservedAt:        workload.ObservedAt,
	}
	if out.Owned {
		out.OwnerName = ownerName(workload)
		out.WhyNotPatched = whyNotPatched(workload)
		out.ChangeLocation = ownerChangeLocation(workload)
	}
	return out
}

// ownerInstructions builds the list of things for the reader to do, from the
// plan's own advisory steps.
//
// The operator rewrites those steps at proposal time for the specific owner, so
// they are passed through as written. When the plan carries none, the concrete
// resource change becomes one instruction, because a refusal that hands over
// nothing is a dead end.
func ownerInstructions(
	steps []Step,
	workload *dorguv1.WorkloadRef,
	intent resourceIntent,
) []OwnerInstruction {
	var out []OwnerInstruction
	for _, step := range steps {
		// A persona-update step is Dorgu's own write and is always safe, so it
		// is never something to hand to the workload's owner.
		if step.Type == dorguv1.StepTypePersonaUpdate {
			continue
		}
		if strings.TrimSpace(step.Description) == "" {
			continue
		}
		out = append(out, OwnerInstruction{
			Order:       len(out) + 1,
			Description: step.Description,
			Command:     step.Command,
		})
	}
	if len(out) > 0 {
		return out
	}

	if intent.isEmpty() {
		return nil
	}
	return []OwnerInstruction{{
		Order:       1,
		Description: ownerFallbackInstruction(workload, intent),
	}}
}

// diffCommandFor is the CLI command that shows this plan in a terminal.
//
// It is the affordance this view offers in place of an approve button. The
// dashboard is read-only in this release, and pointing at the tool that can act
// is honest in a way a disabled control is not.
func diffCommandFor(action *dorguv1.RemediationAction) string {
	command := "dorgu remediation diff " + action.Name
	if action.Namespace != "" {
		command += " -n " + action.Namespace
	}
	return command
}

// sortRemediations puts what needs deciding first, then newest-first.
func sortRemediations(rows []Remediation) {
	sort.SliceStable(rows, func(a, b int) bool {
		x, y := rows[a], rows[b]
		if awaitingDecision(x) != awaitingDecision(y) {
			return awaitingDecision(x)
		}
		if !x.CreatedAt.Equal(&y.CreatedAt) {
			return x.CreatedAt.After(y.CreatedAt.Time)
		}
		if x.Namespace != y.Namespace {
			return x.Namespace < y.Namespace
		}
		return x.Name < y.Name
	})
}

// awaitingDecision reports whether a plan is waiting on a person.
func awaitingDecision(row Remediation) bool {
	return row.Phase == RemediationPending
}

func summariseRemediations(rows []Remediation) RemediationsSummary {
	summary := RemediationsSummary{Total: len(rows)}
	for _, row := range rows {
		if row.Phase == RemediationPending {
			summary.Pending++
		}
		if row.Appliable {
			summary.Appliable++
		} else {
			summary.Advisory++
		}
		if row.GuardrailCount > 0 {
			summary.Guarded++
		}
		if row.Workload != nil && row.Workload.Owned {
			summary.Owned++
		}
		if row.AIPlanned {
			summary.AIPlanned++
		}
		switch row.Phase {
		case RemediationCompleted:
			summary.Completed++
		case RemediationFailed:
			summary.Failed++
		}
	}
	return summary
}

func incidentNameSet(incidents []*dorguv1.IncidentMemory) map[string]bool {
	out := make(map[string]bool, len(incidents))
	for _, incident := range incidents {
		if incident != nil {
			out[incident.Namespace+"/"+incident.Name] = true
		}
	}
	return out
}
