import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import type { IncidentsPayload, RemediationsPayload } from '@/lib/types'
import { incident, meta, overview, reportDoc, remediation, readyReadiness } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'

import { OverviewView } from './overview'

const incidents: IncidentsPayload = {
  incidents: [incident(), incident({ id: 'apps/done', name: 'done', phase: 'Resolved' })],
  summary: { total: 2, open: 1, critical: 1, warning: 0, info: 0, unattributed: 0, diagnosed: 0 },
  readiness: readyReadiness,
}
const proposals: RemediationsPayload = {
  remediations: [
    remediation(),
    remediation({ id: 'apps/old', name: 'old', phase: 'Expired', expired: { reason: 'ApprovalTimedOut' } }),
    remediation({ id: 'apps/done', name: 'done', phase: 'Completed' }),
  ],
  summary: { total: 3, pending: 1, appliable: 3, advisory: 0, guarded: 0, owned: 0, aiPlanned: 0, completed: 1, failed: 0 },
  readiness: readyReadiness,
}

function renderOverview(payload = overview(), window = '7d') {
  return renderWithApp(<OverviewView window={window} />, {
    seed: (qc) => {
      qc.setQueryData(['meta'], meta())
      qc.setQueryData(['overview'], payload)
      qc.setQueryData(['overview-window', '24h'], payload)
      qc.setQueryData(['incidents'], incidents)
      qc.setQueryData(['remediations'], proposals)
    },
  })
}

const tile = async (label: string) => {
  const heading = await screen.findByRole('heading', { level: 3, name: label })
  return within(heading.closest('[data-testid="kpi-tile"]') as HTMLElement)
}

