import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { incident } from '@/test/fixtures'

import { LineageLine, ProposalCell } from './incident-parts'

describe('the proposal cell', () => {
  it('says none when the gate recorded nothing', () => {
    render(<ProposalCell incident={incident()} />)
    expect(screen.getByText('none')).toBeInTheDocument()
  })

  it('shows a refusal with its stage and guardrail', () => {
    render(<ProposalCell incident={incident({ proposal: { state: 'refused', reason: 'Validate_StepCommand', stage: 'Validate', guardrail: 'StepCommand', since: '' } })} />)
    expect(screen.getByText('refused')).toBeInTheDocument()
    expect(screen.getByText('Validate: StepCommand')).toBeInTheDocument()
  })

  it('never calls a deferral a refusal', () => {
    render(<ProposalCell incident={incident({ proposal: { state: 'deferred', reason: 'DedupCooldown_ActiveRemediation', since: '' } })} />)
    expect(screen.getByText('deferred, not refused')).toBeInTheDocument()
    expect(screen.queryByText('refused')).toBeNull()
  })

  it('shows the durable refusal record as its own line', () => {
    render(<ProposalCell incident={incident({ refusalRecord: { reason: 'Validate_StepCommand', at: '2026-11-14T10:00:00Z' } })} />)
    expect(screen.getByText('refusal recorded: Validate_StepCommand')).toBeInTheDocument()
  })
})

describe('the lineage line', () => {
  it('says nothing for a first, simple incident', () => {
    const { container } = render(<LineageLine incident={incident()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('names the recurrence, the family rank and the escalation', () => {
    render(
      <LineageLine
        incident={incident({
          lineage: { key: 'x', generation: 2, flapping: true, recurrenceOf: 'x' },
          signals: { rank: 4, family: ['a', 'b', 'c', 'd', 'e'], escalatedFrom: ['ReadinessNotReady', 'RestartLoop'] },
        })}
      />,
    )
    const line = screen.getByText(/recurrence r2/)
    expect(line).toHaveTextContent('flapping')
    expect(line).toHaveTextContent('4 of 5 in family')
    expect(line).toHaveTextContent('from ReadinessNotReady > RestartLoop')
  })
})
