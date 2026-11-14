import { Link } from '@tanstack/react-router'

import type { Incident } from '@/lib/types'
import { recurrenceLabel } from '@/lib/utils'

import { Badge } from './ui/badge'
import { Tooltip } from './ui/tooltip'

/**
 * The signal badge, section 5.3: the signal family (Readiness, RestartLoop,
 * Liveness, CrashLoopBackOff, OOMKilled) plus the recurrence, `r1`, `r2`, which is
 * the incident's -rN suffix. It links to the lineage: the Incidents view filtered
 * to every incident that shares this one's key, so a recurrence can be read in the
 * order it happened.
 */
export function SignalBadge({ incident, link = true }: { incident: Pick<Incident, 'signal' | 'lineage'>; link?: boolean }) {
  const { lineage, signal } = incident
  const recurrence = recurrenceLabel(lineage.generation)

  const badge = (
    <Badge tone="outline" className="font-mono">
      {signal} <span className="text-ink">{recurrence}</span>
    </Badge>
  )
  const note = `Lineage ${lineage.key}${lineage.recurrenceOf ? `, recurring ${lineage.recurrenceOf}` : ''}. Opens every incident in this lineage.`

  if (!link) return badge
  return (
    <Tooltip content={note}>
      <Link
        to="/incidents"
        search={{ lineage: lineage.key }}
        aria-label={`${signal} ${recurrence}: show the lineage ${lineage.key}`}
        className="rounded-pill"
      >
        {badge}
      </Link>
    </Tooltip>
  )
}
