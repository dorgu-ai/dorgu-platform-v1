package report

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// Build computes the report from the cluster reads, a fixed clock and the
// window. It is a pure function: the same inputs give the same document.
//
// Time bases, so a number can be checked against the cluster: a RemediationAction
// and an IncidentMemory count when they were created inside the window; a
// proposal-time refusal counts when its ProposalRefused condition was recorded;
// an Event counts at its last timestamp.
func Build(in Inputs, now time.Time, since time.Duration) Doc {
	start := now.Add(-since)
	w := windowOf(now, start, since)

	ras := raInWindow(in.Remediations, start)
	incidents := incidentIndex(in.Incidents)

	doc := Doc{
		Schema:      Schema,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Window:      w,
	}
	doc.InputErrors = append([]InputError{}, in.Errors...)

	// What cannot be known is no data, with the reason, and never a zero. The
	// incidents carry the resolution outcome and the proposal-time refusals; the
	// operator before v0.13.0 writes neither tickets nor HealCredited.
	incidentsGone := ""
	if in.unreadable("incidents") {
		incidentsGone = "the IncidentMemories could not be read, and the resolution outcome and proposal-time refusals live on them"
	}
	healGone := incidentsGone
	if healGone == "" {
		healGone = ticketSupportGone(in.OperatorVersion, ras)
	}
	doc.HealAccounting = accountForHeals(ras, incidents, healGone)

	decisions := countDecisions(ras)
	refusals := collectRefusals(ras, in.Incidents, in.Remediations, start)

	doc.TierA = buildTierA(doc.HealAccounting, decisions, since)
	q6 := refusalMix(len(ras)+refusals.proposalTime, refusals.rows, refusals.deferred)
	if incidentsGone != "" {
		q6 = Refusals{NoData: true, Reason: incidentsGone, Rows: []Refusal{}}
	}
	doc.TierQ = TierQ{
		Q1TimeToHeal: timeToHeal(ras, in.Remediations, incidents, incidentsGone),
		Q2Precision:  precision(decisions),
		Q3LLM:        llmEfficiency(ras),
		Q6Refusals:   q6,
	}
	doc.FirstRunClamps = firstRunClamps(ras)
	doc.CLIDeclines = cliDeclines(in, start)
	doc.NotMeasurable = notMeasurable()
	return doc
}

func windowOf(now, start time.Time, since time.Duration) Window {
	return Window{
		Since: FormatWindow(since),
		Start: start.UTC().Format(time.RFC3339),
		End:   now.UTC().Format(time.RFC3339),
	}
}

// raInWindow keeps the RemediationActions created inside the window. One whose
// creation time is unreadable cannot be placed in it and is left out.
func raInWindow(all []remAction, start time.Time) []*remAction {
	var out []*remAction
	for i := range all {
		if t, ok := createdAt(&all[i]); ok && !t.Before(start) {
			out = append(out, &all[i])
		}
	}
	return out
}

// incidentIndex finds an incident by namespace/name.
func incidentIndex(all []incidentMemory) map[string]*incidentMemory {
	idx := make(map[string]*incidentMemory, len(all))
	for i := range all {
		idx[all[i].Namespace+"/"+all[i].Name] = &all[i]
	}
	return idx
}

// incidentOf is the incident a RemediationAction answers, if it can be read.
func incidentOf(ra *remAction, idx map[string]*incidentMemory) *incidentMemory {
	ns := ra.Spec.IncidentRef.Namespace
	if ns == "" {
		ns = ra.Namespace
	}
	return idx[ns+"/"+ra.Spec.IncidentRef.Name]
}

// decisions is what humans decided in the window.
type decisions struct {
	approved, rejected int
	rejections         map[string]int
}

// countDecisions counts approvals and rejections. A RemediationAction that was
// approved and then rejected is a rejection, counted once.
func countDecisions(ras []*remAction) decisions {
	d := decisions{rejections: map[string]int{}}
	for _, r := range rejectionReasons {
		d.rejections[r.flag] = 0
	}
	d.rejections[NoReasonGiven] = 0

	for _, ra := range ras {
		switch {
		case ra.Status.Phase == "Rejected":
			d.rejected++
			d.rejections[rejectionReasonOf(ra)]++
		case ra.Status.ApprovedAt != nil && !ra.Status.ApprovedAt.IsZero():
			d.approved++
		}
	}
	return d
}