describe('the Overview', () => {
  it('shows every key figure with its provenance and window', async () => {
    renderOverview()
    const safe = await tile('Safe heals')
    expect(safe.getByText('1')).toBeInTheDocument()
    expect(safe.getByText(/from RemediationActions, IncidentMemories and the gate record, the last 7 days/)).toBeInTheDocument()

    expect(screen.getAllByTestId('kpi-tile')).toHaveLength(8)
    for (const t of screen.getAllByTestId('kpi-tile')) expect(t).toHaveTextContent(/from .*, the last 7 days/)
  })

  it('shows the approval rate with its band, as dorgu report words it', async () => {
    renderOverview()
    const approval = await tile('Approval rate')
    expect(approval.getByText('60.0%')).toBeInTheDocument()
    expect(approval.getByText(/3 approved, 2 rejected; inside the 30-80% band/)).toBeInTheDocument()
  })

  it('formats time to heal as the CLI does', async () => {
    renderOverview()
    const t = await tile('Detect to proposal')
    expect(t.getByText('1m30s')).toBeInTheDocument()
    expect(t.getByText(/p50 of 7; p90 3h30m/)).toBeInTheDocument()
  })

  it('says too few decisions rather than judging on a handful', async () => {
    const doc = reportDoc()
    doc.tier_a.a6_approval_rate = { approved: 2, rejected: 1, rate: null, no_data: false, too_few_decisions: true }
    doc.tier_q.q2_proposal_precision = { decided: 3, wrong: 0, rate: null, no_data: false, too_few_decisions: true, rejections_by_reason: {} }
    renderOverview(overview({ report: doc }))
    const approval = await tile('Approval rate')
    expect(approval.getByText('2 of 3')).toBeInTheDocument()
    expect(approval.getByText(/too few decisions \(n=3\) to judge/)).toBeInTheDocument()
    const wrong = await tile('Called wrong')
    expect(wrong.getByText(/too few decisions \(n=3\) to judge/)).toBeInTheDocument()
    expect(wrong.queryByText('0.0%')).toBeNull()
  })

  it('shows no data, never zero, for a figure with no sample', async () => {
    const doc = reportDoc({
      heal_accounting: { total: 4, completed: 2, clean: null, safe_heals: null, no_data: true, reason: 'x', excluded: {}, heal_not_credited_by_reason: {} },
    })
    doc.tier_a.ns_safe_heals = { safe_heals: null, clean_heals: null, no_data: true, reason: 'the IncidentMemories could not be read' }
    doc.tier_a.a6_approval_rate = { approved: 0, rejected: 0, rate: null, no_data: true, too_few_decisions: false }
    doc.tier_a.a5_review_depth = { proposals_decided: 0, window_days: 7, per_week: null, no_data: true }
    doc.tier_q.q2_proposal_precision = { decided: 0, wrong: 0, rate: null, no_data: true, too_few_decisions: false, rejections_by_reason: {} }
    doc.tier_q.q1_time_to_heal.detect_to_proposal = noData('the incidents could not be read')
    doc.tier_q.q6_refusal_mix = { proposals: 0, no_data: true, reason: 'no proposals', rows: [], deferred_proposals: null, deferred_note: 'x' }
    renderOverview(overview({ report: doc }))

    const safe = await tile('Safe heals')
    expect(safe.getByText('no data')).toBeInTheDocument()
    expect(safe.getByText('the IncidentMemories could not be read')).toBeInTheDocument()
    for (const label of ['Clean heals', 'Approval rate', 'Called wrong', 'Detect to proposal', 'Decisions per week', 'Refused by the gate']) {
      const t = await tile(label)
      expect(t.getByText('no data'), label).toBeInTheDocument()
      expect(screen.getByRole('heading', { level: 3, name: label }).closest('[data-testid="kpi-tile"]')).toHaveAttribute('data-no-data', 'true')
    }
    const text = screen.getAllByTestId('kpi-tile').map((t) => t.textContent).join(' ')
    expect(text).not.toMatch(/(^|\s)0\.0%/)
  })

  it('shows not recorded, as dorgu report does, when the Events could not be read', async () => {
    const doc = reportDoc({
      cli_declines: { no_data: true, reason: 'the Events could not be read', by_reason: {}, note: 'Events expire.' },
      input_errors: [{ input: 'events', error: 'timeout' }],
    })
    renderOverview(overview({ report: doc }))
    const declines = await tile('Declined by the CLI')
    expect(declines.getByText('not recorded')).toBeInTheDocument()
    expect(declines.getByText('the Events could not be read')).toBeInTheDocument()

    const alert = screen.getByTestId('input-errors')
    expect(alert).toHaveTextContent('Could not read events (timeout)')
    expect(alert).toHaveTextContent('the sections that need it say no data, never zero')
  })

  it('never shows a zero for deferrals: they are not recorded on cluster objects', async () => {
    renderOverview()
    const note = await screen.findByTestId('deferred-note')
    expect(note).toHaveTextContent('Deferrals are not refusals.')
    expect(note).toHaveTextContent('not recorded on cluster objects')
    expect(note).not.toHaveTextContent('Deferred proposals: 0')
  })

  it('counts the deferrals an older operator wrote, and says they are a floor', async () => {
    const doc = reportDoc()
    doc.tier_q.q6_refusal_mix.deferred_proposals = 2
    renderOverview(overview({ report: doc }))
    expect(await screen.findByTestId('deferred-note')).toHaveTextContent('2 incident(s) with a proposal waiting')
  })

  it('shows no data, not 0, for the heal figures when nothing was proposed in the window', async () => {
    const doc = reportDoc({
      heal_accounting: { total: 0, completed: 0, clean: 0, safe_heals: 0, no_data: false, excluded: {}, heal_not_credited_by_reason: {} },
    })
    doc.tier_a.ns_safe_heals = { safe_heals: 0, clean_heals: 0, no_data: false }
    renderOverview(overview({ report: doc }))
    for (const label of ['Safe heals', 'Clean heals']) {
      const t = await tile(label)
      expect(t.getByText('no data'), label).toBeInTheDocument()
      expect(t.getByText('no proposals were created in the window')).toBeInTheDocument()
    }
  })

  it('lists where proposals ended, with only the outcomes that happened', async () => {
    renderOverview()
    const table = await screen.findByRole('table', { name: /clean heal or excluded/ })
    expect(within(table).getByText('clean heal')).toBeInTheDocument()
    expect(within(table).getByText('in flight')).toBeInTheDocument()
    expect(within(table).queryByText('partial')).toBeNull()
  })

  it('says why the heal accounting is empty when it cannot be computed', async () => {
    const doc = reportDoc({
      heal_accounting: { total: 4, completed: 2, clean: null, safe_heals: null, no_data: true, reason: 'the operator (v0.12.0) predates v0.13.0', excluded: {}, heal_not_credited_by_reason: {} },
    })
    renderOverview(overview({ report: doc }))
    expect((await screen.findAllByText(/predates v0.13.0/)).length).toBeGreaterThan(0)
  })

  it('shows the refusal mix and flags a reason over 30%', async () => {
    const doc = reportDoc()
    doc.tier_q.q6_refusal_mix.rows = [{ phase: 'proposal', stage: 'ModePolicy', reason: 'SelfHealingMode', count: 5, share_of_proposals: 0.5, flagged: true }]
    renderOverview(overview({ report: doc }))
    expect(await screen.findByText('ModePolicy/SelfHealingMode')).toBeInTheDocument()
    expect(screen.getByText('over 30% of proposals')).toBeInTheDocument()
    expect(screen.getByText('50.0%')).toBeInTheDocument()
  })

  it('lists open incidents only, and pending and expired proposals only', async () => {
    renderOverview()
    const open = await screen.findByRole('list', { name: 'Open incidents' })
    expect(within(open).getAllByRole('listitem')).toHaveLength(1)
    expect(within(open).getByText('oom')).toBeInTheDocument()

    const waiting = screen.getByRole('list', { name: 'Pending and expired proposals' })
    const items = within(waiting).getAllByRole('listitem')
    expect(items).toHaveLength(2)
    expect(waiting).toHaveTextContent('expired: ApprovalTimedOut')
    expect(waiting).not.toHaveTextContent('done')
  })

  it('says a quiet window is quiet, and still shows the figures as no data', async () => {
    const doc = reportDoc({
      heal_accounting: { total: 0, completed: 0, clean: 0, safe_heals: 0, no_data: false, excluded: {}, heal_not_credited_by_reason: {} },
    })
    renderOverview(overview({ report: doc }))
    expect(await screen.findByText('Nothing has been proposed in this window')).toBeInTheDocument()
    expect(screen.getByText(/no data: no RemediationActions were created in the window/)).toBeInTheDocument()
  })

  it('offers the three windows, marks the current one and links the others', async () => {
    renderOverview()
    const nav = await screen.findByRole('navigation', { name: 'Report window' })
    expect(within(nav).getByRole('link', { name: '7d' })).toHaveAttribute('aria-current', 'page')
    expect(within(nav).getByRole('link', { name: '24h' })).not.toHaveAttribute('aria-current')
    expect(within(nav).getByRole('link', { name: '24h' }).getAttribute('href')).toContain('since=24h')
    expect(within(nav).getByRole('link', { name: '30d' }).getAttribute('href')).toContain('since=30d')
    expect(within(nav).getByRole('link', { name: '7d' }).getAttribute('href')).not.toContain('since')
  })

  it('shows another window from its own query', async () => {
    renderOverview(overview({ window: '1d' }), '24h')
    expect(await screen.findByText(/the last day/)).toBeInTheDocument()
  })

  it('shows what the figures were read from and what is not measurable', async () => {
    renderOverview()
    const footer = await screen.findByRole('region', { name: 'Where these figures came from' })
    expect(footer).toHaveTextContent('RemediationActions 9')
    expect(footer).toHaveTextContent('operator v0.13.1')
    await userEvent.click(within(footer).getByText('Not measurable from a cluster'))
    expect(within(footer).getByText(/External installs/)).toBeInTheDocument()
  })

  it('shows the warming, not-installed and refused states instead of a report', async () => {
    const { unmount } = renderOverview(overview({ report: null, readiness: { synced: false, crdsInstalled: true } }))
    expect(await screen.findByText(/Reading your cluster/)).toBeInTheDocument()
    unmount()

    const second = renderOverview(overview({ report: null, readiness: { synced: true, crdsInstalled: false, missingCRDs: ['remediationactions'] } }))
    expect(await screen.findByText('The Dorgu operator is not installed on this cluster')).toBeInTheDocument()
    second.unmount()

    renderOverview(overview({ report: null, readiness: { synced: false, crdsInstalled: true, unavailable: [{ resource: 'remediationactions', reason: 'forbidden' }] } }))
    expect(await screen.findByText('Dorgu could not read remediationactions')).toBeInTheDocument()
    expect(screen.queryAllByTestId('kpi-tile')).toHaveLength(0)
  })

  it('shows skeletons while loading, not zeros', async () => {
    renderWithApp(<OverviewView />, { seed: (qc) => qc.setQueryData(['meta'], meta()) })
    expect(await screen.findByTestId('overview-loading')).toBeInTheDocument()
    expect(screen.queryAllByTestId('kpi-tile')).toHaveLength(0)
  })

  it('shows the error state with what to do when the read fails', async () => {
    const { queryClient } = renderWithApp(<OverviewView window="24h" />, { seed: (qc) => qc.setQueryData(['meta'], meta()) })
    await waitFor(() => expect(queryClient.getQueryState(['overview-window', '24h'])).toBeDefined())
    // The fetch fails in jsdom (no server): the state is an alert, not a blank page.
    expect(await screen.findByRole('alert', {}, { timeout: 3000 })).toHaveTextContent('Could not read the Overview')
  })
})

function noData(reason: string) {
  return { n: 0, p50_seconds: null, p90_seconds: null, no_data: true, reason }
}
