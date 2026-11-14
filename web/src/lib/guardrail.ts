import type { GateVerdict, Remediation, StepSafety } from './types'

/**
 * The arithmetic behind a guardrail meter, kept out of the component so it can be
 * tested with plain numbers.
 *
 * The operator records a blast-radius verdict as data: a baseline, a requested
 * value, the ratio between them and the ceiling as a multiplier (`max`). A ratio
 * of 1.5 against a ceiling of 2.0 is "+50% of the limit, max +100%". Decreases
 * have their own ceiling, which the operator fixes at half the baseline, so a
 * ratio under one is measured against 50%.
 */
export interface GuardrailReading {
  field: string
  /** What the guardrail did with the value, when a verdict says. Absent for bare numbers. */
  outcome?: GuardrailOutcome
  baseline?: string
  requested?: string
  permitted?: string
  /** Signed change as a percentage of the baseline: +50 or -30. */
  changePercent: number
  /** The ceiling in the same direction, as a positive percentage: 100 or 50. */
  ceilingPercent: number
  /** changePercent as a share of the ceiling, 0 to 100 or beyond when over it. */
  usedPercent: number
  over: boolean
}

/** What a guardrail did: let a value through, narrowed it to what Dorgu permits, or refused it. */
export type GuardrailOutcome = 'pass' | 'clamp' | 'refuse'

/**
 * The one place a guardrail outcome becomes a status tone, so the gate pill, the
 * meter and the safety badges cannot disagree. A clamp is a warning: the plan
 * asked for more than the ceiling allowed and Dorgu narrowed it. It is not a
 * clean pass, so it is never ok, and it is not a refusal, so it is not danger.
 */
export function outcomeTone(outcome: GuardrailOutcome): 'ok' | 'warn' | 'danger' {
  switch (outcome) {
    case 'refuse':
      return 'danger'
    case 'clamp':
      return 'warn'
    default:
      return 'ok'
  }
}

const OUTCOME_RANK: Record<GuardrailOutcome, number> = { pass: 0, clamp: 1, refuse: 2 }

/** The stronger of two outcomes: a later pass never softens a clamp, nor a clamp a refusal. */
function strongest(a: GuardrailOutcome | undefined, b: GuardrailOutcome | undefined): GuardrailOutcome | undefined {
  if (!a || !b) return a ?? b
  return OUTCOME_RANK[b] > OUTCOME_RANK[a] ? b : a
}

/** A step safety verdict as an outcome. `derived` is a computed value, not a ruling, so it has none. */
export function safetyOutcome(verdict: string): GuardrailOutcome | undefined {
  if (verdict === 'clamped') return 'clamp'
  if (verdict === 'rejected') return 'refuse'
  return undefined
}

/** Decreases are capped at half the baseline: the operator's MaxResourceDecrease. */
const MAX_DECREASE_PERCENT = 50

export function readGuardrail(input: {
  field: string
  ratio?: string
  max?: string
  baseline?: string
  requested?: string
  permitted?: string
  outcome?: GuardrailOutcome
}): GuardrailReading | undefined {
  const ratio = Number.parseFloat(input.ratio ?? '')
  const max = Number.parseFloat(input.max ?? '')
  if (!Number.isFinite(ratio) || ratio <= 0 || !Number.isFinite(max) || max <= 1) return undefined

  const changePercent = Math.round((ratio - 1) * 100)
  const ceilingPercent = ratio >= 1 ? Math.round((max - 1) * 100) : MAX_DECREASE_PERCENT
  const usedPercent = (Math.abs(changePercent) / ceilingPercent) * 100
  return {
    field: input.field,
    outcome: input.outcome,
    baseline: input.baseline,
    requested: input.requested,
    permitted: input.permitted,
    changePercent,
    ceilingPercent,
    usedPercent,
    over: Math.abs(changePercent) > ceilingPercent,
  }
}

/**
 * The meter's tone. A refusal is danger and a clamp is warn, whatever the numbers
 * say: the verdict is what the gate decided. With no verdict (bare numbers) the
 * bar is ink, warn above 80% of the ceiling and danger when over it. Ink is the
 * neutral bar of an ordinary pass; the meter never uses ok, because a bar nearly
 * at its ceiling is not a success to be coloured as one.
 */
