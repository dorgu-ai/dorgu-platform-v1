import { Check, Clock, Hourglass, Ticket } from 'lucide-react'
import { useEffect, useState } from 'react'

import { expiredText } from '@/lib/gate'
import type { RemediationReason, RemediationTicket } from '@/lib/types'
import { absolute, countdown } from '@/lib/utils'

import { Badge } from './ui/badge'
import { Tooltip } from './ui/tooltip'

function useNow(active: boolean, override?: number): number {
  const [now, setNow] = useState(() => override ?? Date.now())
  useEffect(() => {
    if (!active || override !== undefined) return
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [active, override])
  return override ?? now
}

/**
 * The ticket chip, section 5.3. Four states:
 *
 *   not issued          neutral
 *   issued              accent outline with a countdown, "expires in 12:41"
 *   consumed            ok
 *   expired, with why   warn: ApprovalTimedOut, Superseded, PredatesGate,
 *                       IncidentResolved, or the ticket's own clock running out
 *
 * `now` is a parameter so a test can pin the clock; production leaves it unset and
 * the chip ticks once a second while a ticket is issued.
 */
export function TicketChip({
  ticket,
  expired,
  now: pinned,
}: {
  ticket?: RemediationTicket
  /** Why the whole plan expired, when it did. A plan can expire with no ticket ever issued. */
  expired?: RemediationReason
  now?: number
}) {
  const live = ticket?.state === 'issued' && !expired
  const now = useNow(live, pinned)

  if (expired) {
    return (
      <Tooltip content={expiredText(expired)}>
        <Badge tone="warn" className="cursor-default">
          <Hourglass className="size-3" aria-hidden="true" />
          Ticket expired: {expired.reason}
        </Badge>
      </Tooltip>
    )
  }

  if (!ticket) {
    return (
      <Tooltip content="No apply ticket was issued. A ticket is written when an approval clears the gate's re-check, and it is the only thing that may be applied.">
        <Badge tone="neutral" className="cursor-default">
          <Ticket className="size-3" aria-hidden="true" />
          No ticket issued
        </Badge>
      </Tooltip>
    )
  }

  if (ticket.state === 'consumed') {
    return (
      <Tooltip content={`The plan was applied under this ticket. It ${ticket.issuedAt ? `was issued ${absolute(ticket.issuedAt)} and ` : ''}can not be used again.`}>
        <Badge tone="ok" className="cursor-default">
          <Check className="size-3" aria-hidden="true" />
          Ticket consumed
        </Badge>
      </Tooltip>
    )
  }

  const remaining = Date.parse(ticket.expiresAt) - now
  if (ticket.state === 'expired' || Number.isNaN(remaining) || remaining <= 0) {
    return (
      <Tooltip content={`Expired ${absolute(ticket.expiresAt)}. Nothing is applied under an expired ticket.`}>
        <Badge tone="warn" className="cursor-default">
          <Hourglass className="size-3" aria-hidden="true" />
          Ticket expired: past its expiry
        </Badge>
      </Tooltip>
    )
  }

  return (
    <Tooltip content={`Issued ${ticket.issuedAt ? absolute(ticket.issuedAt) : 'by the gate'}. It expires ${absolute(ticket.expiresAt)}.`}>
      <Badge tone="accent" className="cursor-default">
        <Clock className="size-3" aria-hidden="true" />
        Ticket issued, expires in {countdown(remaining)}
      </Badge>
    </Tooltip>
  )
}
