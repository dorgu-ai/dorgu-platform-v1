import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { readStages } from '@/lib/gate-stages'
import { ALL_PASS, gate, verdict } from '@/test/fixtures'
import { TooltipProvider } from '@/components/ui/tooltip'

import { GateReasons, GateStrip } from './gate-strip'

const renderStrip = (g: Parameters<typeof GateStrip>[0]['gate']) =>
  render(
    <TooltipProvider delayDuration={0}>
      <GateStrip gate={g} />
    </TooltipProvider>,
  )

const states = () =>
  Array.from(document.querySelectorAll('[data-stage]')).map((el) => [el.getAttribute('data-stage'), el.getAttribute('data-state')])

describe('the gate strip', () => {
  it('draws six pills in the order the operator runs its gate', () => {
    renderStrip(gate(ALL_PASS))
    const pills = within(screen.getByTestId('gate-strip')).getAllByRole('listitem')
    expect(pills.map((p) => p.textContent?.replace(/: .*/, ''))).toEqual([
      'Validate', 'Ground', 'Ownership', 'Guardrails', 'Mode', 'Dedup',
    ])
  })

  it('marks every stage passed when the gate passed them all', () => {
    renderStrip(gate(ALL_PASS))
    expect(states().every(([, s]) => s === 'passed')).toBe(true)
  })

  it('says each state in words, not only in colour', () => {
    renderStrip(gate(ALL_PASS))
    expect(screen.getAllByText(': passed')).toHaveLength(6)
  })

  it('stops at a refusal: the stages after it are not run, even if verdicts follow', () => {
    renderStrip(
      gate(
        [
          verdict('Validate', 'step-type', 'pass'),
          verdict('Ground', 'live-workload', 'pass'),
          verdict('Ownership', 'ownership', 'pass'),
          verdict('Guardrails', 'blast-radius', 'refuse', { reason: 'change exceeds 100% of limit' }),
          // A verdict the operator should never write after a refusal. The strip must not draw it.
          verdict('ModePolicy', 'self-healing-mode', 'pass'),
        ],
        'Refuse',
      ),
    )
    expect(states()).toEqual([
      ['Validate', 'passed'], ['Ground', 'passed'], ['Ownership', 'passed'],
      ['Guardrails', 'refused'], ['ModePolicy', 'not-run'], ['DedupCooldown', 'not-run'],
    ])
  })

  it('shows the refusal reason on hover', async () => {
    renderStrip(gate([verdict('Validate', 'step-command', 'refuse', { reason: 'step 2 is not read-only' })], 'Refuse'))
    await userEvent.hover(document.querySelector('[data-stage="Validate"]') as Element)
    expect((await screen.findAllByText(/step 2 is not read-only/))[0]).toBeInTheDocument()
  })

  it('draws a deferral as deferred and says it is not a refusal', async () => {
    renderStrip(
      gate([
        ...ALL_PASS.slice(0, 5),
        verdict('DedupCooldown', 'active-remediation', 'refuse', { reason: 'a proposal is waiting' }),
      ]),
    )
    const dedup = document.querySelector('[data-stage="DedupCooldown"]') as Element
    expect(dedup.getAttribute('data-state')).toBe('deferred')
    expect(dedup).toHaveTextContent('deferred')
    await userEvent.hover(dedup)
    expect((await screen.findAllByText(/Deferred, not refused/))[0]).toBeInTheDocument()
  })

  it('draws a cooldown refusal as a refusal, because only the active-remediation stand-down is a deferral', () => {
    renderStrip(gate([...ALL_PASS.slice(0, 5), verdict('DedupCooldown', 'rejection-cooldown', 'refuse')], 'Refuse'))
    expect(document.querySelector('[data-stage="DedupCooldown"]')?.getAttribute('data-state')).toBe('refused')
  })

  it('labels a clamp as a pass that narrowed a value', () => {
    renderStrip(gate([verdict('Validate', 'x', 'pass'), verdict('Guardrails', 'blast-radius', 'clamp')], 'Clamp'))
    const guardrails = document.querySelector('[data-stage="Guardrails"]') as Element
    expect(guardrails.getAttribute('data-state')).toBe('passed')
    expect(guardrails).toHaveTextContent('clamped')
  })

  it('renders every pill as not run, and says so, when the operator wrote no gate record', () => {
    renderStrip(undefined)
    expect(states().every(([, s]) => s === 'not-run')).toBe(true)
    expect(screen.getByText(/wrote no gate record/)).toBeInTheDocument()
  })

  it('does not run a stage the gate wrote nothing for', () => {
    renderStrip(gate([verdict('Validate', 'step-type', 'pass')]))
    expect(states()[1]).toEqual(['Ground', 'not-run'])
  })
})

describe('readStages', () => {
  it('keeps each stage its own verdicts', () => {
    const stages = readStages(gate(ALL_PASS))
    expect(stages[3]?.verdicts.map((v) => v.guardrail)).toEqual(['blast-radius'])
  })
})

describe('the reasons under the strip', () => {
  it('says what the gate said for a refusal and a deferral, and names a refusal at approval', () => {
    render(
      <GateReasons
        gate={gate([verdict('Guardrails', 'blast-radius', 'refuse', { reason: 'over the ceiling' })], 'Refuse')}
        refusal={{ reason: 'BlastRadius', message: 'The change exceeds 100% of the live limit.' }}
      />,
    )
    expect(screen.getByText(/the gate refused/)).toBeInTheDocument()
    expect(screen.getByText(/over the ceiling/)).toBeInTheDocument()
    expect(screen.getByText(/Refused at approval \(BlastRadius\)/)).toBeInTheDocument()
    expect(screen.getByText(/exceeds 100% of the live limit/)).toBeInTheDocument()
  })

  it('has a default sentence for a refusal with no message', () => {
    render(<GateReasons gate={undefined} refusal={{ reason: 'NoPlan' }} />)
    expect(screen.getByText(/Nothing was applied/)).toBeInTheDocument()
  })

  it('renders nothing when there is nothing to say', () => {
    const { container } = render(<GateReasons gate={gate(ALL_PASS)} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('says a clamp narrowed a value', () => {
    render(<GateReasons gate={gate([verdict('Guardrails', 'blast-radius', 'clamp', { reason: 'narrowed to 256Mi' })], 'Clamp')} />)
    expect(screen.getByText(/passed with a value narrowed/)).toBeInTheDocument()
  })

  it('says a deferral is not a refusal', () => {
    render(
      <GateReasons
        gate={gate([verdict('DedupCooldown', 'active-remediation', 'refuse', { reason: 'waiting' })], 'Refuse')}
      />,
    )
    expect(screen.getByText(/deferred, which is not a refusal/)).toBeInTheDocument()
  })
})
