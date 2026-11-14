package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// testNow is the fixed clock every report test computes against.
var testNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// fixtureWindow is the 7 day window the fixtures are built for.
const fixtureWindow = 7 * 24 * time.Hour

// ago renders testNow minus d as the RFC3339 timestamp the API server writes.
func ago(d time.Duration) string { return testNow.Add(-d).Format(time.RFC3339) }

// cond renders a status condition.
func cond(condType, status, reason string, at time.Duration) string {
	return fmt.Sprintf(`{"type":%q,"status":%q,"reason":%q,"lastTransitionTime":%q}`, condType, status, reason, ago(at))
}

// raOpts describes one RemediationAction fixture.
type raOpts struct {
	name, phase  string
	created      time.Duration // age at testNow
	approvedAt   time.Duration // age; 0 means never approved
	appliedAt    time.Duration
	incident     string
	conds        []string
	gate         string // JSON of status.gate, empty for none
	safety       string // JSON array of spec.steps[0].safety
	planSource   string
	verification string
}

func raJSON(o raOpts) string {
	status := fmt.Sprintf(`"phase":%q`, o.phase)
	if o.approvedAt > 0 {
		status += fmt.Sprintf(`,"approvedAt":%q`, ago(o.approvedAt))
	}
	if o.appliedAt > 0 {
		status += fmt.Sprintf(`,"appliedAt":%q`, ago(o.appliedAt))
	}
	if o.verification != "" {
		status += fmt.Sprintf(`,"verificationResult":%q`, o.verification)
	}
	if o.gate != "" {
		status += `,"gate":` + o.gate
	}
	status += `,"conditions":[` + strings.Join(o.conds, ",") + `]`
	safety := o.safety
	if safety == "" {
		safety = `[]`
	}
	src := o.planSource
	if src == "" {
		src = "rule-based"
	}
	return fmt.Sprintf(`{"metadata":{"name":%q,"namespace":"shop","creationTimestamp":%q},`+
		`"spec":{"planSource":%q,"incidentRef":{"name":%q,"namespace":"shop"},"steps":[{"safety":%s}]},`+
		`"status":{%s}}`, o.name, ago(o.created), src, o.incident, safety, status)
}

func mustRA(t *testing.T, o raOpts) dorguv1.RemediationAction {
	t.Helper()
	var r dorguv1.RemediationAction
	require.NoError(t, json.Unmarshal([]byte(raJSON(o)), &r))
	return r
}

func incidentJSON(name string, firstSeen, created time.Duration, outcome string, conds ...string) string {
	res := ""
	if outcome != "" {
		res = fmt.Sprintf(`,"resolution":{"outcome":%q}`, outcome)
	}
	return fmt.Sprintf(`{"metadata":{"name":%q,"namespace":"shop","creationTimestamp":%q},`+
		`"spec":{"detection":{"firstSeen":%q}%s},"status":{"conditions":[%s]}}`,
		name, ago(created), ago(firstSeen), res, strings.Join(conds, ","))
}

func mustIncident(t *testing.T, raw string) dorguv1.IncidentMemory {
	t.Helper()
	var i dorguv1.IncidentMemory
	require.NoError(t, json.Unmarshal([]byte(raw), &i))
	return i
}

func eventJSON(reason string, at time.Duration, count int) string {
	return fmt.Sprintf(`{"reason":%q,"type":"Warning","lastTimestamp":%q,"count":%d,`+
		`"involvedObject":{"kind":"RemediationAction","name":"x"}}`, reason, ago(at), count)
}

// micro renders testNow minus d the way the API server writes a MicroTime.
func micro(d time.Duration) string { return testNow.Add(-d).Format("2006-01-02T15:04:05.000000Z07:00") }

func mustEvent(t *testing.T, raw string) corev1.Event {
	t.Helper()
	var e corev1.Event
	require.NoError(t, json.Unmarshal([]byte(raw), &e))
	return e
}

const approvalGatePass = `{"phase":"ApprovalRecheck","result":"Pass","verdicts":[{"stage":"Guardrails","guardrail":"blast-radius","verdict":"Pass"}]}`

