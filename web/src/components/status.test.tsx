import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import type { ManagedBy } from '@/lib/types'

import { ADVISORY_ONLY, HealthBadge, ManagedByBadge, SeverityBadge, UnattributedBadge } from './status'

const wrap = (ui: React.ReactNode) => render(<TooltipProvider>{ui}</TooltipProvider>)

describe('the ownership badge', () => {
  it('shows Unmanaged as neutral and says nothing about advisory', () => {
    wrap(<ManagedByBadge managedBy="unmanaged" showAdvisory />)
    const badge = screen.getByText('Unmanaged')
    expect(badge.className).toContain('bg-surface-sunken')
    expect(badge.className).not.toContain('border-warn')
    expect(screen.queryByText(ADVISORY_ONLY)).not.toBeInTheDocument()
  })

  const owners: [ManagedBy, string][] = [
    ['helm', 'Helm'], ['argocd', 'Argo CD'], ['flux', 'Flux'], ['kustomize', 'Kustomize'],
  ]
  for (const [owner, label] of owners) {
    it(`shows ${label} as a warn outline with the advisory sentence`, () => {
      wrap(<ManagedByBadge managedBy={owner} showAdvisory />)
      expect(screen.getByText(label).className).toContain('border-warn')
      expect(screen.getByText(ADVISORY_ONLY)).toHaveTextContent('advisory only: Dorgu will not write this workload')
    })
  }

  it('treats an unknown owner as owned: the advisory sentence, but no claim about a named owner', () => {
    wrap(<ManagedByBadge managedBy="unknown" showAdvisory />)
    expect(screen.getByText('Unknown owner').className).not.toContain('border-warn')
    expect(screen.getByText(ADVISORY_ONLY)).toBeInTheDocument()
  })

  it('leaves the sentence out when not asked, as in a table cell', () => {
    wrap(<ManagedByBadge managedBy="helm" />)
    expect(screen.queryByText(ADVISORY_ONLY)).not.toBeInTheDocument()
  })
})

describe('the other status badges', () => {
  it('pairs a severity colour with its word', () => {
    wrap(<SeverityBadge severity="critical" />)
    expect(screen.getByText('critical')).toBeInTheDocument()
  })

  it('says where a health reading came from, and flags an operator that disagrees', () => {
    wrap(<HealthBadge status="Unhealthy" source="observed" disagreement="Operator says Healthy." />)
    expect(screen.getByText('Unhealthy')).toBeInTheDocument()
    expect(screen.getByText('observed')).toBeInTheDocument()
    expect(screen.getByText('operator says otherwise')).toBeInTheDocument()
  })

  it('shows a persona verdict without the observed tag', () => {
    wrap(<HealthBadge status="Healthy" source="persona-status" />)
    expect(screen.queryByText('observed')).not.toBeInTheDocument()
  })

  it('flags an unattributed incident', () => {
    wrap(<UnattributedBadge />)
    expect(screen.getByText('unattributed')).toBeInTheDocument()
  })
})