export function guardrailTone(reading: GuardrailReading): 'ink' | 'warn' | 'danger' {
  if (reading.outcome === 'refuse') return 'danger'
  if (reading.outcome === 'clamp') return 'warn'
  if (reading.over) return 'danger'
  if (reading.usedPercent > 80) return 'warn'
  return 'ink'
}

export function signed(percent: number): string {
  return `${percent >= 0 ? '+' : '-'}${Math.abs(percent)}%`
}

/** A resource field as a short label: spec.resources.limits.memory is "memory limit". */
export function fieldLabel(field: string): string {
  const parts = field.split('.')
  const resource = parts.at(-1) ?? field
  const kind = parts.at(-2)
  if (kind === 'limits') return `${resource} limit`
  if (kind === 'requests') return `${resource} request`
  return field
}

/** Readings from the gate's verdict numbers: the operator's blast-radius records. */
export function readingsFromVerdicts(verdicts: GateVerdict[] | undefined): GuardrailReading[] {
  const out: GuardrailReading[] = []
  for (const v of verdicts ?? []) {
    if (v.guardrail !== 'blast-radius' || !v.numbers) continue
    const reading = readGuardrail({
      field: v.numbers.field ?? '',
      ratio: v.numbers.ratio,
      max: v.numbers.max,
      baseline: v.numbers.baseline,
      requested: v.numbers.requested,
      outcome: v.verdict,
    })
    if (reading) out.push(reading)
  }
  return out
}

/** Readings from a step's safety entries: the proposal-time verdict, with the permitted value. */
export function readingsFromSafety(entries: StepSafety[] | undefined): GuardrailReading[] {
  const out: GuardrailReading[] = []
  for (const e of entries ?? []) {
    if (e.rule !== 'blast-radius') continue
    const reading = readGuardrail({
      field: e.field,
      ratio: e.ratio,
      max: e.maxRatio,
      baseline: e.baseline,
      requested: e.requested,
      permitted: e.permitted,
      outcome: safetyOutcome(e.verdict),
    })
    if (reading) out.push(reading)
  }
  return out
}

/** What the percentage is of: the limit, the request, or the baseline when the field says neither. */
export function baselineWord(field: string): string {
  if (field.includes('.limits.')) return 'limit'
  if (field.includes('.requests.')) return 'request'
  return 'baseline'
}

/**
 * The gate's readings first, then any step safety reading for a field the gate did
 * not measure.
 *
 * Where both name a field they are one reading, and the stronger outcome wins. The
 * gate records the approval seal as a pass over the already-clamped patch, so its
 * own verdict can be a clean pass for a field the step records as clamped; the
 * clamp is what happened, so it is kept, along with the values the step recorded.
 */
export function collectReadings(remediation: Remediation): GuardrailReading[] {
  const fromGate = [
    ...readingsFromVerdicts(remediation.gate?.verdicts),
    ...readingsFromVerdicts(remediation.ticket?.verdicts),
  ]
  const out = new Map<string, GuardrailReading>()
  for (const reading of fromGate) {
    const seen = out.get(reading.field)
    out.set(reading.field, seen ? { ...seen, outcome: strongest(seen.outcome, reading.outcome) } : reading)
  }
  for (const step of remediation.steps) {
    for (const entry of readingsFromSafety(step.safety)) {
      const seen = out.get(entry.field)
      out.set(
        entry.field,
        seen
          ? {
              ...seen,
              outcome: strongest(seen.outcome, entry.outcome),
              requested: seen.requested ?? entry.requested,
              permitted: seen.permitted ?? entry.permitted,
            }
          : entry,
      )
    }
  }
  return [...out.values()]
}

/** Whether any step records that a guardrail clamped a value when the plan was made. */
export function proposalClamped(remediation: Remediation): boolean {
  return remediation.steps.some((s) => s.safety?.some((e) => safetyOutcome(e.verdict) === 'clamp') ?? false)
}
