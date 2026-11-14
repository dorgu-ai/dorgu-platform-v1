import type {
  GateVerdict,
  Incident,
  Meta,
  Readiness,
  Remediation,
  RemediationGate,
  RemediationTicket,
  Step,
} from '@/lib/types'

/** Builders for the payload shapes the components read. Each takes overrides and nothing is shared between calls. */

export const NOW = Date.parse('2026-11-14T12:00:00Z')
export const iso = (offsetMs: number) => new Date(NOW + offsetMs).toISOString()

export function verdict(stage: string, guardrail: string, kind: GateVerdict['verdict'], extra: Partial<GateVerdict> = {}): GateVerdict {
  return { stage, guardrail, verdict: kind, ...extra }
}

export const ALL_PASS: GateVerdict[] = [
  verdict('Validate', 'step-type', 'pass'),
  verdict('Ground', 'live-workload', 'pass'),
  verdict('Ownership', 'ownership', 'pass'),
  verdict('Guardrails', 'blast-radius', 'pass'),
  verdict('ModePolicy', 'self-healing-mode', 'pass'),
  verdict('DedupCooldown', 'active-remediation', 'pass'),
]

export function gate(verdicts: GateVerdict[], result: RemediationGate['result'] = 'Pass'): RemediationGate {
  return { phase: 'Proposal', result, evaluatedAt: iso(-60_000), verdicts }
}

export function ticket(extra: Partial<RemediationTicket> = {}): RemediationTicket {
  return { state: 'issued', issuedAt: iso(-60_000), expiresAt: iso(12 * 60_000 + 41_000), ...extra }
}

export function step(extra: Partial<Step> = {}): Step {
  return {
    order: 1, id: 'raise-memory', type: 'persona-update', risk: 'low', mode: 'auto', autoExecutable: true,
    description: 'Set spec.resources.limits.memory to 256Mi.', ...extra,
  }
}

export function remediation(extra: Partial<Remediation> = {}): Remediation {
  return {
    id: 'apps/fix-oom', name: 'fix-oom', namespace: 'apps', phase: 'Pending', planSource: 'rule-based', aiPlanned: false,
    confidence: '0.85', trustLevel: 2, incident: { name: 'oom', namespace: 'apps', exists: true },
    persona: { name: 'checkout', namespace: 'apps', exists: true }, steps: [step()], appliable: true, advisoryOnly: false,
    guardrailCount: 0, diffCommand: 'dorgu remediation diff fix-oom -n apps', approvalRequired: true,
    createdAt: iso(-3_600_000), ...extra,
  }
}

export function incident(extra: Partial<Incident> = {}): Incident {
  return {
    id: 'apps/oom', name: 'oom', namespace: 'apps', severity: 'critical', category: 'resource', phase: 'Detected',
    attribution: 'persona', signal: 'OOMKilled', source: 'pod-failure-detector',
    persona: { kind: 'ApplicationPersona', name: 'checkout', namespace: 'apps', exists: true },
    lineage: { key: 'oom', generation: 0, flapping: false }, signals: {}, firstSeen: iso(-3_600_000), lastSeen: iso(-60_000),
    occurrenceCount: 1, createdAt: iso(-3_600_000), ...extra,
  }
}

export const readyReadiness: Readiness = { synced: true, crdsInstalled: true }

export function meta(extra: Partial<Meta> = {}): Meta {
  return {
    version: '0.1.0', cluster: { context: 'dorgu-dev', server: 'https://example', inCluster: false },
    identity: { available: true, username: 'alex@example.com', groups: ['platform-team'] },
    crds: {}, operatorInstalled: true, synced: true, streamClients: 1,
    operator: { found: true, desired: 1, ready: 1, version: 'v0.13.1' },
    views: [
      { id: 'overview', label: 'Overview', available: true },
      { id: 'incidents', label: 'Incidents', available: true },
      { id: 'remediations', label: 'Proposals', available: true, limitation: 'Read-only.' },
      { id: 'apps', label: 'Apps', available: true },
      { id: 'memory', label: 'Memory', available: true },
      { id: 'cluster', label: 'Cluster', available: true },
      { id: 'agents', label: 'Agents', available: false, reason: 'Connected agents arrive with the agent interface.' },
    ],
    ...extra,
  }
}

