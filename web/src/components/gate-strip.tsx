import { Check, CircleDashed, Hourglass, X } from 'lucide-react'

import { readStages, type StageReading, type StageState } from '@/lib/gate-stages'
import type { RemediationGate, RemediationReason } from '@/lib/types'
import { cn } from '@/lib/utils'

import { Tooltip } from './ui/tooltip'

/**
 * The gate strip, section 5.3: six pills in the order the operator runs its gate.
 *
 *   passed    ok soft
 *   refused   danger soft, with the reason on hover and in the panel below
 *   deferred  warn soft, and says it is not a refusal
 *   not run   neutral outline
 *
 * A refusal stops the strip. The operator writes no verdicts after one, and the
 * strip does not invent any: the stages after it render as "not run". A plan with
 * no gate record at all renders every pill as not run, which is what it is.
 */
const pillClass: Record<StageState, string> = {
  passed: 'border-ok/40 bg-ok-soft text-ok',
  refused: 'border-danger/40 bg-danger-soft text-danger',
  deferred: 'border-warn/40 bg-warn-soft text-warn',
  'not-run': 'border-line-strong bg-transparent text-ink-muted',
}

const stateWord: Record<StageState, string> = {
  passed: 'passed',
  refused: 'refused',
  deferred: 'deferred',
  'not-run': 'not run',
}

function StateIcon({ state }: { state: StageState }) {
  const className = 'size-3 shrink-0'
  switch (state) {
    case 'passed':
      return <Check className={className} aria-hidden="true" />
    case 'refused':
      return <X className={className} aria-hidden="true" />
    case 'deferred':
      return <Hourglass className={className} aria-hidden="true" />
    default:
      return <CircleDashed className={className} aria-hidden="true" />
  }
}

function stageNote(r: StageReading): string | undefined {
  switch (r.state) {
    case 'refused':
      return `The gate refused at ${r.stage}${r.reason ? `: ${r.reason}` : ''}. Later stages did not run.`
    case 'deferred':
      return 'Deferred, not refused: a proposal for this incident is already waiting, so the gate stood down. Nothing was rejected.'
    case 'passed':
      return r.clamped
        ? `Passed with a value narrowed to what Dorgu permits${r.reason ? `. ${r.reason}` : ''}`
        : r.reason
    default:
      return undefined
  }
}

export function GateStrip({
  gate,
  className,
}: {
  gate: RemediationGate | undefined
  className?: string
}) {
  const stages = readStages(gate)

  return (
    <div className={cn('space-y-2', className)}>
      <ol
        aria-label="Gate stages"
        className="flex flex-wrap items-center gap-1.5"
        data-testid="gate-strip"
      >
        {stages.map((r) => (
          <li key={r.stage}>
            <Tooltip content={stageNote(r)}>
              <span
                data-stage={r.stage}
                data-state={r.state}
                className={cn(
                  'inline-flex cursor-default items-center gap-1 rounded-pill border px-2 py-1 text-caption font-medium',
                  pillClass[r.state],
                )}
              >
                <StateIcon state={r.state} />
                {r.label}
                {r.clamped && <span>· clamped</span>}
                <span className="sr-only">: {stateWord[r.state]}</span>
              </span>
            </Tooltip>
          </li>
        ))}
      </ol>
      {!gate && (
        <p className="text-small text-ink-muted">
          The operator wrote no gate record for this plan, so no stage ran against it.
        </p>
      )}
    </div>
  )
}

/**
 * The reasons behind the strip, in words. The strip is the summary; this is what
 * the gate said, for the stages that said something.
 */
export function GateReasons({
  gate,
  refusal,
}: {
  gate: RemediationGate | undefined
  refusal?: RemediationReason
}) {
  const stages = readStages(gate).filter((r) => r.state === 'refused' || r.state === 'deferred' || r.clamped)
  if (stages.length === 0 && !refusal) return null

  return (
    <ul className="space-y-1 text-small text-ink-muted" aria-label="Gate reasons">
      {stages.map((r) => (
        <li key={r.stage}>
          <span className="font-medium text-ink">{r.label}</span>
          {r.state === 'refused' && ': the gate refused. '}
          {r.state === 'deferred' && ': deferred, which is not a refusal. '}
          {r.state === 'passed' && r.clamped && ': passed with a value narrowed. '}
          {r.reason}
        </li>
      ))}
      {refusal && (
        <li>
          <span className="font-medium text-ink">Refused at approval ({refusal.reason})</span>
          {': '}
          {refusal.message ?? 'The gate refused this plan when it was approved. Nothing was applied.'}
        </li>
      )}
    </ul>
  )
}
