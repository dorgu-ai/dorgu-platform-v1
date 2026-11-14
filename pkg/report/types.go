package report

// Schema names the JSON shape `dorgu report --json` prints. It changes only
// when a field is removed or its meaning changes; the golden test pins it.
const Schema = "dorgu.report/v1"

// The report never prints a zero as if it were a measurement. A ratio or a
// percentile that has no sample behind it is null with no_data true, a count is
// an integer only where the cluster could be read, and anything the cluster
// cannot tell us (A1, A3, A4, A8, A9, A10) is listed under not_measurable.

// Doc is the whole report.
type Doc struct {
	Schema      string  `json:"schema"`
	GeneratedAt string  `json:"generated_at"`
	Window      Window  `json:"window"`
	Cluster     Cluster `json:"cluster"`
	Sources     Sources `json:"sources"`

	// InputErrors names every cluster read that failed, with a bounded class. A
	// section that depends on a failed read is no_data with a reason, never zero.
	InputErrors []InputError `json:"input_errors"`

	// HealAccounting is how every RemediationAction created in the window ended,
	// and which of them are clean heals under KPIS.md.
	HealAccounting HealAccounting `json:"heal_accounting"`

	TierA TierA `json:"tier_a"`
	TierQ TierQ `json:"tier_q"`

	// FirstRunClamps is what the guardrails narrowed before a human saw a plan.
	FirstRunClamps Clamps `json:"first_run_clamps"`

	// CLIDeclines are refusals the CLI wrote as Events on the RemediationAction.
	CLIDeclines CLIDeclines `json:"cli_declines"`

	// NotMeasurable lists the KPIs a cluster cannot report, so their absence
	// reads as "not measured" and never as zero.
	NotMeasurable []NotMeasurable `json:"not_measurable"`
}

