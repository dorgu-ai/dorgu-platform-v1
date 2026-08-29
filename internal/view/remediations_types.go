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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// The Remediations view. The diff is the hero, and the guardrail verdict is not
// part of it.
//
// # What this view has to keep separate
//
// A remediation plan mixes two kinds of statement and they must not be allowed
// to blur:
//
//   - what a model reasoned: the root cause, the plan summary, a step's
//     rationale. A hypothesis with a confidence attached.
//   - what Dorgu measured: the guardrail verdicts on spec.steps[].safety. Every
//     string in that field is Dorgu's own arithmetic against the workload it
//     observed, and no model is asked to characterise it.
//
// The field exists because the verdict used to arrive as a
// "[safety:blast-radius] ..." prefix spliced onto the model's rationale. In
// clean-room run #4 that put Dorgu's measurement one line below the model's
// claim that the same 16x change was "well within a 2x ceiling", with nothing to
// tell the reader which of the two had been computed. So the payload keeps them
// in different fields and the UI renders them in visually different blocks.
//
// # Read-only
//
// There is no approve action here and nothing in this package can produce one.
// Every field is a read, and the affordance offered instead is the CLI command
// that shows the same plan in a terminal.

// Remediation phases the operator writes.
//
// Acknowledged is the one worth knowing about: it is terminal for an approved
// plan with nothing to apply, so the decision is recorded and the operator
// changed nothing. It is not a failure and rendering it as a success would be
// the cheerful lie this product exists to avoid.
const (
	RemediationPending      = "Pending"
	RemediationApproved     = "Approved"
	RemediationApplying     = "Applying"
	RemediationVerifying    = "Verifying"
	RemediationCompleted    = "Completed"
	RemediationAcknowledged = "Acknowledged"
	RemediationRolledBack   = "RolledBack"
	RemediationFailed       = "Failed"
	RemediationRejected     = "Rejected"
	RemediationExpired      = "Expired"
)

// Step modes. Only a persona-update step may be auto-executable; everything else
// is recorded for a human to carry out.
const (
	// StepModeAuto means the operator may apply this step itself.
	StepModeAuto = "auto"
	// StepModeAdvisory means the step is recorded for a human, the CLI or a
	// platform to apply. The operator never writes a workload.
	StepModeAdvisory = "advisory"
)

// RiskUnknown is what an empty risk renders as. A step with no risk recorded is
// not a low-risk step, and defaulting it to low would be the most expensive
// possible place to guess.
const RiskUnknown = "unknown"

// DefaultRemediationLimit caps the list, matching the incident feed's reasoning:
// a cluster in a bad hour can hold thousands, and pushing all of them over SSE
// on every change would make the live update the slowest part of the product.
const DefaultRemediationLimit = 200

// AbsentResourceValue is how a resource key the workload does not set renders.
//
// Never blank and never zero. "This container has no CPU limit" is the fact that
// makes adding one a change worth seeing, and it is the fact that made a
// silently introduced limit invisible when the diff compared persona to persona.
const AbsentResourceValue = "not set"

// UnobservedResourceValue is how a key reads when Dorgu never saw the workload
// at all, which is a different thing from seeing it and finding the key absent.
const UnobservedResourceValue = "unknown"

// RemediationsPayload is the whole Remediations view in one object.
type RemediationsPayload struct {
	Remediations []Remediation       `json:"remediations"`
	Summary      RemediationsSummary `json:"summary"`
	Readiness    Readiness           `json:"readiness"`
	Truncation   *TruncationNotice   `json:"truncation,omitempty"`
}

