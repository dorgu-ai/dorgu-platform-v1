import type * as React from 'react'

import { cn } from '@/lib/utils'

import { Tooltip } from './tooltip'

/**
 * A stat tile: one current value, labelled.
 *
 * A handful of headline numbers is a KPI row of these, not a grouped bar chart.
 * The number is the chart.
 *
 * There is deliberately no delta and no sparkline. Both need a previous value,
 * and this process holds only current state: the informers give what is true
 * now, not a history. A trend line drawn from one sample would be a fabricated
 * claim, which is the failure mode this product exists to avoid, and inventing
 * one to fill the shape of a tile would be the worst possible reason.
 *
 * The value uses proportional figures rather than the tabular ones the body sets,
 * because equal-width digits make a large standalone number look loose. Columns
 * of numbers keep tabular; a headline does not.
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
  tone?: 'ok' | 'warn' | 'danger' | 'info'
  note?: React.ReactNode
}) {
  const body = (
    <div className="flex min-w-0 flex-col gap-0.5 rounded-lg border border-line bg-surface-raised px-3 py-2">
      <span className="truncate text-[11px] text-ink-faint">{label}</span>
      <span
        className={cn(
          'text-lg leading-tight font-semibold [font-variant-numeric:proportional-nums]',
          tone === 'danger' && 'text-danger',
          tone === 'warn' && 'text-warn',
          tone === 'ok' && 'text-ok',
          tone === 'info' && 'text-info',
          !tone && 'text-ink',
        )}
      >
        {value}
      </span>
      {detail && <span className="truncate text-[11px] text-ink-muted">{detail}</span>}
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
    <div className="grid grid-cols-2 gap-2 px-4 py-3 sm:grid-cols-3 lg:grid-cols-6">{children}</div>
  )
}
