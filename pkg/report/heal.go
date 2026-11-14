package report

import (
	"strings"
)

// Condition types and reasons the operator writes that the report reads.
const (
	conditionHealCredited     = "HealCredited"
	conditionWorkloadDiverged = "WorkloadDiverged"
	conditionVerified         = "Verified"
	conditionRejected         = "Rejected"
	conditionProposalRefused  = "ProposalRefused"
	// conditionProposalRefusalRecorded is the durable record of a proposal refusal: unlike
	// ProposalRefused it is not cleared when the incident resolves or a proposal answers it.
	conditionProposalRefusalRecorded = "ProposalRefusalRecorded"
	incidentOutcomeResolved          = "resolved"
	incidentOutcomePartial           = "partial"
	incidentOutcomeSelfResolve       = "self-resolved"
)

// accountForHeals sorts every RemediationAction created in the window into a clean
// heal or the one reason it is not, following KPIS.md: a heal is clean when the
// RemediationAction Completed, HealCredited is True and the incident's outcome is
// resolved. Partial outcomes, self-resolved recoveries and HealCredited=False are
// never counted, however healthy the workload looks afterwards.
//
// unavailable, when not empty, says why the accounting cannot be done at all (the
// incidents were unreadable, or the operator predates the records it needs): the
// RemediationActions are still counted, but how they ended is no data, never zero.
func accountForHeals(ras []*remAction, incidents map[string]*incidentMemory, unavailable string) HealAccounting {
	h := HealAccounting{
		Total:           len(ras),
		Excluded:        map[string]int{},
		HealNotCredited: map[string]int{},
	}
	if unavailable != "" {
		for _, ra := range ras {
			if ra.Status.Phase == "Completed" {
				h.Completed++
			}
		}
		h.NoData, h.Reason = true, unavailable
		return h
	}
	for _, name := range AllExclusions {
		h.Excluded[name] = 0
	}
	clean, safe := 0, 0

	for _, ra := range ras {
		if ra.Status.Phase == "Completed" {
			h.Completed++
			if c := condition(ra.Status.Conditions, conditionHealCredited); c != nil && c.Status == "False" {
				h.HealNotCredited[c.Reason]++
			}
		}
		category := classifyOutcome(ra, incidentOf(ra, incidents))
		if category != "" {
			h.Excluded[category]++
			continue
		}
		clean++
		if approvalGatePassed(ra) {
			safe++
		}
	}
	h.Clean, h.SafeHeals = &clean, &safe
	return h
}

// classifyOutcome returns the exclusion category of a RemediationAction, or ""
// for a clean heal.
func classifyOutcome(ra *remAction, incident *incidentMemory) string {
	switch ra.Status.Phase {
	case "Completed":
		return classifyCompleted(ra, incident)
	case "RolledBack":
		return classifyRolledBack(ra)
	case "Failed":
		if c := condition(ra.Status.Conditions, conditionGateRefused); c != nil && c.Status == "True" {
			return ExcludedRefusedAtGate
		}
		return ExcludedFailed
	case "Rejected":
		return ExcludedRejected
	case "Expired":
		return ExcludedExpired
	case "Acknowledged":
		return ExcludedAcknowledged
	default:
		return ExcludedInFlight
	}
}

// classifyRolledBack tells a heal that was rolled back from an approval that never
// became one. An action approved with --no-heal (or whose heal never happened) is
// never patched, then verification fails because the workload still has the
// problem, and the operator rolls the persona back: there was no heal to roll back.
// The evidence is the operator's own record that nothing was left to undo
// (WorkloadDiverged=False/WorkloadRestored) and the absence of the CLI's
// WorkloadPatched record. An action with no such record (an older operator) stays a
// rollback: the report does not claim what it cannot show.
func classifyRolledBack(ra *remAction) string {
	diverged := condition(ra.Status.Conditions, conditionWorkloadDiverged)
	if diverged != nil && diverged.Status == "False" && diverged.Reason == "WorkloadRestored" &&
		condition(ra.Status.Conditions, conditionWorkloadPatched) == nil {
		return ExcludedApprovedNotHealed
	}
	return ExcludedRolledBack
}

// classifyCompleted decides a Completed RemediationAction. The order matters: a
// diverged or partial outcome is named first because it is the most specific
// reason, a self-resolved recovery next (HealCredited=False carries it too, and is
// broken down by reason separately), and only then the credit record itself.
func classifyCompleted(ra *remAction, incident *incidentMemory) string {
	outcome := ""
	if incident != nil && incident.Spec.Resolution != nil {
		outcome = incident.Spec.Resolution.Outcome
	}
	if outcome == incidentOutcomePartial {
		return ExcludedPartial
	}
	if c := condition(ra.Status.Conditions, conditionWorkloadDiverged); c != nil && c.Status == "True" {
		return ExcludedPartial
	}
	if outcome == incidentOutcomeSelfResolve {
		return ExcludedSelfResolved
	}
	credit := condition(ra.Status.Conditions, conditionHealCredited)
	switch {
	case credit == nil:
		return ExcludedNoCreditRecord
	case credit.Status != "True":
		return ExcludedHealNotCredited
	case outcome != incidentOutcomeResolved:
		return ExcludedOutcomeUnknown
	}
	return ""
}

// approvalGatePassed reports whether the gate's approval-time run cleared this
// action: status.gate is an ApprovalRecheck that passed or clamped.
func approvalGatePassed(ra *remAction) bool {
	g := ra.Status.Gate
	if g == nil || g.Phase != "ApprovalRecheck" {
		return false
	}
	return strings.EqualFold(g.Result, "Pass") || strings.EqualFold(g.Result, "Clamp")
}