import type { OverviewPayload, ReportDistribution, ReportDoc } from '@/lib/types'

const noDist: ReportDistribution = { n: 0, p50_seconds: null, p90_seconds: null, no_data: true }

/** A report with measured values, shaped as `dorgu report --json` prints it. Override any part. */
export function reportDoc(extra: Partial<ReportDoc> = {}): ReportDoc {
  return {
    schema: 'dorgu.report/v1',
    generated_at: iso(0),
    window: { since: '7d', start: iso(-7 * 86_400_000), end: iso(0) },
    cluster: { context: 'dorgu-dev', namespace: 'all', operator_version: 'v0.13.1', application_personas: 3, self_healing_modes: ['propose'] },
    sources: {
      remediation_actions: { ok: true, count: 9 }, incidents: { ok: true, count: 8 }, personas: { ok: true, count: 3 },
      cluster_personas: { ok: true, count: 1 }, events: { ok: true, count: 2 },
    },
    input_errors: [],
    heal_accounting: {
      total: 9, completed: 1, clean: 1, safe_heals: 1, no_data: false,
      excluded: { partial: 0, failed: 1, rejected: 2, in_flight: 3, refused_by_gate: 1, expired: 1 },
      heal_not_credited_by_reason: {},
    },
    tier_a: {
      ns_safe_heals: { safe_heals: 1, clean_heals: 1, no_data: false },
      a2_active_cluster: { active: true, proposals_decided: 5 },
      a5_review_depth: { proposals_decided: 5, window_days: 7, per_week: 5, no_data: false },
      a6_approval_rate: { approved: 3, rejected: 2, rate: 0.6, no_data: false, too_few_decisions: false, band: 'in' },
      a7_external_agent_share: { no_data: true, reason: 'not measured yet' },
      decisions: { approved: 3, rejected: 2 },
      notes: [],
    },
    tier_q: {
      q1_time_to_heal: {
        detect_to_proposal: { n: 7, p50_seconds: 90, p90_seconds: 12_600, no_data: false },
        proposal_to_decision: noDist, decision_to_applied: noDist, applied_to_verified: noDist,
      },
      q2_proposal_precision: { decided: 5, wrong: 1, rate: 0.2, no_data: false, too_few_decisions: false, rejections_by_reason: { 'wrong-diagnosis': 1 } },
      q3_llm_efficiency: { no_data: true, reason: 'operator metrics', plan_sources: { 'rule-based': 7 } },
      q6_refusal_mix: {
        proposals: 11, no_data: false,
        rows: [{ phase: 'approval', stage: 'Guardrails', reason: 'BlastRadius', count: 2, share_of_proposals: 2 / 11, flagged: false }],
        deferred_proposals: null, deferred_note: 'not recorded on cluster objects (see the dorgu_gate_decisions_total metric, result=deferred)',
      },
    },
    first_run_clamps: { proposals_with_clamp: 1, step_verdicts: {}, clamps_by_rule: {}, gate_clamps_by_guardrail: {}, no_data: false },
    cli_declines: { no_data: false, by_reason: { DorguCLINoTicket: 3 }, note: 'Events expire.' },
    not_measurable: [{ id: 'A1', name: 'External installs', reason: 'read from partner reports' }],
    ...extra,
  }
}

export function overview(extra: Partial<OverviewPayload> = {}): OverviewPayload {
  return {
    report: reportDoc(), window: '7d', readiness: { synced: true, crdsInstalled: true },
    operator: { found: true, desired: 1, ready: 1, version: 'v0.13.1' }, ...extra,
  }
}