const approvalGateRefused = `{"phase":"ApprovalRecheck","result":"Refuse","verdicts":[` +
	`{"stage":"Ground","guardrail":"persona","verdict":"Pass"},` +
	`{"stage":"Guardrails","guardrail":"blast-radius","verdict":"Refuse"}]}`

const clampedSafety = `[{"rule":"blast-radius","verdict":"clamped","field":"spec.resources.limits.memory","message":"m"},` +
	`{"rule":"absent-field","verdict":"rejected","field":"spec.resources.limits.cpu","message":"m"}]`

// reportFixture is a cluster with every way a RemediationAction can end, two
// proposal-time refusals, and CLI declines. The numbers the tests and the golden
// files assert on are derived from it by hand.
func reportFixture(t *testing.T) Inputs {
	t.Helper()
	h := time.Hour
	min := time.Minute

	ras := []raOpts{
		// 1. A clean heal, with a first-run clamp.
		{name: "ra-clean", phase: "Completed", created: 50 * h, approvedAt: 49*h + 30*min, appliedAt: 49*h + 29*min,
			incident: "inc-clean", gate: approvalGatePass, safety: clampedSafety, verification: "Healthy", conds: []string{
				cond("Ticketed", "True", "TicketIssued", 49*h+29*min),
				cond("HealCredited", "True", "WorkloadPatchedByTicket", 49*h+19*min),
				cond("Verified", "True", "HealthRestored", 49*h+19*min),
			}},
		// 2. Verified healthy, but the workload diverged: partial, not a heal.
		{name: "ra-partial", phase: "Completed", created: 40 * h, approvedAt: 39 * h, appliedAt: 39*h - min,
			incident: "inc-partial", gate: approvalGatePass, verification: "Healthy", conds: []string{
				cond("WorkloadDiverged", "True", "WorkloadDiverged", 38*h),
				cond("Verified", "True", "HealthRestored", 38*h),
			}},
		// 3. Recovered on its own: nothing was patched.
		{name: "ra-self", phase: "Completed", created: 30 * h, approvedAt: 29 * h, appliedAt: 29*h - min,
			incident: "inc-self", gate: approvalGatePass, verification: "Healthy", conds: []string{
				cond("HealCredited", "False", "NothingPatched", 28*h),
				cond("Verified", "True", "HealthRestored", 28*h),
			}},
		// 4. Something changed the workload, but not to the ticket's values.
		{name: "ra-notticket", phase: "Completed", created: 20 * h, approvedAt: 19 * h, appliedAt: 19*h - min,
			incident: "inc-notticket", gate: approvalGatePass, verification: "Healthy", conds: []string{
				cond("HealCredited", "False", "ResourcesNotTicket", 18*h),
				cond("Verified", "True", "HealthRestored", 18*h),
			}},
		// 5. Rolled back.
		{name: "ra-rolledback", phase: "RolledBack", created: 25 * h, approvedAt: 24 * h, appliedAt: 24*h - min,
			incident: "inc-rolledback", gate: approvalGatePass, verification: "Degraded"},
		// 6. Failed after apply.
		{name: "ra-failed", phase: "Failed", created: 26 * h, approvedAt: 25 * h, incident: "inc-failed"},
		// 7 to 9. Rejected, with and without a reason.
		{name: "ra-rej-diag", phase: "Rejected", created: 10 * h, incident: "inc-rej1", conds: []string{
			cond("Rejected", "True", "UserRejected", 9*h), cond("RejectionReason", "True", "WrongDiagnosis", 9*h)}},
		{name: "ra-rej-none", phase: "Rejected", created: 11 * h, incident: "inc-rej2", conds: []string{
			cond("Rejected", "True", "UserRejected", 10*h)}},
		{name: "ra-rej-risky", phase: "Rejected", created: 12 * h, incident: "inc-rej3", conds: []string{
			cond("Rejected", "True", "UserRejected", 11*h), cond("RejectionReason", "True", "TooRisky", 11*h)}},
		// 10. Refused at approval by the gate, in the guardrails stage.
		{name: "ra-refused-approval", phase: "Failed", created: 8 * h, approvedAt: 7 * h, incident: "inc-ref1",
			gate: approvalGateRefused, conds: []string{
				cond("Applied", "False", "GateRefusedAtApproval", 7*h),
				cond("GateRefused", "True", "BlastRadius", 7*h)}},
		// 11. Refused at apply: the ticket expired.
		{name: "ra-refused-apply", phase: "Failed", created: 6 * h, approvedAt: 5 * h, incident: "inc-ref2",
			gate: approvalGatePass, conds: []string{
				cond("Applied", "False", "PreconditionRejected", 5*h),
				cond("GateRefused", "True", "TicketExpired", 5*h)}},
		// 12. Still waiting for a human.
		{name: "ra-pending", phase: "Pending", created: 2 * h, incident: "inc-pending", planSource: "ai-anthropic"},
		// Outside the window: must not be counted anywhere.
		{name: "ra-old", phase: "Completed", created: 10 * 24 * h, approvedAt: 10*24*h - h, appliedAt: 10*24*h - 2*h,
			incident: "inc-old", gate: approvalGatePass, conds: []string{
				cond("HealCredited", "True", "WorkloadPatchedByTicket", 10*24*h-3*h)}},
	}

	incidents := []string{
		incidentJSON("inc-clean", 51*h, 51*h, "resolved"),
		incidentJSON("inc-partial", 41*h, 41*h, "partial"),
		incidentJSON("inc-self", 31*h, 31*h, "self-resolved"),
		incidentJSON("inc-notticket", 21*h, 21*h, "self-resolved"),
		incidentJSON("inc-rolledback", 26*h, 26*h, "rollback"),
		incidentJSON("inc-failed", 27*h, 27*h, "failed"),
		incidentJSON("inc-old", 11*24*h, 11*24*h, "resolved"),
		// Proposal-time refusals that created no RemediationAction.
		incidentJSON("inc-mode", 5*h, 5*h, "", cond("ProposalRefused", "True", "ModePolicy_SelfHealingMode", 4*h)),
		incidentJSON("inc-gone", 3*h, 3*h, "", cond("ProposalRefused", "True", "Ground_WorkloadGone", 2*h)),
		// An outstanding proposal, as an operator before the fix recorded it: not a
		// refusal, and it must not be counted as one or as a second proposal.
		incidentJSON("inc-pending", 3*h, 3*h, "", cond("ProposalRefused", "True", "DedupCooldown_ActiveRemediation", 2*h)),
		// A refusal a later proposal cleared is not counted.
		incidentJSON("inc-cleared", 4*h, 4*h, "", cond("ProposalRefused", "False", "Proposed", 3*h)),
	}

	events := []string{
		eventJSON("DorguCLIDeclined", 2*h, 1),
		eventJSON("DorguCLIDeclined", 3*h, 1),
		eventJSON("DorguCLITicketMismatch", 4*h, 1),
		eventJSON("DorguCLINoTicket", 5*h, 3),
		eventJSON("DorguCLITicketExpired", 6*h, 1),
		eventJSON("DorguCLIDeclined", 9*24*h, 1), // outside the window
		eventJSON("DorguGroundRefused", 3*h, 1),  // the operator's, not a CLI decline
	}

	in := Inputs{EventsRead: true}
	for _, o := range ras {
		in.Remediations = append(in.Remediations, mustRA(t, o))
	}
	for _, raw := range incidents {
		in.Incidents = append(in.Incidents, mustIncident(t, raw))
	}
	for _, raw := range events {
		in.Events = append(in.Events, mustEvent(t, raw))
	}
	return in
}

func strp(s string) *string { return &s }

// Short names for the operator types the fixtures build.
type (
	remActionForTest = dorguv1.RemediationAction
	incidentForTest  = dorguv1.IncidentMemory
	eventForTest     = corev1.Event
)
