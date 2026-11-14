import type * as React from 'react'

import { cn } from '@/lib/utils'

import { Tooltip } from './ui/tooltip'

/**
 * A KPI tile, for the Overview only (the only place `text-kpi` is used).
 *
 * Numbers carry their provenance, so a tile is a figure, what it is a figure of,
 * where it came from and over what window: "from RemediationActions and
 * IncidentMemories, the last 7 days". A figure that cannot be computed shows
 * "no data" and the reason in words, never 0. The tile has no delta and no
 * sparkline, for the reason StatTile gives.
 */
export function KpiTile({
  label,
  value,
  caption,
  provenance,
  noData = false,
  note,
  tone,
}: {
  label: string
  value: React.ReactNode
  caption?: React.ReactNode
  provenance: string
  /** True when value is the words "no data" rather than a measurement. */
  noData?: boolean
  note?: React.ReactNode
  tone?: 'ok' | 'warn' | 'danger'
}) {
  const body = (
    <div
      className="flex h-full min-w-0 flex-col gap-1 rounded-ctl border border-line bg-surface p-4"
      data-testid="kpi-tile"
      data-no-data={noData ? 'true' : 'false'}
    >
      <h3 className="text-small font-medium text-ink-muted">{label}</h3>
      <p
        className={cn(
          noData ? 'text-h2 font-semibold text-ink-muted' : 'text-kpi font-semibold',
          !noData && !tone && 'text-ink',
          !noData && tone === 'ok' && 'text-ok',
          !noData && tone === 'warn' && 'text-warn',
          !noData && tone === 'danger' && 'text-danger',
        )}
      >
        {value}
      </p>
      {caption && <p className="text-small text-ink-muted">{caption}</p>}
      <p className="mt-auto pt-1 text-small text-ink-muted">{provenance}</p>
    </div>
  )
  if (!note) return body
  return (
    <Tooltip content={note}>
      <div className="min-w-0 cursor-default">{body}</div>
    </Tooltip>
  )
}
