import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { APPROVE_LATER_NOTE } from '@/lib/copy'
import type { Remediation } from '@/lib/types'
import { ALL_PASS, gate, remediation, step, ticket, verdict } from '@/test/fixtures'
import { TooltipProvider } from '@/components/ui/tooltip'

import { ReviewPanel } from './review-panel'

const identity = { available: true, username: 'alex@example.com' }

function open(r: Remediation, onClose = () => {}) {
  return render(
    <TooltipProvider>
      <ReviewPanel remediation={r} identity={identity} onClose={onClose} />
    </TooltipProvider>,
  )
}

const helm: Remediation['workload'] = {
  kind: 'Deployment', name: 'checkout-podinfo', namespace: 'apps', container: 'podinfo', managedBy: 'helm',
  managedByDetail: 'Helm release "checkout"', ownershipSource: 'gate', owned: true, ownerName: 'A Helm release',
  whyNotPatched: 'A patch would fight Helm.', observed: true,
}

const changes = [{ path: 'resources.limits.memory', before: '128Mi', after: '256Mi', added: false, changed: true }]

const clamped = remediation({
  name: 'fix-oom-checkout', id: 'apps/fix-oom-checkout', aiPlanned: true, planSource: 'ai-anthropic',
  planSummary: 'Raise the memory limit. <b>now</b>', explanation: 'It was OOMKilled.',
  approveCommand: 'dorgu remediation approve fix-oom-checkout -n apps', workload: helm, workloadChanges: changes,
  gate: gate(
    [
      ...ALL_PASS.slice(0, 3),
      verdict('Guardrails', 'blast-radius', 'clamp', {
        reason: 'clamped', numbers: { field: 'spec.resources.limits.memory', baseline: '128Mi', requested: '4Gi', ratio: '32.00', max: '2.0' },
      }),
      ...ALL_PASS.slice(4),
    ],
    'Clamp',
  ),
  steps: [
    step({
      description: 'Set the memory limit.', rationale: 'Well within a 2x ceiling.',
      patchChanges: changes,
      safety: [{ rule: 'blast-radius', verdict: 'clamped', field: 'spec.resources.limits.memory', ratio: '32.00', maxRatio: '2.0', baseline: '128Mi', requested: '4Gi', permitted: '256Mi', message: 'Clamped.' }],
    }),
  ],
})

