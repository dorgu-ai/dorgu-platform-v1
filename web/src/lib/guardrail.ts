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

/** Decreases are capped at half the baseline: the operator's MaxResourceDecrease. */
const MAX_DECREASE_PERCENT = 50

export function readGuardrail(input: {
  field: string
  ratio?: string
  max?: string
  baseline?: string
  requested?: string
  permitted?: string
}): GuardrailReading | undefined {
  const ratio = Number.parseFloat(input.ratio ?? '')
  const max = Number.parseFloat(input.max ?? '')
  if (!Number.isFinite(ratio) || ratio <= 0 || !Number.isFinite(max) || max <= 1) return undefined

  const changePercent = Math.round((ratio - 1) * 100)
  const ceilingPercent = ratio >= 1 ? Math.round((max - 1) * 100) : MAX_DECREASE_PERCENT
  const usedPercent = (Math.abs(changePercent) / ceilingPercent) * 100
  return {
    field: input.field,
    baseline: input.baseline,
    requested: input.requested,
    permitted: input.permitted,
    changePercent,
    ceilingPercent,
    usedPercent,
    over: Math.abs(changePercent) > ceilingPercent,
  }
}

/** The bar is ink, warn above 80% of the ceiling and danger when over it. */
export function guardrailTone(reading: GuardrailReading): 'ink' | 'warn' | 'danger' {
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

/** The gate's readings first, then any step safety reading for a field the gate did not measure. */
export function collectReadings(remediation: Remediation): GuardrailReading[] {
  const fromGate = [
    ...readingsFromVerdicts(remediation.gate?.verdicts),
    ...readingsFromVerdicts(remediation.ticket?.verdicts),
  ]
  const seen = new Set(fromGate.map((r) => r.field))
  const out = [...fromGate]
  for (const step of remediation.steps) {
    for (const reading of readingsFromSafety(step.safety)) {
      if (seen.has(reading.field)) continue
      seen.add(reading.field)
      out.push(reading)
    }
  }
  return out
}

