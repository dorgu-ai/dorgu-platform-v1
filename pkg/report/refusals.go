package report

import (
	"slices"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// refusalKey identifies one row of the refusal mix.
type refusalKey struct{ phase, stage, reason string }

// collected is every refusal found, and how many of them happened before a
// RemediationAction existed.
type collected struct {
	rows         map[refusalKey]int
	proposalTime int

	// deferred counts incidents whose proposal is outstanding: not refusals.
	deferred int
}

// DeferralsNotRecorded is what the report says about deferrals. The operator deliberately does
// not write them on objects (B6: a deferral repeats every cycle and is not an event in the life of
// an incident), so the report cannot count them and must not say 0.
const DeferralsNotRecorded = "not recorded on cluster objects (see the dorgu_gate_decisions_total metric, result=deferred)"

// isDeferredProposal reports whether a ProposalRefused reason is really an
// outstanding proposal. The operator no longer writes it, but an operator before
// the fix wrote DedupCooldown_ActiveRemediation on every open incident one cycle
// after proposing, and those conditions stand until the incident goes. An incident
// waiting for a human to decide is the normal state, not a refusal, and it already
// has the RemediationAction that is waiting.
func isDeferredProposal(stage, guardrail string) bool {
	return stage == "DedupCooldown" && guardrail == "ActiveRemediation"
}

// actionsByIncident indexes every RemediationAction's phase by the incident it
// answers (namespace/name), whatever the window.
func actionsByIncident(all []remAction) map[string][]string {
	idx := map[string][]string{}
	for i := range all {
		ns := all[i].Spec.IncidentRef.Namespace
		if ns == "" {
			ns = all[i].Namespace
		}
		key := ns + "/" + all[i].Spec.IncidentRef.Name
		idx[key] = append(idx[key], all[i].Status.Phase)
	}
	return idx
}

// ownFixPhases are the phases in which a RemediationAction is the incident's own
// fix, pending or in flight. A finished one (Completed, Acknowledged, RolledBack,
// Failed, Rejected, Expired) is not: the incident can be re-proposed after it, and a
// refusal standing beside it is then a real one.
var ownFixPhases = []string{"Pending", "Approved", "Ticketed", "Applying", "Verifying"}

// isOwnFixInFlight reports whether a standing Guardrails_Concurrent refusal is the
// incident's own proposal and not another incident's fix holding the persona. The
// gate answers a repeat proposal with Guardrails/concurrent for as long as the
// incident's own RemediationAction is in flight (an operator before the fix left the
// condition standing then), and nothing is refused: the proposal exists. The same
// holds once the incident is Resolved (the operator clears the condition, an older
// one did not). Only those two cases. A resolution record that is not Resolved
// (a rollback leaves the incident open and re-proposable) and a finished action of
// its own, however old, do not make a real refusal a deferral. With no pending or
// in-flight action of its own the persona is held by another incident, and that
// stays a refusal.
func isOwnFixInFlight(stage, guardrail string, inc *incidentMemory, own map[string][]string) bool {
	if stage != "Guardrails" || guardrail != "Concurrent" {
		return false
	}
	if inc.Status.Phase == "Resolved" {
		return true
	}
	for _, phase := range own[inc.Namespace+"/"+inc.Name] {
		if slices.Contains(ownFixPhases, phase) {
			return true
		}
	}
	return false
}

// collectRefusals gathers the refusals the operator persisted, from CRD state
// (Events expire, conditions do not):
//
//   - approval: a RemediationAction Failed with Applied=GateRefusedAtApproval. The
//     stage is the refusing verdict in status.gate; the reason is GateRefused's
//     reason, which is the guardrail enum.
//   - apply: Applied=PreconditionRejected. The ticket check is not a pipeline stage,
//     so the stage is "Ticket".
//   - proposal: an IncidentMemory ProposalRefused condition that is True, recorded as
//     <Stage>_<Reason>. Those created no RemediationAction.
func collectRefusals(ras []*remAction, incidents []incidentMemory, allRAs []remAction, start time.Time) collected {
	c := collected{rows: map[refusalKey]int{}}
	ownActions := actionsByIncident(allRAs)

	for _, ra := range ras {
		refused := condition(ra.Status.Conditions, conditionGateRefused)
		if refused == nil || refused.Status != "True" {
			continue
		}
		phase, stage := PhaseApproval, refusingStage(ra)
		if appliedReasonOf(ra) == reasonPreconditionRejected {
			phase, stage = PhaseApply, StageTicket
		}
		c.rows[refusalKey{phase, stage, refused.Reason}]++
	}

	for i := range incidents {
		cond, recorded := proposalRefusalOf(&incidents[i])
		if cond == nil {
			continue
		}
		at, ok := timeOf(cond.LastTransitionTime)
		if !ok {
			at, ok = timeOf(incidents[i].CreationTimestamp)
		}
		if !ok || at.Before(start) {
			continue
		}
		stage, reason := splitProposalRefusal(cond.Reason)
		if isDeferredProposal(stage, reason) || isOwnFixInFlight(stage, reason, &incidents[i], ownActions) {
			c.deferred++
			continue
		}
		c.rows[refusalKey{PhaseProposal, stage, reason}]++
		// A refusal that was answered by a proposal (the operator proposes the rest of a plan
		// it refused part of) has its proposal among the RemediationActions already.
		if !recorded || len(ownActions[incidents[i].Namespace+"/"+incidents[i].Name]) == 0 {
			c.proposalTime++
		}
	}
	return c
}

// proposalRefusalOf returns the condition that says the gate refused a proposal for the
// incident, and whether it is the operator's durable record.
//
// ProposalRefused is the live state: it is rewritten to False when the incident resolves or a
// later proposal answers it, which erased 3 refusals in D1.1. From operator v0.13.1 the same
// refusal is also kept as ProposalRefusalRecorded, which is never cleared; when present it is
// the record. An older operator writes only the live condition, which is read as before (a
// refusal it cleared is gone). Both present is one refusal, dated by the later of the two.
func proposalRefusalOf(inc *incidentMemory) (cond *metav1.Condition, recorded bool) {
	live := condition(inc.Status.Conditions, conditionProposalRefused)
	if live != nil && live.Status != "True" {
		live = nil
	}
	if rec := condition(inc.Status.Conditions, conditionProposalRefusalRecorded); rec != nil && rec.Status == "True" {
		// A refusal that comes back for the same reason does not move the record's time on an
		// operator before v0.13.1, while the live condition is dated by it. The later of the
		// two is when the incident was last refused.
		if live != nil {
			if recAt, ok := timeOf(rec.LastTransitionTime); ok {
				if liveAt, ok := timeOf(live.LastTransitionTime); ok && liveAt.After(recAt) {
					latest := *rec
					latest.LastTransitionTime = live.LastTransitionTime
					return &latest, true
				}
			}
		}
		return rec, true
	}
	if live != nil {
		return live, false
	}
	return nil, false
}

func appliedReasonOf(ra *remAction) string {
	if c := condition(ra.Status.Conditions, conditionApplied); c != nil {
		return c.Reason
	}
	return ""
}

// refusingStage is the stage of the first Refuse verdict in status.gate.
func refusingStage(ra *remAction) string {
	if g := ra.Status.Gate; g != nil {
		for _, v := range g.Verdicts {
			if strings.EqualFold(v.Verdict, "Refuse") && v.Stage != "" {
				return v.Stage
			}
		}
	}
	return StageUnknown
}

// splitProposalRefusal splits the IncidentMemory ProposalRefused reason, an enum
// compound of the stage and the guardrail (ModePolicy_SelfHealingMode), on its
// first underscore. A reason with no stage in it is kept whole under Unknown.
func splitProposalRefusal(reason string) (stage, guardrail string) {
	if stage, guardrail, ok := strings.Cut(reason, "_"); ok && stage != "" && guardrail != "" {
		return stage, guardrail
	}
	return StageUnknown, reason
}

// refusalMix turns the collected refusals into Q6's rows, largest first, and flags
// any reason that is a large share of proposals.
func refusalMix(proposals int, rows map[refusalKey]int, deferred int) Refusals {
	out := Refusals{Proposals: proposals, NoData: proposals == 0, Rows: []Refusal{}, DeferredNote: DeferralsNotRecorded}
	if deferred > 0 {
		// Only an operator before the fix wrote deferrals onto objects, so this is a floor.
		out.Deferred = &deferred
	}
	for k, n := range rows {
		share := ratio(n, proposals)
		out.Rows = append(out.Rows, Refusal{
			Phase: k.phase, Stage: k.stage, Reason: k.reason, Count: n, ShareOfProposals: share,
			Flagged: share != nil && *share > refusalFlagShare && proposals >= minSampleForFlag,
		})
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		a, b := out.Rows[i], out.Rows[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Phase != b.Phase {
			return a.Phase < b.Phase
		}
		if a.Stage != b.Stage {
			return a.Stage < b.Stage
		}
		return a.Reason < b.Reason
	})
	return out
}
