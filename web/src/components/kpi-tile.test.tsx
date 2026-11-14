import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'

import { KpiTile } from './kpi-tile'

const tile = (props: Partial<Parameters<typeof KpiTile>[0]> = {}) =>
  render(
    <TooltipProvider>
      <KpiTile label="Safe heals" value={4} provenance="from RemediationActions, the last 7 days" {...props} />
    </TooltipProvider>,
  )

describe('the KPI tile', () => {
  it('shows a figure at KPI size with its provenance', () => {
    tile()
    expect(screen.getByText('4').className).toContain('text-kpi')
    expect(screen.getByText('from RemediationActions, the last 7 days')).toBeInTheDocument()
  })

  it('shows no data in words at heading size, flagged for tests and readers', () => {
    tile({ value: 'no data', noData: true, caption: 'the incidents could not be read' })
    expect(screen.getByText('no data').className).not.toContain('text-kpi')
    expect(screen.getByTestId('kpi-tile')).toHaveAttribute('data-no-data', 'true')
    expect(screen.getByText('the incidents could not be read')).toBeInTheDocument()
  })

  it('does not paint a no-data tile in a status colour', () => {
    tile({ value: 'no data', noData: true, tone: 'ok' })
    expect(screen.getByText('no data').className).not.toContain('text-ok')
  })

  it('paints a measured figure in its status colour', () => {
    tile({ tone: 'warn' })
    expect(screen.getByText('4').className).toContain('text-warn')
  })
})
