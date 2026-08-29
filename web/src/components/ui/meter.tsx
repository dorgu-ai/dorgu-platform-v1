import type * as React from 'react'

import { cn } from '@/lib/utils'

import { Tooltip } from './tooltip'

/**
 * A meter: one ratio against a limit.
 *
 * # Why a meter and not a chart
 *
 * The data's job here is "how full is this, against what it is allowed to be".
 * That is a single ratio against a limit, and a bar chart with one bar or a pie
 * with two slices is the wrong shape for it. The number is the point and the
 * track is the context.
 *
 * # The specs, and the one that was measured
 *
 * The fill carries severity and the unfilled track is a lighter step of the same
 * ramp, so state reads across the whole bar. The `-soft` tokens are that step,
 * and they clear 3.86:1 against their own fill in dark mode and 4.05:1 in light,
 * so the boundary is always visible.
 *
 * They do NOT clear against the card they sit on: 1.13:1 to 1.25:1, which means
 * the track's extent would be invisible and the reader could not see where 100%
 * is. So the track carries a hairline border in the line token. That is chrome
 * doing an axis's job, not a stroke around a mark: it bounds the scale rather
 * than outlining the data.
 *
 * The bar is 8px, well under the 24px cap, with a 4px rounded data-end and a
 * square baseline. The value is direct-labelled beside it in an ink token,
 * because text never wears the data colour.
 */
export type MeterTone = 'ok' | 'warn' | 'danger' | 'info' | 'neutral'

const fillColour: Record<MeterTone, string> = {
  ok: 'bg-ok',
  warn: 'bg-warn',
  danger: 'bg-danger',
  info: 'bg-info',
  neutral: 'bg-ink-faint',
}

const trackColour: Record<MeterTone, string> = {
  ok: 'bg-ok-soft',
  warn: 'bg-warn-soft',
  danger: 'bg-danger-soft',
  info: 'bg-info-soft',
  neutral: 'bg-surface-sunken',
}

export function Meter({
  label,
  /** 0 to 100. Values above 100 are clamped for the bar and shown in the value. */
  percent,
  /** The number beside the bar. Never only in a tooltip. */
  value,
  tone = 'info',
  note,
  emphasis = false,
}: {
  label: string
  percent: number
  value: React.ReactNode
  tone?: MeterTone
  note?: React.ReactNode
  emphasis?: boolean
}) {
  const width = Math.min(100, Math.max(0, percent))

  return (
    <div className="flex min-w-0 items-center gap-3">
      <span
        className={cn(
          'w-20 shrink-0 text-[11px]',
          emphasis ? 'text-ink-muted' : 'text-ink-faint',
        )}
      >
        {label}
      </span>

      <Tooltip content={note}>
        <div
          className="h-2 min-w-0 flex-1 cursor-default overflow-hidden rounded border border-line"
          role="meter"
          aria-label={label}
          aria-valuenow={Math.round(percent)}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div className={cn('h-full w-full', trackColour[tone])}>
            {/*
              Rounded on the data end only. A bar rounded at the baseline reads as
              floating rather than as growing from zero.
            */}
            <div
              className={cn('h-full rounded-r-[4px]', fillColour[tone])}
              style={{ width: `${width}%` }}
            />
          </div>
        </div>
      </Tooltip>

      <span className="w-44 shrink-0 text-right font-mono text-[11px] text-ink">{value}</span>
    </div>
  )
}

/**
 * The row a meter takes when there is nothing to measure.
 *
 * There is no bar at all, because a zero-length bar reads as 0% and "nobody
 * measured this" is not "this is idle". The absence and its reason take the
 * bar's place.
 */
export function MeterUnavailable({
  label,
  reason,
}: {
  label: string
  reason: string
}) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <span className="w-20 shrink-0 text-[11px] text-ink-faint">{label}</span>
      {/*
        The reason wraps rather than truncating. It is the whole content of this
        row, and a tooltip may enhance a value but must never be the only way to
        read one.
      */}
      <span className="min-w-0 flex-1 text-[11px] leading-relaxed text-ink-muted italic">
        n/a ({reason})
      </span>
      <span className="w-44 shrink-0 text-right font-mono text-[11px] text-ink-faint">n/a</span>
    </div>
  )
}

/**
 * A meter sized for a table cell.
 *
 * The full Meter reserves a label gutter and a wide value column, which is right
 * beside three other meters in a card and wrong inside a table column: the two
 * fixed gutters left the bar with almost no width at all. Here the value sits
 * above a full-width bar, so the bar gets the whole cell and the column header
 * carries the label.
 */
export function InlineMeter({
  percent,
  value,
  tone = 'info',
  note,
}: {
  percent: number
  value: string
  tone?: MeterTone
  note?: React.ReactNode
}) {
  const width = Math.min(100, Math.max(0, percent))

  return (
    <Tooltip content={note}>
      <div className="min-w-0 cursor-default space-y-1">
        <span className="block truncate font-mono text-[11px] text-ink-muted">{value}</span>
        <div
          className="h-1.5 overflow-hidden rounded border border-line"
          role="meter"
          aria-valuenow={Math.round(percent)}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div className={cn('h-full w-full', trackColour[tone])}>
            <div
              className={cn('h-full rounded-r-[4px]', fillColour[tone])}
              style={{ width: `${width}%` }}
            />
          </div>
        </div>
      </div>
    </Tooltip>
  )
}
