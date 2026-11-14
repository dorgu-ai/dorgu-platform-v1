import type * as React from 'react'

import { cn } from '@/lib/utils'

import { Tooltip } from './tooltip'

/**
 * A stat tile: one current value, labelled.
 *
 * A handful of headline numbers is a row of these, not a grouped bar chart. The
 * number is the chart.
 *
 * There is deliberately no delta and no sparkline. Both need a previous value,
 * and this process holds only current state: the informers give what is true
 * now, not a history. A trend line drawn from one sample would be a fabricated
 * claim, which is the failure mode this product exists to avoid.
 */
export function StatTile({
  label,
  value,
  detail,
  tone,
  note,
}: {
  label: string
  value: React.ReactNode
  detail?: React.ReactNode
  tone?: 'ok' | 'warn' | 'danger' | 'neutral'
  note?: React.ReactNode
}) {
  const body = (
    <div className="flex min-w-0 flex-col gap-0.5 rounded-ctl border border-line bg-surface px-3 py-2">
      <span className="truncate text-small text-ink-muted">{label}</span>
      <span
        className={cn(
          'text-h2 font-semibold',
          tone === 'danger' && 'text-danger',
          tone === 'warn' && 'text-warn',
          tone === 'ok' && 'text-ok',
          (!tone || tone === 'neutral') && 'text-ink',
        )}
      >
        {value}
      </span>
      {detail && <span className="truncate text-small text-ink-muted">{detail}</span>}
    </div>
  )

  if (!note) return body
  return (
    <Tooltip content={note}>
      <div className="min-w-0 cursor-default">{body}</div>
    </Tooltip>
  )
}

/** A row of stat tiles, which is what a handful of headline numbers wants. */
export function StatRow({ children }: { children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-2 gap-2 px-6 py-3 sm:grid-cols-3 lg:grid-cols-6">{children}</div>
  )
}