// rejectionReasonOf is the enum a rejection was recorded with, no-reason-given when
// the RemediationAction carries none, and the raw reason when it is one this CLI
// does not know (a newer CLI wrote it).
func rejectionReasonOf(ra *remAction) string {
	c := condition(ra.Status.Conditions, conditionRejectionReason)
	if c == nil || c.Status != metav1.ConditionTrue || c.Reason == "" {
		return NoReasonGiven
	}
	if flag := rejectionFlagFor(c.Reason); flag != "" {
		return flag
	}
	if c.Reason == "WrongAction" {
		// Written by a build before the enum was final. A wrong fix for a right
		// diagnosis is not a wrong diagnosis, so it falls in the "other" bucket.
		return "other"
	}
	return "unrecognised:" + c.Reason
}

// wrongRejections is the numerator of Q2: rejections whose reason says Dorgu
// misread the problem. The other reasons (too risky, not now, duplicate, not
// needed, other) are shown as a breakdown but do not call the diagnosis wrong.
func wrongRejections(d decisions) int {
	return d.rejections["wrong-diagnosis"]
}

func buildTierA(h HealAccounting, d decisions, since time.Duration) TierA {
	decided := d.approved + d.rejected
	weeks := since.Hours() / (24 * 7)

	a := TierA{
		NorthStar:  NorthStar{SafeHeals: h.SafeHeals, CleanHeal: h.Clean, NoData: h.NoData, Reason: h.Reason},
		A2Active:   Active{Active: decided > 0, ProposalsDecided: decided},
		A5Depth:    Depth{ProposalsDecided: decided, WindowDays: since.Hours() / 24, NoData: decided == 0},
		A6Approval: Approval{Approved: d.approved, Rejected: d.rejected, Rate: ratio(d.approved, decided), NoData: decided == 0},
		A7External: NoDataReason{NoData: true, Reason: "a RemediationAction records how its plan was written (planSource), " +
			"not who proposed it; there is no source field to count until proposals can come from other tools, so this is not measured yet"},
		Reviewed: Decided{Approved: d.approved, Rejected: d.rejected},
		ClusterNotes: []string{
			"A2 and A5 count decisions (approved or rejected). Reading a proposal with `dorgu remediation diff` leaves no record, so a decision is the measurable proxy for a review.",
			"A6 is a band, not a target: under 30% means noisy proposals, over 80% means rubber-stamping.",
		},
	}
	if decided > 0 {
		perWeek := float64(decided) / weeks
		a.A5Depth.PerWeek = &perWeek
	}
	if decided > 0 && decided < minDecisions {
		// The counts are real; a rate and a band over so few would not be.
		a.A6Approval.Rate, a.A6Approval.TooFewDecisions = nil, true
	}
	if a.A6Approval.Rate != nil {
		switch rate := *a.A6Approval.Rate; {
		case rate < 0.30:
			a.A6Approval.Band = "below"
		case rate > 0.80:
			a.A6Approval.Band = "above"
		default:
			a.A6Approval.Band = "in"
		}
	}
	return a
}

func precision(d decisions) Precision {
	decided := d.approved + d.rejected
	wrong := wrongRejections(d)
	p := Precision{
		Decided: decided, Wrong: wrong, Rate: ratio(wrong, decided), NoData: decided == 0,
		RejectionsByReason: d.rejections,
	}
	if decided > 0 && decided < minDecisions {
		p.Rate, p.TooFewDecisions = nil, true
	}
	return p
}

func llmEfficiency(ras []*remAction) LLM {
	sources := map[string]int{}
	for _, ra := range ras {
		src := ra.Spec.PlanSource
		if src == "" {
			src = "unspecified"
		}
		sources[src]++
	}
	return LLM{
		NoData: true,
		Reason: "LLM calls and cost per incident are operator metrics (dorgu_llm_requests_total, " +
			"dorgu_llm_tokens_total), not stored on any CRD",
		PlanSources: sources,
	}
}

func firstRunClamps(ras []*remAction) Clamps {
	c := Clamps{
		NoData:       len(ras) == 0,
		StepVerdicts: map[string]int{},
		ClampsByRule: map[string]int{},
		GateClamps:   map[string]int{},
	}
	for _, ra := range ras {
		clamped := false
		for _, step := range ra.Spec.Steps {
			for _, s := range step.Safety {
				c.StepVerdicts[s.Verdict]++
				if s.Verdict == dorguv1.SafetyVerdictClamped {
					c.ClampsByRule[s.Rule]++
					clamped = true
				}
			}
		}
		if clamped {
			c.ProposalsWithClamp++
		}
		if ra.Status.Gate != nil {
			for _, v := range ra.Status.Gate.Verdicts {
				if strings.EqualFold(v.Verdict, "Clamp") {
					c.GateClamps[v.Guardrail]++
				}
			}
		}
	}
	return c
}

