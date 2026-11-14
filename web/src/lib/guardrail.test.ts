import { describe, expect, it } from 'vitest'

import { remediation, step, verdict } from '@/test/fixtures'

import {
  baselineWord, collectReadings, fieldLabel, guardrailTone, outcomeTone, readGuardrail, readingsFromSafety, readingsFromVerdicts, safetyOutcome, signed,
} from './guardrail'

const read = (ratio: string, max = '2.0') => readGuardrail({ field: 'spec.resources.limits.memory', ratio, max })!

describe('guardrail arithmetic', () => {
  it('reads +50% against a ceiling of +100%', () => {
    const r = read('1.50')
    expect(r.changePercent).toBe(50)
    expect(r.ceilingPercent).toBe(100)
    expect(r.usedPercent).toBe(50)
    expect(r.over).toBe(false)
  })

  it('measures a decrease against the 50% floor the operator fixes', () => {
    const r = read('0.70')
    expect(r.changePercent).toBe(-30)
    expect(r.ceilingPercent).toBe(50)
    expect(r.usedPercent).toBeCloseTo(60)
  })

  it('is over the ceiling past it, in either direction', () => {
    expect(read('32.00').over).toBe(true)
    expect(read('0.20').over).toBe(true)
    expect(read('2.00').over).toBe(false)
  })

  it('returns nothing for numbers it cannot read, rather than a bar for a guess', () => {
    expect(readGuardrail({ field: 'f', ratio: 'x', max: '2' })).toBeUndefined()
    expect(readGuardrail({ field: 'f', ratio: '1.5' })).toBeUndefined()
    expect(readGuardrail({ field: 'f', ratio: '0', max: '2' })).toBeUndefined()
    expect(readGuardrail({ field: 'f', ratio: '1.5', max: '1' })).toBeUndefined()
  })

  it('is ink up to 80% of the ceiling, warn above it and danger over it', () => {
    expect(guardrailTone(read('1.50'))).toBe('ink')
    expect(guardrailTone(read('1.80'))).toBe('ink')
    expect(guardrailTone(read('1.81'))).toBe('warn')
    expect(guardrailTone(read('2.00'))).toBe('warn')
    expect(guardrailTone(read('2.01'))).toBe('danger')
  })

  it('formats signs and labels', () => {
    expect(signed(50)).toBe('+50%')
    expect(signed(-30)).toBe('-30%')
    expect(signed(0)).toBe('+0%')
    expect(fieldLabel('spec.resources.limits.memory')).toBe('memory limit')
    expect(fieldLabel('spec.resources.requests.cpu')).toBe('cpu request')
    expect(fieldLabel('spec.replicas')).toBe('spec.replicas')
    expect(baselineWord('spec.resources.limits.memory')).toBe('limit')
    expect(baselineWord('spec.resources.requests.cpu')).toBe('request')
    expect(baselineWord('spec.replicas')).toBe('baseline')
  })
})

describe('where the readings come from', () => {
  const numbers = { field: 'spec.resources.limits.memory', baseline: '128Mi', requested: '4Gi', ratio: '32.00', max: '2.0' }

  it('takes only blast-radius verdicts that carry numbers', () => {
    const readings = readingsFromVerdicts([
      verdict('Guardrails', 'blast-radius', 'clamp', { numbers }),
      verdict('Guardrails', 'blast-radius', 'pass'),
      verdict('Validate', 'step-type', 'pass', { numbers }),
      verdict('Guardrails', 'blast-radius', 'pass', { numbers: { field: 'x', ratio: 'nan', max: '2' } }),
    ])
    expect(readings).toHaveLength(1)
    expect(readings[0]?.requested).toBe('4Gi')
    expect(readingsFromVerdicts(undefined)).toEqual([])
  })

  it('takes a step safety entry with the permitted value', () => {
    const readings = readingsFromSafety([
      { rule: 'blast-radius', verdict: 'clamped', field: 'spec.resources.limits.memory', ratio: '32.00', maxRatio: '2.0', permitted: '256Mi', message: 'm' },
      { rule: 'absent-field', verdict: 'rejected', field: 'spec.resources.limits.cpu', message: 'm' },
    ])
    expect(readings).toHaveLength(1)
    expect(readings[0]?.permitted).toBe('256Mi')
    expect(readingsFromSafety(undefined)).toEqual([])
  })

  it('prefers the gate reading and adds a step reading only for a field the gate did not measure', () => {
    const r = remediation({
      gate: { phase: 'Proposal', result: 'Clamp', evaluatedAt: '', verdicts: [verdict('Guardrails', 'blast-radius', 'clamp', { numbers })] },
      steps: [
        step({
          safety: [
            { rule: 'blast-radius', verdict: 'clamped', field: 'spec.resources.limits.memory', ratio: '9.00', maxRatio: '2.0', message: '' },
            { rule: 'blast-radius', verdict: 'clamped', field: 'spec.resources.limits.cpu', ratio: '3.00', maxRatio: '2.0', message: '' },
            { rule: 'blast-radius', verdict: 'clamped', field: 'spec.resources.limits.cpu', ratio: '3.00', maxRatio: '2.0', message: '' },
          ],
        }),
      ],
    })
    const fields = collectReadings(r).map((x) => [x.field, x.changePercent])
    expect(fields).toEqual([['spec.resources.limits.memory', 3100], ['spec.resources.limits.cpu', 200]])
  })
})

