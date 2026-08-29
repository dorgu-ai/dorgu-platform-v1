import { Ruler } from 'lucide-react'

import type { StepSafety } from '@/lib/types'

import { Badge, type BadgeTone } from './ui/badge'
import { Tooltip } from './ui/tooltip'

/**
 * The guardrail verdict, rendered as Dorgu's own arithmetic.
 *
 * # Why this block looks nothing like the prose above it
 *
 * A remediation plan mixes two kinds of statement. A model may have written the
 * root cause, the plan summary and a step's rationale: those are hypotheses with
 * a confidence attached. Everything in `spec.steps[].safety` is Dorgu measuring
 * a live workload: the field, the baseline, the ratio, the ceiling, the value
 * that will actually be applied.
 *
 * The field exists because that distinction was lost. The verdict used to arrive
 * as a `[safety:blast-radius] ...` prefix spliced onto the model's rationale, and
 * in clean-room run #4 that put Dorgu's measurement one line below the model's
 * claim that the same 16x change was "well within a 2x ceiling", with nothing to
 * tell the reader which of the two had been computed.
 *
 * So this is a bordered panel with a heading that says whose numbers these are,
 * a monospace fact grid rather than a sentence, and no prose styling anywhere.
 * The model's rationale is rendered as plain paragraph text, italicised and
 * attributed. A reader should be able to tell them apart from across the room.
 *
 * # Where a refused value may appear
 *
 * Here, and nowhere else. A field a guardrail refused is removed from the step's
 * patch, so it is in no diff on this screen: this block is the only place the
 * reader learns it was asked for at all. `applying nothing` is stated rather than
 * left to be inferred from a missing word.
 */

const verdictTone: Record<string, BadgeTone> = {
  rejected: 'danger',
  clamped: 'warn',
  derived: 'info',
}

const verdictNote: Record<string, string> = {
  rejected:
    'Dorgu refused the value the plan asked for and nothing replaces it. This field is gone from the patch, so no diff on this screen shows it: it will not change.',
  clamped:
    'Dorgu refused the value the plan asked for and substituted one it permits. The diff shows the permitted value, because that is what will actually happen.',
  derived:
    'The plan named a change and carried no patch Dorgu could apply, so Dorgu computed the value itself from the live workload rather than recording a fix that applies nothing.',
}

/** The heading, and the wording is load-bearing rather than decorative. */
const HEADING = 'Dorgu guardrails (Dorgu’s measurement, not the plan’s)'

export function StepSafetyBlock({
  entries,
  className,
}: {
  entries: StepSafety[]
  className?: string
}) {
  if (entries.length === 0) return null

  return (
    <section
      className={
        className ??
        // Capped to a comfortable measure. The messages are full sentences and a
        // line the width of a 1500px viewport is measurably harder to read.
        'max-w-3xl rounded-md border border-line-strong bg-surface-sunken px-2.5 py-2 [font-variant-numeric:tabular-nums]'
      }
      aria-label={HEADING}
    >
      <h4 className="flex items-center gap-1.5 text-[11px] font-semibold tracking-wide text-ink">
        <Ruler className="size-3 text-ink-muted" aria-hidden="true" />
        {HEADING}
      </h4>
      <ul className="mt-1.5 space-y-2">
        {entries.map((entry, index) => (
          <SafetyEntry key={`${entry.field}-${entry.rule}-${index}`} entry={entry} />
        ))}
      </ul>
    </section>
  )
}

function SafetyEntry({ entry }: { entry: StepSafety }) {
  const tone = verdictTone[entry.verdict] ?? 'neutral'

  return (
    <li className="min-w-0">
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        {/*
          An unrecognised verdict still renders. An operator newer than this build
          may add one, and it is still Dorgu's verdict, so dropping it would lose
          a measurement rather than avoid a mistake.
        */}
        <Tooltip content={verdictNote[entry.verdict]}>
          <Badge tone={tone} className="cursor-default">
            {entry.verdict}
          </Badge>
        </Tooltip>
        <code className="truncate font-mono text-[11px] text-ink">{entry.field}</code>
        <span className="text-[11px] text-ink-faint">({entry.rule})</span>
      </div>

      <SafetyFacts entry={entry} />

      {entry.message && (
        <p className="mt-1 text-[11px] leading-relaxed text-ink-muted">{entry.message}</p>
      )}
    </li>
  )
}

/**
 * The facts, as a grid rather than a sentence.
 *
 * Every value is optional in the CRD, so each is rendered only when the entry
 * carries it: a partial record reads as a shorter grid rather than as blanks.
 * The outcome is the exception and is always shown, because an empty permitted
 * value means nothing will be applied and that is worth stating.
 */
function SafetyFacts({ entry }: { entry: StepSafety }) {
  const facts: { label: string; value: string; tone?: 'danger' | 'ok' }[] = []

  if (entry.requested) facts.push({ label: 'requested', value: entry.requested })
  if (entry.baseline) facts.push({ label: 'baseline', value: entry.baseline })
  if (entry.ratio && entry.maxRatio) {
    facts.push({ label: 'measured', value: `${entry.ratio} against a ${entry.maxRatio} ceiling` })
  } else if (entry.ratio) {
    facts.push({ label: 'ratio', value: entry.ratio })
  } else if (entry.maxRatio) {
    facts.push({ label: 'ceiling', value: entry.maxRatio })
  }

  facts.push(
    entry.permitted
      ? { label: 'applying', value: entry.permitted, tone: 'ok' }
      : { label: 'applying', value: 'nothing', tone: 'danger' },
  )

  return (
    <dl className="mt-1 grid grid-cols-[5.5rem_1fr] gap-x-2 gap-y-0.5 text-[11px]">
      {facts.map((fact) => (
        <div key={fact.label} className="col-span-2 grid grid-cols-subgrid">
          <dt className="text-ink-faint">{fact.label}</dt>
          <dd
            className={
              fact.tone === 'danger'
                ? 'font-mono text-danger'
                : fact.tone === 'ok'
                  ? 'font-mono text-ok'
                  : 'font-mono text-ink-muted'
            }
          >
            {fact.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}

/**
 * The strongest verdict in a plan, for the list column.
 *
 * A pointer to a plan worth opening, not the record. The field-by-field account
 * is in the expanded row.
 */
export function VerdictBadge({ verdict, count }: { verdict: string; count: number }) {
  return (
    <Tooltip
      content={`${verdictNote[verdict] ?? 'A Dorgu guardrail ruled on this plan.'} ${count === 1 ? 'One field was ruled on' : `${count} fields were ruled on`}; open the plan for the field-by-field account.`}
    >
      <Badge tone={verdictTone[verdict] ?? 'neutral'} className="cursor-default">
        {verdict}
        {count > 1 && <span className="text-ink-faint">{count}</span>}
      </Badge>
    </Tooltip>
  )
}