type Window struct {
	Since string `json:"since"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// Cluster is context only: no number in the report is computed from it.
// application_personas is null when the ApplicationPersonas could not be read
// (never 0, which would claim the cluster has none), and self_healing_modes is null
// or empty when the ClusterPersonas could not be read or declare none. They are kept
// because a design partner pasting the report is asked what mode the cluster runs in.
type Cluster struct {
	Context         string   `json:"context"`
	Namespace       string   `json:"namespace"`
	OperatorVersion *string  `json:"operator_version"`
	Personas        *int     `json:"application_personas"`
	SelfHealing     []string `json:"self_healing_modes"`
}

// Sources says which cluster reads worked. A report built from a partial
// read says so rather than presenting the gap as zero.
type Sources struct {
	RemediationActions Source `json:"remediation_actions"`
	Incidents          Source `json:"incidents"`
	Personas           Source `json:"personas"`
	ClusterPersonas    Source `json:"cluster_personas"`
	Events             Source `json:"events"`
}

// InputError is one failed cluster read: which input, and what kind of
// failure (forbidden, not_found, timeout, unparseable, error).
type InputError struct {
	Input string `json:"input"`
	Error string `json:"error"`
}

type Source struct {
	OK    bool   `json:"ok"`
	Count int    `json:"count"`
	Error string `json:"error,omitempty"`
}

// --- heal accounting ---

type HealAccounting struct {
	// Total is the RemediationActions created in the window.
	Total int `json:"total"`

	// Completed reached phase Completed. Clean is the subset KPIS.md counts as a
	// heal: HealCredited=True and the incident outcome resolved. Clean and SafeHeals
	// are null when they cannot be known: the incidents could not be read, or the
	// operator predates the records they are computed from (NoData, Reason).
	Completed int  `json:"completed"`
	Clean     *int `json:"clean"`

	// SafeHeals is the north star (NS): clean heals whose approval-time gate run
	// passed (status.gate is an ApprovalRecheck that cleared).
	SafeHeals *int `json:"safe_heals"`

	// NoData is true when the clean-heal accounting cannot be computed, and Reason
	// says why. It is never a zero.
	NoData bool   `json:"no_data"`
	Reason string `json:"reason,omitempty"`

	// Excluded is every RemediationAction that is not a clean heal, by why. Empty
	// when NoData.
	Excluded map[string]int `json:"excluded"`

	// HealNotCredited breaks HealCredited=False down by its reason.
	HealNotCredited map[string]int `json:"heal_not_credited_by_reason"`
}

// Exclusion categories. Each RemediationAction lands in exactly one of Clean or
// these, so Total = Clean + the sum of Excluded.
const (
	ExcludedPartial           = "partial"
	ExcludedSelfResolved      = "self_resolved"
	ExcludedHealNotCredited   = "heal_not_credited"
	ExcludedNoCreditRecord    = "no_credit_record"
	ExcludedOutcomeUnknown    = "outcome_unconfirmed"
	ExcludedRolledBack        = "rolled_back"
	ExcludedApprovedNotHealed = "approved_not_healed"
	ExcludedFailed            = "failed"
	ExcludedRejected          = "rejected"
	ExcludedExpired           = "expired"
	ExcludedAcknowledged      = "acknowledged"
	ExcludedRefusedAtGate     = "refused_by_gate"
	ExcludedInFlight          = "in_flight"
)

// AllExclusions is every category, so the report lists all of them with their
// counts, zeros included: a zero here is a measurement.
var AllExclusions = []string{
	ExcludedPartial, ExcludedSelfResolved, ExcludedHealNotCredited, ExcludedNoCreditRecord,
	ExcludedOutcomeUnknown, ExcludedRolledBack, ExcludedApprovedNotHealed, ExcludedFailed, ExcludedRejected,
	ExcludedExpired, ExcludedAcknowledged, ExcludedRefusedAtGate, ExcludedInFlight,
}

// Refusal phases and the stage the CLI cannot read off the gate.
const (
	PhaseProposal = "proposal"
	PhaseApproval = "approval"
	PhaseApply    = "apply"
	StageTicket   = "Ticket"
	StageUnknown  = "Unknown"

	// NoReasonGiven counts a rejection that carries no RejectionReason.
	NoReasonGiven = "no-reason-given"

	// minSampleForFlag and refusalFlagShare: a refusal reason is
	// flagged (KPI Q6) when it is over 30% of proposals on at least ten of them.
	minSampleForFlag = 10
	refusalFlagShare = 0.30

	// minDecisions is how many decided proposals a rate needs before the
	// report passes judgement on it (the approval band, "called wrong").
	minDecisions = 5

	// TicketOperator is the first operator release that issues apply tickets
	// and writes HealCredited. Earlier ones cannot answer the heal questions.
	TicketOperator = "v0.13.0"
)

// --- tier A ---

type TierA struct {
	// NorthStar mirrors heal_accounting.safe_heals so the headline number is
	// where KPIS.md says to look for it.
	NorthStar    NorthStar    `json:"ns_safe_heals"`
	A2Active     Active       `json:"a2_active_cluster"`
	A5Depth      Depth        `json:"a5_review_depth"`
	A6Approval   Approval     `json:"a6_approval_rate"`
	A7External   NoDataReason `json:"a7_external_agent_share"`
	Reviewed     Decided      `json:"decisions"`
	ClusterNotes []string     `json:"notes"`
}

type NorthStar struct {
	SafeHeals *int   `json:"safe_heals"`
	CleanHeal *int   `json:"clean_heals"`
	NoData    bool   `json:"no_data"`
	Reason    string `json:"reason,omitempty"`
}

// Active is A2: a cluster is active in the window when at least one
// proposal was decided. Reviews done with `diff` leave no record, so a decision
// is the measurable proxy.
type Active struct {
	Active           bool `json:"active"`
	ProposalsDecided int  `json:"proposals_decided"`
}

type Depth struct {
	ProposalsDecided int      `json:"proposals_decided"`
	WindowDays       float64  `json:"window_days"`
	PerWeek          *float64 `json:"per_week"`
	NoData           bool     `json:"no_data"`
}

type Approval struct {
	Approved int      `json:"approved"`
	Rejected int      `json:"rejected"`
	Rate     *float64 `json:"rate"`
	NoData   bool     `json:"no_data"`
	// TooFewDecisions is true when fewer than five proposals were decided: the
	// counts are real, a rate and a band would not be.
	TooFewDecisions bool `json:"too_few_decisions"`
	// Band is "below" (under 30%, noisy proposals), "in" or "above" (over 80%,
	// rubber-stamping). Empty with no_data or too few decisions.
	Band string `json:"band,omitempty"`
}

type Decided struct {
	Approved int `json:"approved"`
	Rejected int `json:"rejected"`
}

type NoDataReason struct {
	NoData bool   `json:"no_data"`
	Reason string `json:"reason"`
}

// --- tier Q ---

type TierQ struct {
	Q1TimeToHeal TimeToHeal `json:"q1_time_to_heal"`
	Q2Precision  Precision  `json:"q2_proposal_precision"`
	Q3LLM        LLM        `json:"q3_llm_efficiency"`
	Q6Refusals   Refusals   `json:"q6_refusal_mix"`
}

type TimeToHeal struct {
	DetectToProposal   Distribution `json:"detect_to_proposal"`
	ProposalToDecision Distribution `json:"proposal_to_decision"`
	DecisionToApplied  Distribution `json:"decision_to_applied"`
	AppliedToVerified  Distribution `json:"applied_to_verified"`
}

// Distribution is p50/p90 of one stage in seconds. N is the number of
// RemediationActions that had both timestamps.
type Distribution struct {
	N          int      `json:"n"`
	P50Seconds *float64 `json:"p50_seconds"`
	P90Seconds *float64 `json:"p90_seconds"`
	NoData     bool     `json:"no_data"`
	Reason     string   `json:"reason,omitempty"`
}

type Precision struct {
	Decided int `json:"decided"`
	Wrong   int `json:"wrong"`
	// Wrong counts rejections with reason wrong-diagnosis, the only reason that says
	// the proposal itself was wrong; Decided is every approved or rejected proposal.
	Rate            *float64 `json:"rate"`
	NoData          bool     `json:"no_data"`
	TooFewDecisions bool     `json:"too_few_decisions"`
	// RejectionsByReason counts rejected proposals by the enum `reject --reason`
	// writes, plus no-reason-given for a rejection that carries none.
	RejectionsByReason map[string]int `json:"rejections_by_reason"`
}

type LLM struct {
	NoData bool   `json:"no_data"`
	Reason string `json:"reason"`
	// PlanSources counts RemediationActions by how the plan was produced.
	PlanSources map[string]int `json:"plan_sources"`
}

type Refusals struct {
	// Proposals is the denominator: RemediationActions created plus proposals
	// the gate refused before one was written.
	Proposals int       `json:"proposals"`
	NoData    bool      `json:"no_data"`
	Reason    string    `json:"reason,omitempty"`
	Rows      []Refusal `json:"rows"`
	// Deferred counts incidents whose proposal is outstanding (the gate answers the
	// repeat attempt with active-remediation). It is waiting for a human, not a
	// refusal, and is in neither Rows nor Proposals.
	Deferred *int `json:"deferred_proposals"`
	// DeferredNote says where deferrals are actually recorded.
	DeferredNote string `json:"deferred_note"`
}

type Refusal struct {
	Phase  string `json:"phase"`
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`
	// ShareOfProposals is Count over Proposals.
	ShareOfProposals *float64 `json:"share_of_proposals"`
	// Flagged marks a reason above 30% of proposals on a sample of at least ten.
	Flagged bool `json:"flagged"`
}

