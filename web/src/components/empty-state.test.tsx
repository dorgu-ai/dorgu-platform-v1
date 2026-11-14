import { screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { settledEmptyKind } from '@/lib/empty'
import type { Readiness } from '@/lib/types'
import { meta, readyReadiness } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'

import { ViewEmptyState } from './empty-state'

const idleOperator = { found: true, desired: 1, ready: 0, version: 'v0.13.1' }

function renderEmpty(readiness: Readiness | undefined, view: Parameters<typeof ViewEmptyState>[0]['view'], operator = meta().operator, rowCount = 0) {
  return renderWithApp(<ViewEmptyState readiness={readiness} rowCount={rowCount} view={view} />, {
    seed: (qc) => qc.setQueryData(['meta'], meta({ operator })),
  })
}

const kind = () => document.querySelector('[data-empty]')?.getAttribute('data-empty')

describe('the five empty states are five different screens', () => {
  it('loading: skeleton rows, no claim about the cluster', async () => {
    renderEmpty(undefined, 'incidents')
    await screen.findByTestId('skeleton-rows')
    expect(kind()).toBeUndefined()
  })

  it('warming: says it is the initial list, with skeleton rows', async () => {
    renderEmpty({ synced: false, crdsInstalled: true }, 'incidents')
    expect(await screen.findByText(/Reading your cluster/)).toBeInTheDocument()
    expect(kind()).toBe('warming')
    expect(screen.getByTestId('skeleton-rows')).toBeInTheDocument()
  })

  it('CRDs missing: the operator is not installed, names the resources and offers the install command', async () => {
    renderEmpty({ synced: true, crdsInstalled: false, missingCRDs: ['incidentmemories'] }, 'incidents')
    expect(await screen.findByText('The Dorgu operator is not installed on this cluster')).toBeInTheDocument()
    expect(kind()).toBe('crds-missing')
    expect(screen.getByText('incidentmemories')).toBeInTheDocument()
    expect(screen.getByText('dorgu install')).toBeInTheDocument()
    expect(screen.getByText(/rather than good news about your apps/)).toBeInTheDocument()
  })

  it('operator idle: the CRDs exist and no replica is ready, so empty says nothing', async () => {
    renderEmpty(readyReadiness, 'incidents', idleOperator)
    expect(await screen.findByText('The operator is installed and nothing is running')).toBeInTheDocument()
    expect(kind()).toBe('operator-idle')
    expect(screen.getByText(/0 of 1 replicas ready/)).toBeInTheDocument()
    expect(screen.getByText(/not good news about your apps/)).toBeInTheDocument()
  })

  it('none: the operator is running and recorded nothing', async () => {
    renderEmpty(readyReadiness, 'incidents')
    await waitFor(() => expect(kind()).toBe('none'))
    expect(screen.getByText('No incidents recorded')).toBeInTheDocument()
    expect(screen.getByText(/The operator is running/)).toBeInTheDocument()
  })

  it('read refused: names the resource and the reason, and what to do', async () => {
    renderEmpty(
      { synced: false, crdsInstalled: true, unavailable: [{ resource: 'remediationactions', reason: 'forbidden: cannot list' }] },
      'remediations',
    )
    expect(await screen.findByText('Dorgu could not read remediationactions')).toBeInTheDocument()
    expect(kind()).toBe('refused')
    expect(screen.getByText(/forbidden: cannot list/)).toBeInTheDocument()
    expect(screen.getByText('--namespace')).toBeInTheDocument()
  })

  it('refused wins over warming: a read that was refused is not still loading', async () => {
    renderEmpty({ synced: false, crdsInstalled: false, unavailable: [{ resource: 'nodes', reason: 'x' }] }, 'cluster')
    await screen.findByText('Dorgu could not read nodes')
    expect(kind()).toBe('refused')
  })

  it('renders nothing when there are rows', () => {
    const { container } = renderEmpty(readyReadiness, 'incidents', meta().operator, 3)
    expect(container.querySelector('[data-empty]')).toBeNull()
  })
})

describe('the empty states per view', () => {
  const cases: [Parameters<typeof ViewEmptyState>[0]['view'], string][] = [
    ['apps', 'No Deployments in scope'],
    ['remediations', 'No proposals yet'],
    ['memory', 'The operator has recorded no incidents'],
    ['cluster', 'This cluster reports no nodes'],
    ['overview', 'Nothing has been proposed in this window'],
  ]
  for (const [view, title] of cases) {
    it(`${view}: ${title}`, async () => {
      renderEmpty(readyReadiness, view)
      expect(await screen.findByText(title)).toBeInTheDocument()
    })
  }

  it('says the proposals screen cannot approve anything', async () => {
    renderEmpty(readyReadiness, 'remediations')
    expect(await screen.findByText(/read-only in this release/)).toBeInTheDocument()
  })

  it('never says no data means a healthy cluster for the overview when nothing is installed', async () => {
    renderEmpty({ synced: true, crdsInstalled: false, missingCRDs: [] }, 'overview')
    expect(await screen.findByText(/a zero here would claim a cluster that was measured/)).toBeInTheDocument()
  })

  it('names the idle operator for every operator-record view', async () => {
    for (const view of ['remediations', 'memory', 'overview'] as const) {
      const { unmount } = renderEmpty(readyReadiness, view, idleOperator)
      expect(await screen.findByText('The operator is installed and nothing is running')).toBeInTheDocument()
      unmount()
    }
  })
})

describe('settledEmptyKind', () => {
  it('is none for the views that list live objects, whatever the operator does', () => {
    expect(settledEmptyKind('apps', idleOperator)).toBe('none')
    expect(settledEmptyKind('cluster', idleOperator)).toBe('none')
  })

  it('is operator-idle only with evidence that the operator exists and has no ready replica', () => {
    expect(settledEmptyKind('incidents', idleOperator)).toBe('operator-idle')
    expect(settledEmptyKind('incidents', { found: false, desired: 0, ready: 0, version: null })).toBe('none')
    expect(settledEmptyKind('incidents', undefined)).toBe('none')
    expect(settledEmptyKind('incidents', meta().operator)).toBe('none')
  })
})
