import { render, screen } from '@testing-library/react'
import { act } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { countdown } from '@/lib/utils'
import { NOW, ticket } from '@/test/fixtures'

import { TicketChip } from './ticket-chip'

const renderChip = (props: Parameters<typeof TicketChip>[0]) =>
  render(
    <TooltipProvider>
      <TicketChip {...props} />
    </TooltipProvider>,
  )

afterEach(() => vi.useRealTimers())

describe('the ticket chip', () => {
  it('is neutral when no ticket was issued', () => {
    renderChip({ now: NOW })
    expect(screen.getByText('No ticket issued')).toBeInTheDocument()
  })

  it('shows an issued ticket with a countdown', () => {
    renderChip({ ticket: ticket(), now: NOW })
    expect(screen.getByText('Ticket issued, expires in 12:41')).toBeInTheDocument()
  })

  it('shows a consumed ticket as consumed', () => {
    renderChip({ ticket: ticket({ state: 'consumed' }), now: NOW })
    expect(screen.getByText('Ticket consumed')).toBeInTheDocument()
    expect(screen.queryByText(/expires in/)).not.toBeInTheDocument()
  })

  it('applies the browser clock to an issued ticket: past its expiry it is expired', () => {
    renderChip({ ticket: ticket({ expiresAt: new Date(NOW - 1000).toISOString() }), now: NOW })
    expect(screen.getByText(/Ticket expired: past its expiry/)).toBeInTheDocument()
  })

  it('shows a ticket the server already called expired as expired', () => {
    renderChip({ ticket: ticket({ state: 'expired' }), now: NOW })
    expect(screen.getByText(/Ticket expired/)).toBeInTheDocument()
  })

  for (const reason of ['ApprovalTimedOut', 'Superseded', 'PredatesGate', 'IncidentResolved']) {
    it(`says why a plan expired: ${reason}`, () => {
      renderChip({ expired: { reason }, now: NOW })
      expect(screen.getByText(`Ticket expired: ${reason}`)).toBeInTheDocument()
    })
  }

  it('lets an expired plan win over a ticket that looks issued', () => {
    renderChip({ ticket: ticket(), expired: { reason: 'IncidentResolved' }, now: NOW })
    expect(screen.queryByText(/expires in/)).not.toBeInTheDocument()
    expect(screen.getByText(/IncidentResolved/)).toBeInTheDocument()
  })

  it('ticks once a second, as text only', () => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
    renderChip({ ticket: ticket() })
    expect(screen.getByText('Ticket issued, expires in 12:41')).toBeInTheDocument()
    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(screen.getByText('Ticket issued, expires in 12:38')).toBeInTheDocument()
  })

  it('turns into an expired chip when the countdown reaches zero', () => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
    renderChip({ ticket: ticket({ expiresAt: new Date(NOW + 2000).toISOString() }) })
    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(screen.getByText(/Ticket expired/)).toBeInTheDocument()
  })
})

describe('countdown', () => {
  it('formats minutes and seconds, and hours past an hour', () => {
    expect(countdown(0)).toBe('0:00')
    expect(countdown(9_000)).toBe('0:09')
    expect(countdown(12 * 60_000 + 41_000)).toBe('12:41')
    expect(countdown(3_600_000 + 2 * 60_000 + 9_000)).toBe('1:02:09')
  })

  it('never goes negative', () => {
    expect(countdown(-5000)).toBe('0:00')
  })
})
