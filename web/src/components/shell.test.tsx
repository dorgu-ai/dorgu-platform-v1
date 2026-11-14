import type { QueryClient } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { formatVersion, NAV } from '@/lib/nav'
import type { StreamStatus } from '@/lib/stream'
import { meta } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'

import { PageHeader, SummaryBar } from './app-shell'
import { Sidebar } from './sidebar'
import { LiveIndicator, TopBar } from './topbar'

const seeded = (m = meta()) => ({ seed: (qc: QueryClient) => qc.setQueryData(['meta'], m) })

describe('the sidebar', () => {
  it('lists the views in the order the design gives', async () => {
    renderWithApp(<Sidebar />, seeded())
    const nav = await screen.findByRole('navigation', { name: 'Views' })
    const labels = Array.from(nav.querySelectorAll('[data-nav]')).map((el) => el.getAttribute('data-nav'))
    expect(labels).toEqual(['overview', 'incidents', 'remediations', 'apps', 'memory', 'cluster', 'agents'])
    expect(NAV.map((n) => n.id)).toEqual(labels)
  })

  it('keeps the Proposals route stable at /remediations', async () => {
    renderWithApp(<Sidebar />, seeded())
    const link = await screen.findByRole('link', { name: 'Proposals' })
    expect(link.getAttribute('href')).toBe('/remediations')
  })

  it('disables a view this release does not have, and says why in words', async () => {
    renderWithApp(<Sidebar />, seeded())
    const agents = (await screen.findAllByText(/Connected agents arrive/))[0] as HTMLElement
    const item = agents.closest('[data-nav="agents"]') as HTMLElement
    expect(item).toHaveAttribute('aria-disabled', 'true')
    expect(within(item).queryByRole('link')).toBeNull()
    expect(item).toHaveTextContent('Disabled.')
  })

  it('disables whatever the server says it cannot render', async () => {
    const m = meta()
    m.views = m.views.map((v) => (v.id === 'memory' ? { ...v, available: false, reason: 'Not in this build.' } : v))
    renderWithApp(<Sidebar />, seeded(m))
    expect(await screen.findByText('Not in this build.')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Memory' })).toBeNull()
  })

  it('marks the current view and uses the accent for it', async () => {
    renderWithApp(<Sidebar />, { ...seeded(), path: '/incidents' })
    const link = await screen.findByRole('link', { name: 'Incidents' })
    await waitFor(() => expect(link).toHaveAttribute('aria-current', 'page'))
    expect(link.className).toContain('bg-accent-soft')
  })

  it('collapses to icons and remembers it', async () => {
    renderWithApp(<Sidebar />, seeded())
    await userEvent.click(await screen.findByRole('button', { name: 'Collapse the sidebar' }))
    expect(localStorage.getItem('dorgu-sidebar')).toBe('collapsed')
    expect(screen.getByRole('complementary', { name: 'Primary' }).className).toContain('w-14')
    expect(screen.getByRole('button', { name: 'Expand the sidebar' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Expand the sidebar' }))
    expect(localStorage.getItem('dorgu-sidebar')).toBe('expanded')
  })

  it('shows the version, prefixed only when it is a release', async () => {
    renderWithApp(<Sidebar />, seeded(meta({ version: '0.1.0' })))
    expect(await screen.findByText('v0.1.0')).toBeInTheDocument()
    expect(formatVersion('dev')).toBe('dev')
    expect(formatVersion('1.2.3')).toBe('v1.2.3')
  })

  it('states the trust line', async () => {
    renderWithApp(<Sidebar />, seeded())
    expect(await screen.findByText(/Read-only. Uses your kubeconfig/)).toBeInTheDocument()
  })
})

describe('the top bar', () => {
  const live: StreamStatus = { state: 'live' }

  it('shows the cluster context, the authenticated identity and the live indicator', async () => {
    renderWithApp(<TopBar stream={live} />, seeded())
    expect(await screen.findByTestId('cluster-context')).toHaveTextContent('dorgu-dev')
    expect(screen.getByTestId('identity')).toHaveTextContent('alex@example.com')
    expect(screen.getByTestId('live-indicator')).toHaveTextContent('Live')
  })

  it('says when the identity is unavailable instead of leaving a blank', async () => {
    renderWithApp(<TopBar stream={live} />, seeded(meta({ identity: { available: false, reason: 'older than 1.28' } })))
    expect(await screen.findByTestId('identity')).toHaveTextContent('identity unavailable')
  })

  it('shows the namespace scope and the operator state', async () => {
    renderWithApp(
      <TopBar stream={live} />,
      seeded(meta({ cluster: { context: 'dev', namespace: 'apps', server: 's', inCluster: false }, operatorInstalled: false, missingCRDs: ['x'] })),
    )
    const ctx = await screen.findByTestId('cluster-context')
    expect(ctx).toHaveTextContent('/apps')
    expect(ctx).toHaveTextContent('Operator not installed.')
  })

  it('names an in-cluster connection', async () => {
    renderWithApp(<TopBar stream={live} />, seeded(meta({ cluster: { context: '', namespace: '', server: 's', inCluster: true } })))
    expect(await screen.findByTestId('cluster-context')).toHaveTextContent('in-cluster')
  })

  it('has the theme toggle', async () => {
    renderWithApp(<TopBar stream={live} />, seeded())
    expect(await screen.findByRole('button', { name: /Theme: System/ })).toBeInTheDocument()
  })
})

describe('the live indicator', () => {
  const wrap = (stream: StreamStatus) =>
    render(
      <TooltipProvider>
        <LiveIndicator stream={stream} />
      </TooltipProvider>,
    )

  it('says Live with the live state', () => {
    wrap({ state: 'live' })
    expect(screen.getByTestId('live-indicator')).toHaveAttribute('data-state', 'live')
  })

  it('says Stale since the time the stream dropped, in warn', () => {
    wrap({ state: 'connecting', lastLiveAt: Date.parse('2026-11-14T14:02:00') })
    const el = screen.getByTestId('live-indicator')
    expect(el).toHaveTextContent(/^Stale since 14:02$/)
    expect(el.className).toContain('text-warn')
  })

  it('says Stale for an offline stream too', () => {
    wrap({ state: 'offline', lastLiveAt: Date.parse('2026-11-14T09:30:00') })
    expect(screen.getByTestId('live-indicator')).toHaveTextContent('Stale since 09:30')
  })

  it('says Connecting, not stale, before it was ever live', () => {
    wrap({ state: 'connecting' })
    expect(screen.getByTestId('live-indicator')).toHaveTextContent('Connecting')
  })
})

describe('page chrome', () => {
  beforeEach(() => vi.useRealTimers())
  afterEach(() => vi.useRealTimers())

  it('gives each page one h1 and a description', () => {
    render(<PageHeader title="Memory" description="Every incident." trailing={<button>x</button>} />)
    expect(screen.getByRole('heading', { level: 1, name: 'Memory' })).toBeInTheDocument()
    expect(screen.getByText('Every incident.')).toBeInTheDocument()
  })

  it('renders summary counts with their labels', () => {
    render(<SummaryBar items={[{ label: 'open', value: 3, tone: 'warn' }, { label: 'all', value: 5 }]} trailing={<span>t</span>} />)
    expect(screen.getByText('3').className).toContain('text-warn')
    expect(screen.getByText('all')).toBeInTheDocument()
    act(() => {})
  })
})
