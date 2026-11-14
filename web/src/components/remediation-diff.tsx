import { Plus } from 'lucide-react'

import type { ResourceChange } from '@/lib/types'
import { cn } from '@/lib/utils'

import { Tooltip } from './ui/tooltip'

type DiffSubject = 'workload' | 'persona'

const addedCopy: Record<DiffSubject, { label: string; note: string }> = {
  workload: {
    label: 'adds a key this workload does not set',
    note: 'This workload does not set this key today, so applying the plan introduces it. Dorgu will not do that on its own: the rule engine declines to add a resource key a container has never had.',
  },
  persona: {
    label: 'no prior value recorded',
    note: 'The plan records no prior value for this field, so there is nothing to compare against. An object written by an older operator carries no pre-patch snapshot, and every field then reads as new.',
  },
}

const NOT_SET = 'not set'

/**
 * The diff, section 5.3: mono, two columns on `lg` and up and unified below, the
 * removed side danger-soft and the added side ok-soft, columns labelled "Running
 * now" and "After approval".
 *
 * It is the one place status colours stand for "before and after" rather than
 * health, so every line also carries a - or + sign and a hidden label: colour is
 * never the only signal.
 *
 * Only the diff of what will happen is shown. A value a guardrail refused is not
 * in the patch, so it is in no diff here; the guardrail panel is where the reader
 * learns it was asked for at all.
 */
export function ResourceDiff({
  changes,
  title,
  subtitle,
  unobserved,
  subject = 'workload',
}: {
  changes: ResourceChange[]
  title: string
  subtitle?: string
  unobserved?: boolean
  subject?: DiffSubject
}) {
  if (changes.length === 0) return null

  return (
    <div className="max-w-[760px] min-w-0">
      <div className="flex flex-wrap items-baseline gap-2">
        <h4 className="text-h3 font-semibold text-ink">{title}</h4>
        {subtitle && <span className="font-mono text-small text-ink-muted">{subtitle}</span>}
      </div>

      <div
        className="mt-2 overflow-hidden rounded-ctl border border-line font-mono text-code"
        role="group"
        aria-label={`${title}, before and after`}
      >
        <div className="hidden grid-cols-2 border-b border-line bg-surface-sunken text-caption font-medium tracking-[0.04em] text-ink-muted uppercase lg:grid">
          <span className="px-3 py-1.5">Running now</span>
          <span className="border-l border-line px-3 py-1.5">After approval</span>
        </div>
        <ul className="divide-y divide-line">
          {changes.map((change) => (
            <ChangeRow key={change.path} change={change} added={addedCopy[subject]} />
          ))}
        </ul>
      </div>

      {unobserved && (
        <p className="mt-2 text-small text-ink-muted">
          Dorgu has no record of the live workload, so it cannot show what this container has today.
        </p>
      )}
    </div>
  )
}

function ChangeRow({
  change,
  added,
}: {
  change: ResourceChange
  added: { label: string; note: string }
}) {
  const before = change.added ? NOT_SET : change.before
  return (
    <li>
      <div className="grid grid-cols-1 lg:grid-cols-2">
        <div
          className={cn(
            'flex min-w-0 gap-2 px-3 py-1.5',
            change.changed && !change.added && 'bg-danger-soft text-ink',
            (!change.changed || change.added) && 'text-ink-muted',
          )}
        >
          <span aria-hidden="true" className="select-none">
            {change.changed && !change.added ? '-' : ' '}
          </span>
          <span className="sr-only lg:hidden">Running now: </span>
          <code className="min-w-0 break-all">
            {change.path}: <span className={change.added ? 'italic' : undefined}>{before}</span>
          </code>
        </div>
        <div
          className={cn(
            'flex min-w-0 gap-2 px-3 py-1.5 lg:border-l lg:border-line',
            change.changed ? 'bg-ok-soft text-ink' : 'text-ink-muted',
          )}
        >
          <span aria-hidden="true" className="select-none">
            {change.changed ? '+' : ' '}
          </span>
          <span className="sr-only lg:hidden">After approval: </span>
          <code className="min-w-0 break-all">
            {change.path}: {change.changed ? change.after : 'unchanged'}
          </code>
        </div>
      </div>
      {change.added && (
        <Tooltip content={added.note}>
          <p className="flex w-fit cursor-default items-center gap-1 px-3 pb-1.5 font-sans text-small text-warn">
            <Plus className="size-3" aria-hidden="true" />
            {added.label}
          </p>
        </Tooltip>
      )}
    </li>
  )
}
