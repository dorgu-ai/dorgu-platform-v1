import type { GateVerdict, RemediationGate } from './types'

export const GATE_STAGES = [
  { stage: 'Validate', label: 'Validate' },
  { stage: 'Ground', label: 'Ground' },
  { stage: 'Ownership', label: 'Ownership' },
  { stage: 'Guardrails', label: 'Guardrails' },
  { stage: 'ModePolicy', label: 'Mode' },
  { stage: 'DedupCooldown', label: 'Dedup' },
] as const

export type StageState = 'passed' | 'refused' | 'deferred' | 'not-run'

export interface StageReading {
  stage: string
  label: string
  state: StageState
  /** Clamp is a pass that narrowed a value; the pill says so. */
  clamped: boolean
  reason?: string
  verdicts: GateVerdict[]
}

/**
 * Deferred is the gate standing down because a proposal is already outstanding or
 * cooling down. It is not a refusal and the strip must not paint it as one.
 */
function isDeferral(v: GateVerdict): boolean {
  return v.verdict === 'refuse' && v.stage === 'DedupCooldown' && v.guardrail === 'active-remediation'
}

/**
 * `proposalClamped` is true when a step records that a guardrail clamped a value
 * when the plan was made. The approval gate seals a pass over the already-clamped
 * patch, so its own Guardrails verdict can read as clean; the pill still says the
 * stage narrowed a value, because it did.
 */
export function readStages(gate: RemediationGate | undefined, proposalClamped = false): StageReading[] {
  const verdicts = gate?.verdicts ?? []
  let stopped = false

  return GATE_STAGES.map(({ stage, label }) => {
    const own = verdicts.filter((v) => v.stage === stage)
    const reading: StageReading = { stage, label, state: 'not-run', clamped: false, verdicts: own }
    if (stopped || own.length === 0) return reading

    const refusal = own.find((v) => v.verdict === 'refuse')
    if (refusal) {
      reading.state = isDeferral(refusal) ? 'deferred' : 'refused'
      reading.reason = refusal.reason || refusal.guardrail
      stopped = true
      return reading
    }
    reading.state = 'passed'
    reading.clamped = own.some((v) => v.verdict === 'clamp') || (stage === 'Guardrails' && proposalClamped)
    reading.reason = own.find((v) => v.reason)?.reason
    return reading
  })
}