// cliDeclines counts the CLI's own decline Events inside the window.
func cliDeclines(in Inputs, start time.Time) CLIDeclines {
	out := CLIDeclines{
		ByReason: map[string]int{},
		Note: "The CLI records a decline only as a Kubernetes Event, and Kubernetes keeps Events for about an hour " +
			"unless the cluster is configured otherwise, so declines older than that are not counted here. " +
			"A refusal because the kube context is protected (DORGU_PROTECTED_CONTEXTS) is local only: the CLI writes " +
			"nothing to a cluster it was told not to touch, so those are never counted.",
	}
	if !in.EventsRead {
		out.NoData = true
		out.Reason = "the Events could not be read"
		return out
	}
	for i := range in.Events {
		e := &in.Events[i]
		if !strings.HasPrefix(e.Reason, reasonCLIPrefix) || e.InvolvedObject.Kind != "RemediationAction" {
			continue
		}
		if at, ok := eventAt(e); !ok || at.Before(start) {
			continue
		}
		n := eventOccurrences(e)
		if first, ok := timeOf(e.FirstTimestamp); ok && first.Before(start) {
			// A folded Event whose series began before the window: only the occurrence
			// at its last timestamp is known to be inside it.
			n = 1
		}
		out.ByReason[e.Reason] += n
	}
	return out
}

func notMeasurable() []NotMeasurable {
	return []NotMeasurable{
		{"A1", "External installs", "a cluster cannot tell whether it is ours; read from partner reports"},
		{"A3", "Design partners", "comes from the conversation log"},
		{"A4", "Time to first value", "measured on a clean-room or screen-shared install, not from CRDs"},
		{"A8", "4-week retention", "needs reports from earlier weeks; one cluster's report cannot show it"},
		{"A9", "Would-pay signals", "comes from the conversation log"},
		{"A10", "Inbound", "comes from GitHub"},
		{"Q4", "Operator footprint", "not measured by this report: read the operator's CPU and memory from its own metrics or your cluster's monitoring"},
		{"Q5", "Clean-room", "comes from clean-room runs"},
	}
}

// SortedKeys returns a map's keys in order, for stable text output.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ticketSupportGone is the reason the heal sections cannot be trusted because the
// operator may predate tickets, or "" when they can. It fails closed, by evidence
// and not by tag:
//
//   - a tag that is a release before v0.13.0 is no data, whatever the records say;
//   - a tag that is a release at v0.13.0 or later is trusted (a fast path);
//   - when the version cannot be determined (an unreadable Deployment list, an image
//     pinned by digest, a tag that is not a version, a Deployment without the label)
//     the records decide: every Completed RemediationAction from v0.13.0 on carries a
//     HealCredited condition, so if any Completed action in the window has one the
//     operator writes them and the data is trusted, and if there are Completed
//     actions and none has one, every heal would read as unconfirmed and the north
//     star as a measured zero. That is no data, with this reason;
//   - with no Completed action there is nothing to blank, and zero is true.
func ticketSupportGone(version *string, ras []*remAction) string {
	if version != nil {
		if OperatorPredatesTickets(version) {
			return fmt.Sprintf("the operator (%s) predates %s, which writes the apply ticket and the HealCredited record a clean heal is judged on",
				*version, TicketOperator)
		}
		if _, ok := ParseVersion(*version); ok {
			return ""
		}
	}
	completed := 0
	for _, ra := range ras {
		if ra.Status.Phase != "Completed" {
			continue
		}
		completed++
		if condition(ra.Status.Conditions, conditionHealCredited) != nil {
			return ""
		}
	}
	if completed == 0 {
		return ""
	}
	return fmt.Sprintf("the operator version could not be determined and none of the %d Completed RemediationAction(s) in the window carries a HealCredited record; "+
		"an operator older than %s writes none, so the heals cannot be judged", completed, TicketOperator)
}

// OperatorVersionKnown reports whether the tag is a release version.
func OperatorVersionKnown(tag *string) bool {
	if tag == nil {
		return false
	}
	_, ok := ParseVersion(*tag)
	return ok
}

// OperatorPredatesTickets reports whether the operator's image tag is a release
// before v0.13.0. A tag that is not a version (a dev build, a digest, nothing) is
// unknown, and unknown is never marked: the report only says what it can show.
func OperatorPredatesTickets(tag *string) bool {
	if tag == nil {
		return false
	}
	have, ok := ParseVersion(*tag)
	if !ok {
		return false
	}
	want, _ := ParseVersion(TicketOperator)
	for i := range have {
		if have[i] != want[i] {
			return have[i] < want[i]
		}
	}
	return false
}

// ParseVersion reads vMAJOR.MINOR.PATCH, ignoring a pre-release or build suffix.
func ParseVersion(tag string) ([3]int, bool) {
	var v [3]int
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if i := strings.IndexAny(tag, "-+"); i >= 0 {
		tag = tag[:i]
	}
	parts := strings.Split(tag, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}
