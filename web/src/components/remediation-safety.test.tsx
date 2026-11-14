import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import type { StepSafety } from '@/lib/types'

import { StepSafetyBlock, VerdictBadge } from './remediation-safety'

const entry = (verdict: string, extra: Partial<StepSafety> = {}): StepSafety => ({
  rule: 'blast-radius', verdict, field: 'spec.resources.limits.memory', ratio: '32.00', maxRatio: '2.0',
  baseline: '128Mi', requested: '4Gi', message: 'm', ...extra,
})

const block = (entries: StepSafety[]) =>
  render(
    <TooltipProvider>
      <StepSafetyBlock entries={entries} />
    </TooltipProvider>,
  )

describe('the step safety block tones', () => {
  it('draws a clamped verdict and the value it will apply in warn, never ok', () => {
    block([entry('clamped', { permitted: '256Mi' })])
    expect(screen.getByText('clamped').className).toContain('bg-warn-soft')
    expect(screen.getByText('256Mi').className).toContain('text-warn')
    expect(screen.getByText('256Mi').className).not.toContain('text-ok')
  })

  it('draws a rejected verdict and "applying nothing" in danger', () => {
    block([entry('rejected')])
    expect(screen.getByText('rejected').className).toContain('bg-danger-soft')
    expect(screen.getByText('nothing').className).toContain('text-danger')
  })

  it('draws a derived verdict neutral, because it is no ruling on the plan, and leaves its value as it was', () => {
    block([entry('derived', { permitted: '256Mi' })])
    expect(screen.getByText('derived').className).toContain('bg-surface-sunken')
    expect(screen.getByText('256Mi').className).toContain('text-ok')
  })
})

describe('the verdict badge in the list', () => {
  const badge = (verdict: string) =>
    render(
      <TooltipProvider>
        <VerdictBadge verdict={verdict} count={1} />
      </TooltipProvider>,
    )

  it('uses the same tones as the panel', () => {
    for (const [verdict, cls] of [['clamped', 'bg-warn-soft'], ['rejected', 'bg-danger-soft'], ['derived', 'bg-surface-sunken']] as const) {
      const { unmount } = badge(verdict)
      expect(screen.getByText(verdict).className).toContain(cls)
      unmount()
    }
  })
})
