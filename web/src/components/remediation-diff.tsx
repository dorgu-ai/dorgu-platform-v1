import { ArrowRight, Plus } from 'lucide-react'

import type { ResourceChange } from '@/lib/types'

import { Tooltip } from './ui/tooltip'

/**
 * The diff, which is the hero of the Remediations view.
 *
 * # Why it is a field table and not a text diff
 *
 * The CLI renders two blocks of YAML and lets the reader spot the difference,
 * because that is what a terminal can do. A browser can put the field, the value
 * today and the value after on one line, which is the question being asked.
 *
 * # Two diffs, and the second one is the one that matters
 *
 * A step's patch changes the ApplicationPersona. The Deployment change is what
 * happens to the container that is actually OOMing, and it is grounded in the
 * live workload rather than in the persona. Comparing persona to persona is what
 * made a remediation that introduced a CPU limit the container had never had
 * render as no change at all: the persona was not the thing being changed, so it
 * could not be the thing diffed.
 *
 * # Nothing here can show a refused value
 *
 * Every row comes from the patch, which is the post-guardrail object. A field a
 * guardrail refused is not in it. That is a property of where the data comes
 * from rather than a rule this component has to remember, and it is why the
 * guardrail block is the only place a refused value appears.
 */
/**
 * What this diff is about, which decides what an introduced field means.
 *
 * The two diffs have different subjects, so one sentence cannot serve both. On
 * the Deployment change an introduced key is a resource the running container
 * has never had, which is the case Dorgu's own rule engine declines to create.
 * On the persona change it is a field with no recorded prior value, which is an
 * ordinary state for an object written by an older operator that carries no
 * pre-patch snapshot.
 *
 * It is a prop rather than an exported constant because a module that exports
 * anything besides components loses fast refresh for every file that imports it.
 */
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
  /** True when Dorgu never read the workload, so "before" is unknown not absent. */
  unobserved?: boolean
  /** Which diff this is, which decides how an introduced field is explained. */
  subject?: DiffSubject
}) {
  if (changes.length === 0) return null

  return (
    // Capped rather than full width. The row puts the field on the left and the
    // values on the right, so on a wide screen an uncapped row strands the two
    // halves of one fact at opposite ends of the page.
    <div className="min-w-0 max-w-3xl">
      <div className="flex flex-wrap items-baseline gap-2">
        <h4 className="text-[11px] font-semibold tracking-wide text-ink uppercase">{title}</h4>
        {subtitle && <span className="font-mono text-[11px] text-ink-faint">{subtitle}</span>}
      </div>

      <ul className="mt-1.5 space-y-1">
        {changes.map((change) => (
          <ChangeRow key={change.path} change={change} added={addedCopy[subject]} />
        ))}
      </ul>

      {unobserved && (
        <p className="mt-1.5 text-[11px] text-ink-muted">
          Dorgu has no record of the live workload, so it cannot show what this container has
          today.
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
  return (
    <li className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-0.5 rounded border border-line bg-surface-sunken px-2 py-1 font-mono text-[11px]">
      <code className="truncate text-ink-muted">{change.path}</code>

      <div className="flex shrink-0 items-center gap-1.5">
        <span className={change.added ? 'text-ink-faint italic' : 'text-ink-muted'}>
          {change.before}
        </span>
        {change.changed ? (
          <>
            <ArrowRight className="size-3 text-ink-faint" aria-hidden="true" />
            {/*
              The new value wears the accent, not a status colour: a proposed
              change is neither good nor bad news, and reserving red and green for
              health is what keeps a red dot on this screen a fact about the
              cluster.
            */}
            <span className="font-semibold text-info">{change.after}</span>
          </>
        ) : (
          <span className="text-ink-faint">unchanged</span>
        )}
      </div>

      {/*
        An introduced key is a change of a different kind from one that moves a
        number, and it is the one a review used to miss entirely.
      */}
      {change.added && (
        <Tooltip content={added.note}>
          <span className="col-span-2 flex w-fit cursor-default items-center gap-1 text-[10px] text-warn">
            <Plus className="size-2.5" aria-hidden="true" />
            {added.label}
          </span>
        </Tooltip>
      )}
    </li>
  )
}