describe('the review panel', () => {
  it('lays out the sections in the order the design gives', () => {
    open(clamped)
    const headings = screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent)
    expect(headings.slice(0, 6)).toEqual(['Target', 'Diff', 'Gate', 'Guardrails', 'Plan', 'Ticket'])
  })

  it('shows the target workload with its ownership and the advisory sentence', () => {
    open(clamped)
    expect(screen.getAllByText(/checkout-podinfo \(podinfo\)/).length).toBeGreaterThan(0)
    expect(screen.getByText('Helm')).toBeInTheDocument()
    expect(screen.getAllByText('advisory only: Dorgu will not write this workload').length).toBeGreaterThan(0)
    expect(screen.getByTestId('owned-note')).toHaveTextContent('Dorgu will not patch this Deployment')
  })

  it('shows the diff with both columns labelled', () => {
    open(clamped)
    expect(screen.getAllByText('Running now').length).toBeGreaterThan(0)
    expect(screen.getAllByText('After approval').length).toBeGreaterThan(0)
    expect(screen.getAllByText(/resources.limits.memory: 256Mi/).length).toBeGreaterThan(0)
  })

  it('shows the gate strip and a guardrail meter with its ceiling', () => {
    open(clamped)
    expect(screen.getByTestId('gate-strip')).toBeInTheDocument()
    expect(screen.getByTestId('guardrail-meter')).toHaveTextContent('change +3100% of limit (max +100%)')
    expect(screen.getByText('over the ceiling')).toBeInTheDocument()
    expect(screen.getByText(/ceiling source/)).toBeInTheDocument()
  })

  it('puts every word an AI plan wrote in a machine text block, as plain text', () => {
    const { container } = open(clamped)
    const blocks = screen.getAllByTestId('machine-text')
    expect(blocks.length).toBeGreaterThanOrEqual(3) // summary, explanation, step
    expect(screen.getByText('Raise the memory limit. <b>now</b>')).toBeInTheDocument()
    expect(container.querySelector('b')).toBeNull()
    expect(within(blocks[0] as HTMLElement).getByText(/Plan summary/)).toBeInTheDocument()
    expect(screen.getByText(/Well within a 2x ceiling/)).toBeInTheDocument()
  })

  it('leaves a rule-based plan as plain text, because that is Dorgu speaking', () => {
    open(remediation({ planSummary: 'Raise it.', explanation: 'Because.' }))
    expect(screen.queryByTestId('machine-text')).not.toBeInTheDocument()
    expect(screen.getByText(/Raise it\./)).toBeInTheDocument()
  })

  it('shows the ticket chip', () => {
    // The chip reads the browser clock, so the expiry is relative to the real now.
    open(remediation({ phase: 'Ticketed', ticket: ticket({ expiresAt: new Date(Date.now() + 13 * 60_000).toISOString() }) }))
    expect(screen.getByText(/Ticket issued, expires in/)).toBeInTheDocument()
  })

  it('names who is looking, from the authenticated identity', () => {
    open(clamped)
    expect(screen.getByTestId('viewing-as')).toHaveTextContent('Viewing as alex@example.com')
  })

  it('says when the identity is unavailable', () => {
    render(
      <TooltipProvider>
        <ReviewPanel remediation={clamped} identity={{ available: false, reason: 'older than 1.28' }} onClose={() => {}} />
      </TooltipProvider>,
    )
    expect(screen.getByTestId('viewing-as')).toHaveTextContent('older than 1.28')
  })

  it('offers the exact CLI command with a copy button, and a note that approving arrives later', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    open(clamped)
    expect(screen.getByText('dorgu remediation approve fix-oom-checkout -n apps')).toBeInTheDocument()
    expect(screen.getByTestId('approve-later-note')).toHaveTextContent(APPROVE_LATER_NOTE)

    const copy = screen.getByRole('button', { name: /Copy: dorgu remediation approve/ })
    expect(copy.className).toContain('bg-ink') // the ordinary primary button is solid ink
    await userEvent.click(copy)
    expect(writeText).toHaveBeenCalledWith('dorgu remediation approve fix-oom-checkout -n apps')
  })

  it('has no Approve or Reject button: that action does not exist yet', () => {
    open(clamped)
    expect(screen.queryByRole('button', { name: /^approve/i })).toBeNull()
    expect(screen.queryByRole('button', { name: /^reject/i })).toBeNull()
    expect(screen.queryByText(/^Approve$/)).toBeNull()
    // Nothing on screen carries the accent fill that is reserved for Approve.
    expect(document.querySelectorAll('button.bg-accent')).toHaveLength(0)
  })

  it('offers the diff command once a decision exists and there is nothing left to approve', () => {
    open(remediation({ phase: 'Completed', approveCommand: undefined }))
    expect(screen.getByText('dorgu remediation diff fix-oom -n apps')).toBeInTheDocument()
    expect(screen.getByText('Read this plan in a terminal:')).toBeInTheDocument()
  })

  it('tells the reader there is no workload record rather than guessing an owner', () => {
    open(remediation({ workload: undefined }))
    expect(screen.getByText(/no record of the live workload/)).toBeInTheDocument()
  })

  it('says when no guardrail measured a number', () => {
    open(remediation({ steps: [step()] }))
    expect(screen.getByText(/No guardrail measured a numeric change/)).toBeInTheDocument()
  })

  it('shows a refusal at approval in the gate section', () => {
    open(remediation({
      phase: 'Failed',
      gate: gate([verdict('Guardrails', 'blast-radius', 'refuse', { reason: 'over' })], 'Refuse'),
      gateRefusal: { reason: 'BlastRadius', message: 'Exceeds the limit.' },
    }))
    expect(screen.getByText(/Refused at approval \(BlastRadius\)/)).toBeInTheDocument()
  })

  it('withholds a command it will not offer and says why', () => {
    open(remediation({ steps: [step({ command: 'kubectl logs x', commandWithheld: 'A write on a Helm workload is withheld.' })] }))
    expect(screen.getByText('kubectl logs x')).toBeInTheDocument()
    expect(screen.getByText(/withheld/)).toBeInTheDocument()
  })

  it('lists the owner instructions with read-only commands', () => {
    open(remediation({
      workload: helm,
      ownerInstructions: [{ order: 1, description: 'Edit the chart values.', command: 'kubectl logs deploy/x' }],
    }))
    expect(screen.getByText('Edit the chart values.')).toBeInTheDocument()
    expect(screen.getByText('kubectl logs deploy/x')).toBeInTheDocument()
  })

  it('closes on Escape and on the close button', async () => {
    const onClose = vi.fn()
    open(clamped, onClose)
    await userEvent.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(1)
    await userEvent.click(screen.getByRole('button', { name: 'Close the panel' }))
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('moves focus to its heading when it opens', () => {
    open(clamped)
    expect(screen.getByRole('heading', { level: 2, name: 'fix-oom-checkout' })).toHaveFocus()
  })
})
