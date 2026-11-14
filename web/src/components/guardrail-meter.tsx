import { baselineWord, fieldLabel, guardrailTone, signed, type GuardrailReading } from '@/lib/guardrail'
import { cn } from '@/lib/utils'

import { Tooltip } from './ui/tooltip'

const fill: Record<'ink' | 'warn' | 'danger', string> = {
  ink: 'bg-ink',
  warn: 'bg-warn',
  danger: 'bg-danger',
}

/** Where a ceiling comes from. v0.13.1 of the operator fixes it in the gate itself. */
const DEFAULT_CEILING_SOURCE = 'operator blast-radius rule'

/**
 * The guardrail meter, section 5.3: a label, a thin bar and the numbers, "change
 * +50% of limit (max +100%)". The bar is ink, turns warn above 80% of the ceiling
 * and danger when over it, and always shows the ceiling and where it comes from.
 *
 * Those are Dorgu's numbers, computed from the live workload, which is why they
 * are a meter and not a sentence: nothing here is a model's claim.
 */
export function GuardrailMeter({
  reading,
  source = DEFAULT_CEILING_SOURCE,
}: {
  reading: GuardrailReading
  source?: string
}) {
  const tone = guardrailTone(reading)
  const width = Math.min(100, reading.usedPercent)

  return (
    <div className="space-y-1" data-testid="guardrail-meter" data-tone={tone}>
      <div className="flex flex-wrap items-baseline justify-between gap-x-3">
        <span className="text-small font-medium text-ink">{fieldLabel(reading.field)}</span>
        <span className="font-mono text-code text-ink">
          change {signed(reading.changePercent)} of {baselineWord(reading.field)} (max {signed(reading.ceilingPercent * (reading.changePercent < 0 ? -1 : 1))})
        </span>
      </div>
      <Tooltip content={`${reading.baseline ?? '?'} to ${reading.requested ?? '?'}${reading.permitted ? `, permitted ${reading.permitted}` : ''}`}>
        <div
          role="meter"
          aria-label={`${fieldLabel(reading.field)} change against its ceiling`}
          aria-valuemin={0}
          aria-valuemax={reading.ceilingPercent}
          aria-valuenow={Math.min(Math.abs(reading.changePercent), reading.ceilingPercent)}
          aria-valuetext={`${signed(reading.changePercent)} of a maximum ${signed(reading.ceilingPercent)}${reading.over ? ', over the ceiling' : ''}`}
          className="h-1.5 cursor-default overflow-hidden rounded-pill bg-surface-sunken"
        >
          <div className={cn('h-full', fill[tone])} style={{ width: `${width}%` }} />
        </div>
      </Tooltip>
      <div className="flex flex-wrap gap-x-3 text-small text-ink-muted">
        {reading.baseline && <span>running {reading.baseline}</span>}
        {reading.requested && <span>asked {reading.requested}</span>}
        {reading.permitted && <span>permitted {reading.permitted}</span>}
        <span>ceiling source: {source}</span>
        {reading.over && <span className="font-medium text-danger">over the ceiling</span>}
      </div>
    </div>
  )
}