describe('the tone of a guardrail outcome', () => {
  const clampedInput = { field: 'spec.resources.limits.memory', ratio: '32.00', max: '2.0' }

  it('maps pass to ok, clamp to warn and refuse to danger', () => {
    expect(outcomeTone('pass')).toBe('ok')
    expect(outcomeTone('clamp')).toBe('warn')
    expect(outcomeTone('refuse')).toBe('danger')
  })

  it('reads a step safety verdict as an outcome, and leaves a derived value alone', () => {
    expect(safetyOutcome('clamped')).toBe('clamp')
    expect(safetyOutcome('rejected')).toBe('refuse')
    expect(safetyOutcome('derived')).toBeUndefined()
    expect(safetyOutcome('quarantined')).toBeUndefined()
  })

  it('renders a clamp that asked for more than the ceiling in warn, not danger and not ink', () => {
    const r = readGuardrail({ ...clampedInput, outcome: 'clamp' })!
    expect(r.over).toBe(true)
    expect(guardrailTone(r)).toBe('warn')
  })

  it('keeps a clamp in warn even when the number it was clamped to sits well under the ceiling', () => {
    expect(guardrailTone(readGuardrail({ field: 'f', ratio: '1.10', max: '2.0', outcome: 'clamp' })!)).toBe('warn')
  })

  it('keeps a refusal danger, whatever the numbers say', () => {
    expect(guardrailTone(readGuardrail({ ...clampedInput, outcome: 'refuse' })!)).toBe('danger')
    expect(guardrailTone(readGuardrail({ field: 'f', ratio: '1.10', max: '2.0', outcome: 'refuse' })!)).toBe('danger')
  })

  it('leaves an unclamped pass to the arithmetic: ink under 80% of the ceiling, never ok', () => {
    expect(guardrailTone(readGuardrail({ field: 'f', ratio: '1.50', max: '2.0', outcome: 'pass' })!)).toBe('ink')
    expect(guardrailTone(readGuardrail({ field: 'f', ratio: '1.90', max: '2.0', outcome: 'pass' })!)).toBe('warn')
  })

  it('takes the outcome from the gate verdict', () => {
    const numbers = { field: 'spec.resources.limits.memory', ratio: '32.00', max: '2.0' }
    const [clamp] = readingsFromVerdicts([verdict('Guardrails', 'blast-radius', 'clamp', { numbers })])
    const [refuse] = readingsFromVerdicts([verdict('Guardrails', 'blast-radius', 'refuse', { numbers })])
    expect(clamp?.outcome).toBe('clamp')
    expect(refuse?.outcome).toBe('refuse')
    expect(guardrailTone(clamp!)).toBe('warn')
    expect(guardrailTone(refuse!)).toBe('danger')
  })

  it('takes the outcome from a step safety entry', () => {
    const entry = { rule: 'blast-radius', field: 'spec.resources.limits.memory', ratio: '32.00', maxRatio: '2.0', message: 'm' }
    expect(readingsFromSafety([{ ...entry, verdict: 'clamped' }])[0]?.outcome).toBe('clamp')
    expect(readingsFromSafety([{ ...entry, verdict: 'rejected' }])[0]?.outcome).toBe('refuse')
  })

  it('agrees with the step safety block when the gate sealed a clean pass after an earlier clamp', () => {
    // status.gate records the seal: the patch already holds the permitted value, so the gate's own
    // numbers are inside the ceiling and its verdict is a pass. The step still records the clamp.
    const field = 'spec.resources.limits.memory'
    const r = remediation({
      gate: {
        phase: 'ApprovalRecheck', result: 'Pass', evaluatedAt: '2026-11-14T12:00:00Z',
        verdicts: [verdict('Guardrails', 'blast-radius', 'pass', { numbers: { field, baseline: '128Mi', requested: '256Mi', ratio: '2.00', max: '2.0' } })],
      },
      steps: [step({ safety: [{ rule: 'blast-radius', verdict: 'clamped', field, baseline: '128Mi', requested: '4Gi', permitted: '256Mi', ratio: '32.00', maxRatio: '2.0', message: 'm' }] })],
    })
    const [reading, ...rest] = collectReadings(r)
    expect(rest).toHaveLength(0)
    expect(reading?.outcome).toBe('clamp')
    expect(reading?.permitted).toBe('256Mi')
    expect(guardrailTone(reading!)).toBe('warn')
  })

  it('does not let a later pass soften a refusal', () => {
    const field = 'spec.resources.limits.memory'
    const numbers = { field, ratio: '32.00', max: '2.0' }
    const r = remediation({
      gate: { phase: 'Proposal', result: 'Refuse', evaluatedAt: '2026-11-14T12:00:00Z', verdicts: [verdict('Guardrails', 'blast-radius', 'refuse', { numbers })] },
      steps: [step({ safety: [{ rule: 'blast-radius', verdict: 'clamped', field, ratio: '32.00', maxRatio: '2.0', message: 'm' }] })],
    })
    expect(collectReadings(r)[0]?.outcome).toBe('refuse')
  })
})