// --- clamps and CLI declines ---

type Clamps struct {
	// ProposalsWithClamp is RemediationActions with at least one step whose
	// safety carries a clamped verdict (spec.steps[].safety): the first-run
	// clamp, which status.gate does not show because it records the seal.
	ProposalsWithClamp int `json:"proposals_with_clamp"`
	// StepVerdicts counts spec.steps[].safety entries by verdict.
	StepVerdicts map[string]int `json:"step_verdicts"`
	// ClampsByRule counts clamped entries by the guardrail that clamped.
	ClampsByRule map[string]int `json:"clamps_by_rule"`
	// GateClamps counts Clamp verdicts in status.gate by guardrail.
	GateClamps map[string]int `json:"gate_clamps_by_guardrail"`
	NoData     bool           `json:"no_data"`
}

type CLIDeclines struct {
	NoData   bool           `json:"no_data"`
	Reason   string         `json:"reason,omitempty"`
	ByReason map[string]int `json:"by_reason"`
	Note     string         `json:"note"`
}

type NotMeasurable struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Condition types, reasons and prefixes the operator and the CLI write that the
// report reads. They are restated here because this package must not import the
// CLI; the CLI has a test that fails if it writes one this package does not know.
const (
	conditionRejectionReason   = "RejectionReason"
	conditionGateRefused       = "GateRefused"
	conditionApplied           = "Applied"
	conditionWorkloadPatched   = "WorkloadPatched"
	reasonPreconditionRejected = "PreconditionRejected"

	// reasonCLIPrefix starts the reason of every Event the CLI writes when it
	// declines to act.
	reasonCLIPrefix = "DorguCLI"
)

// rejectionReason is one value of `dorgu remediation reject --reason` and the
// condition reason it is stored as.
type rejectionReason struct{ flag, condition string }

// rejectionReasons is the enum, in the order the CLI lists it.
var rejectionReasons = []rejectionReason{
	{"wrong-diagnosis", "WrongDiagnosis"},
	{"too-risky", "TooRisky"},
	{"not-now", "NotNow"},
	{"duplicate", "Duplicate"},
	{"not-needed", "NotNeeded"},
	{"other", "Other"},
}

// RejectionFlags lists the rejection reason flags the report knows, so a writer
// of the enum can check it has not drifted from the reader.
func RejectionFlags() []string {
	out := make([]string, len(rejectionReasons))
	for i, r := range rejectionReasons {
		out[i] = r.flag
	}
	return out
}

func rejectionFlagFor(condition string) string {
	for _, r := range rejectionReasons {
		if r.condition == condition {
			return r.flag
		}
	}
	return ""
}