// RemediationsSummary is the count line above the list.
type RemediationsSummary struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	// Appliable counts plans carrying a change the product can actually apply.
	// Its complement is the number that matters: a plan nothing can apply is
	// advice, and clean-room run #4 measured nine of those and zero of these.
	Appliable int `json:"appliable"`
	Advisory  int `json:"advisory"`
	// Guarded counts plans where at least one guardrail ruled on a field.
	Guarded int `json:"guarded"`
	// Owned counts plans against a workload Dorgu will not patch, because
	// something else owns its desired state.
	Owned int `json:"owned"`
	// AIPlanned counts plans a model authored. Shown separately because a plan's
	// source changes how its prose should be read.
	AIPlanned int `json:"aiPlanned"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// Remediation is one row of the view, with its whole plan attached.
//
// The steps come down with the list rather than behind a second request: a
// remediation is a handful of steps, the cache already holds all of it, and a
// detail fetch would be a round trip to produce data the payload had already
// paid to project.
type Remediation struct {
	// ID is "<namespace>/<name>", stable across every other field changing.
	ID        string `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`

	Phase string `json:"phase"`
	// PlanSource is "rule-based" or "ai-anthropic", empty on an older object.
	PlanSource string `json:"planSource,omitempty"`
	// AIPlanned is PlanSource being the AI one, resolved here so the UI does not
	// have to match on a provider string that may gain values.
	AIPlanned bool `json:"aiPlanned"`

	// Confidence is the CRD's decimal string, passed through unparsed. The
	// server does not round it: a confidence is a claim the plan made and
	// reformatting it here would put words in its mouth.
	Confidence string `json:"confidence"`
	TrustLevel int32  `json:"trustLevel"`

	// PlanSummary is the plan's own explanation of the root cause. Model prose
	// when AIPlanned.
	PlanSummary string `json:"planSummary,omitempty"`
	// Explanation is the object's explanation field, omitted when it says the
	// same thing as PlanSummary. Older operators wrote the root cause into both,
	// and printing one paragraph twice under two headings is how a screen loses
	// the reader's attention for the part that matters.
	Explanation string `json:"explanation,omitempty"`

	Incident RemediationIncident `json:"incident"`
	Persona  RemediationPersona  `json:"persona"`
	// Workload is the live Deployment this plan concerns and who owns it, absent
	// when the operator could not observe one. Absent is treated as owned.
	Workload *RemediationWorkload `json:"workload,omitempty"`

	// Steps is the ordered plan, the plan of record. A legacy single-Action
	// object is projected into one step so this is never empty for a plan that
	// has one.
	Steps []Step `json:"steps"`

	// WorkloadChanges is what the plan does to the running container, grounded
	// in the observed workload rather than in the persona.
	//
	// This is the hero. It is a separate diff from the per-step patch diffs
	// because the persona is not the thing that is OOMing: a review that
	// compared persona to persona showed nothing at all for a remediation that
	// introduced a resource key the workload had never had.
	WorkloadChanges []ResourceChange `json:"workloadChanges,omitempty"`

	// Appliable reports that this plan carries a change the product can apply.
	// It is the operator's own HasAutoApplicableChange, not a second opinion.
	Appliable bool `json:"appliable"`
	// AppliableBlockedBy names why an appliable plan will still not be applied
	// by Dorgu, which today is only ever workload ownership. Empty when nothing
	// blocks it or when there was nothing to apply in the first place.
	AppliableBlockedBy string `json:"appliableBlockedBy,omitempty"`

	// StrongestVerdict is the most consequential guardrail verdict anywhere in
	// the plan: rejected over clamped over derived. It is the list column, a
	// pointer to a plan worth opening rather than the record. Empty when no
	// guardrail ruled.
	StrongestVerdict string `json:"strongestVerdict,omitempty"`
	// GuardrailCount is how many fields a guardrail ruled on across the plan.
	GuardrailCount int `json:"guardrailCount"`

	// OwnerInstructions is the owner-shaped plan: what to change, and where,
	// when Dorgu will not write the workload itself.
	//
	// Numbered from 1 because it is a list of things for the reader to do, not
	// the plan's own ordering. A list that starts at [2] because step 1 was the
	// operator's own persona write reads as though something is missing.
	OwnerInstructions []OwnerInstruction `json:"ownerInstructions,omitempty"`

	// DiffCommand is the CLI command that shows this same plan in a terminal.
	//
	// It is what this view offers instead of an approve button. The dashboard is
	// read-only in this release and the honest affordance is the command that
	// can act, not a control that cannot.
	DiffCommand string `json:"diffCommand"`

	// Rollback is the plan's rollback configuration, absent when it carries
	// none.
	Rollback *RemediationRollback `json:"rollback,omitempty"`

	// ApprovalRequired reports whether human approval is needed. It is always
	// true in practice; it is reported rather than assumed because the field
	// exists and a screen that hard-codes a policy cannot show it changing.
	ApprovalRequired bool         `json:"approvalRequired"`
	ApprovalDeadline *metav1.Time `json:"approvalDeadline,omitempty"`

	ApprovedBy         string       `json:"approvedBy,omitempty"`
	ApprovedAt         *metav1.Time `json:"approvedAt,omitempty"`
	AppliedAt          *metav1.Time `json:"appliedAt,omitempty"`
	VerificationResult string       `json:"verificationResult,omitempty"`

	// Conditions are passed through so a reader can see what the operator and
	// the CLI have each recorded, including the CLI's WorkloadPatched marker.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	CreatedAt metav1.Time `json:"createdAt"`
}

// RemediationIncident is the incident that triggered a plan.
type RemediationIncident struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	// Exists reports whether that IncidentMemory is in the cache, so a link to
	// it is only offered when there is something to link to.
	Exists bool `json:"exists"`
}

// RemediationPersona is the persona a plan is filed against.
type RemediationPersona struct {
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Exists    bool   `json:"exists"`
}

// RemediationWorkload is the live workload a plan concerns, with what its
// ownership means for what Dorgu will do.
type RemediationWorkload struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Container string `json:"container,omitempty"`

	// ManagedBy is one of the dorguv1.ManagedBy* values. Only "unmanaged"
	// permits patching the Deployment.
	ManagedBy       string `json:"managedBy"`
	ManagedByDetail string `json:"managedByDetail,omitempty"`
	// Owned is ManagedBy being anything other than "unmanaged". A workload the
	// operator could not observe is owned, because no observation is not
	// evidence that patching is safe.
	Owned bool `json:"owned"`
	// OwnerName names the owner in prose, for a sentence that says who rather
	// than that somebody does.
	OwnerName string `json:"ownerName,omitempty"`
	// WhyNotPatched is the one plain line explaining the refusal, written so the
	// reader finishes it understanding what would have gone wrong.
	WhyNotPatched string `json:"whyNotPatched,omitempty"`
	// ChangeLocation names where this workload's desired state actually lives.
	ChangeLocation string `json:"changeLocation,omitempty"`

	// Observed reports that the operator resolved a live workload. False means
	// the fields above name nothing and the diff cannot show a before-state.
	Observed          bool                       `json:"observed"`
	ObservedImage     string                     `json:"observedImage,omitempty"`
	ObservedResources *dorguv1.ObservedResources `json:"observedResources,omitempty"`
	ObservedAt        *metav1.Time               `json:"observedAt,omitempty"`
}

// Step is one ordered action in a plan.
type Step struct {
	Order int32  `json:"order"`
	ID    string `json:"id"`
	Type  string `json:"type"`
	// Risk is low, medium, high, or RiskUnknown when the plan recorded none.
	Risk string `json:"risk"`
	// Mode is StepModeAuto or StepModeAdvisory.
	Mode           string `json:"mode"`
	AutoExecutable bool   `json:"autoExecutable"`

	// Description is the step's own sentence, with the guardrail messages taken
	// back out of it.
	//
	// The operator writes a guarded step's description as its sentence followed
	// by each safety message verbatim, so a client rendering both would print
	// the same forty words twice. Saying it once, under its own heading, with
	// the numbers beside it, is the whole point of the safety field.
	Description string `json:"description"`
	// Rationale is the plan's reasoning, kept in its own field so the UI can
	// render it as prose that may be a model's rather than as fact.
	Rationale string `json:"rationale,omitempty"`

	// Command is a ready-to-run kubectl command, or empty.
	//
	// It is filtered twice: once for shape, because it may have been authored by
	// a model and this payload feeds a copy button, and once for ownership,
	// because a command that writes to a workload Helm or ArgoCD owns is the
	// failure the whole ownership model exists to prevent.
	Command string `json:"command,omitempty"`
	// CommandWithheld says why a command the object carried is not offered.
	// Stating it beats silently dropping it: a reader who saw the command in
	// `kubectl get -o yaml` would otherwise think this screen lost it.
	CommandWithheld string `json:"commandWithheld,omitempty"`

	// Safety is every guardrail verdict on this step, as the operator recorded
	// it.
	//
	// The CRD type is reused rather than restated so the field cannot drift, and
	// so nothing in this package is tempted to reword a verdict. Every string in
	// it is Dorgu's own arithmetic.
	Safety []dorguv1.StepSafety `json:"safety,omitempty"`

	// PatchChanges is what this step's patch does to the ApplicationPersona,
	// field by field.
	//
	// Built from the patch, which is the post-guardrail object: a field a
	// guardrail refused is gone from it, so it appears here for no step. That is
	// structural rather than a rule this code has to remember.
	PatchChanges []ResourceChange `json:"patchChanges,omitempty"`

	// Status is the operator's per-step execution outcome, absent until it
	// writes one.
	Status *StepStatus `json:"status,omitempty"`
}

// StepStatus is the operator's record of what happened to one step.
type StepStatus struct {
	Phase              string       `json:"phase,omitempty"`
	AppliedAt          *metav1.Time `json:"appliedAt,omitempty"`
	VerificationResult string       `json:"verificationResult,omitempty"`
}

// ResourceChange is one field's before and after.
//
// It is used for both diffs this view renders, the persona patch and the live
// container, because they are the same shape and a reader should not have to
// learn two.
type ResourceChange struct {
	// Path is the dotted field path, e.g. "spec.resources.limits.memory".
	Path string `json:"path"`
	// Before is the value today, AbsentResourceValue when the field is not set,
	// UnobservedResourceValue when Dorgu never read it.
	Before string `json:"before"`
	// After is the value the plan will apply.
	After string `json:"after"`
	// Added reports that this field does not exist today, so applying the plan
	// introduces it. It is called out because a remediation that quietly adds a
	// resource key the workload never had is a change of a different kind from
	// one that moves a number.
	Added bool `json:"added"`
	// Changed reports that After differs from Before.
	Changed bool `json:"changed"`
}

// OwnerInstruction is one thing for the reader to do where Dorgu will not write.
type OwnerInstruction struct {
	// Order is 1-based within this list.
	Order int `json:"order"`
	// Description is the instruction, as the operator shaped it. It is passed
	// through rather than reworded: the operator rewrites these steps at
	// proposal time for the specific owner, and a second rewording here would
	// be a third voice in the same paragraph.
	Description string `json:"description"`
	// Command is a read-only kubectl command that helps carry it out, when one
	// survives the filters. Reading is the whole of what Dorgu can hand over on
	// a workload it will not patch, so `kubectl logs` matters most here.
	Command string `json:"command,omitempty"`
}

// RemediationRollback is a plan's rollback configuration.
type RemediationRollback struct {
	Enabled          bool   `json:"enabled"`
	HealthCheckAfter string `json:"healthCheckAfter,omitempty"`
	MaxRetries       int32  `json:"maxRetries"`
}
