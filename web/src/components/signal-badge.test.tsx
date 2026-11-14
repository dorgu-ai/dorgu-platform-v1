import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { renderWithApp } from '@/test/render'

import { SignalBadge } from './signal-badge'

const lineage = (generation: number) => ({ key: 'frontend-crashloop', generation, flapping: false })

describe('the signal badge', () => {
  it('shows the signal with its recurrence', () => {
    renderWithApp(<SignalBadge incident={{ signal: 'OOMKilled', lineage: lineage(0) }} />)
    return screen.findByText('OOMKilled').then((el) => expect(el).toHaveTextContent('OOMKilled r1'))
  })

  it('reads r2 for the -r2 recurrence', async () => {
    renderWithApp(<SignalBadge incident={{ signal: 'CrashLoopBackOff', lineage: lineage(2) }} />)
    expect(await screen.findByText('r2')).toBeInTheDocument()
  })

  it('links to the lineage', async () => {
    const { history } = renderWithApp(<SignalBadge incident={{ signal: 'CrashLoopBackOff', lineage: lineage(2) }} />)
    const link = await screen.findByRole('link', { name: /show the lineage frontend-crashloop/ })
    await userEvent.click(link)
    await waitFor(() => expect(history.location.pathname).toBe('/incidents'))
    expect(history.location.search).toContain('lineage=frontend-crashloop')
  })

  it('is plain when it must not link', async () => {
    renderWithApp(<SignalBadge link={false} incident={{ signal: 'RestartLoop', lineage: lineage(1) }} />)
    expect(await screen.findByText('RestartLoop')).toBeInTheDocument()
    expect(screen.queryByRole('link')).toBeNull()
  })
})
