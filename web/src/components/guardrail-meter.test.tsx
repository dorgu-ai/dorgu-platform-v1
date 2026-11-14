import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { readGuardrail } from '@/lib/guardrail'

import { GuardrailMeter } from './guardrail-meter'

const meter = (ratio: string, extra: Record<string, string> = {}) =>
  render(
    <TooltipProvider>
      <GuardrailMeter
        reading={readGuardrail({ field: 'spec.resources.limits.memory', ratio, max: '2.0', baseline: '128Mi', requested: '192Mi', ...extra })!}
      />
    </TooltipProvider>,
  )

describe('the guardrail meter', () => {
  it('says the change, the ceiling and where the ceiling comes from', () => {
    meter('1.50')
    expect(screen.getByText('memory limit')).toBeInTheDocument()
    expect(screen.getByText('change +50% of limit (max +100%)')).toBeInTheDocument()
    expect(screen.getByText(/ceiling source: operator blast-radius rule/)).toBeInTheDocument()
  })

  it('shows the baseline and the requested value as text, not only on hover', () => {
    meter('1.50')
    expect(screen.getByText('running 128Mi')).toBeInTheDocument()
    expect(screen.getByText('asked 192Mi')).toBeInTheDocument()
  })

  it('is exposed as a meter with a value text a screen reader can use', () => {
    meter('1.50')
    const m = screen.getByRole('meter')
    expect(m).toHaveAttribute('aria-valuenow', '50')
    expect(m).toHaveAttribute('aria-valuemax', '100')
    expect(m).toHaveAttribute('aria-valuetext', '+50% of a maximum +100%')
  })

  it('is ink under the ceiling, warn above 80% of it and danger over it', () => {
    for (const [ratio, tone] of [['1.50', 'ink'], ['1.90', 'warn'], ['32.00', 'danger']] as const) {
      const { unmount } = meter(ratio)
      expect(screen.getByTestId('guardrail-meter')).toHaveAttribute('data-tone', tone)
      unmount()
    }
  })

  it('says in words that it is over the ceiling, and caps the bar', () => {
    meter('32.00', { permitted: '256Mi' })
    expect(screen.getByText('over the ceiling')).toBeInTheDocument()
    expect(screen.getByText('permitted 256Mi')).toBeInTheDocument()
    expect(screen.getByRole('meter')).toHaveAttribute('aria-valuenow', '100')
    expect(screen.getByRole('meter').firstElementChild).toHaveStyle({ width: '100%' })
  })

  it('shows a decrease with a negative sign and the 50% floor', () => {
    meter('0.70')
    expect(screen.getByText('change -30% of limit (max -50%)')).toBeInTheDocument()
  })
})
