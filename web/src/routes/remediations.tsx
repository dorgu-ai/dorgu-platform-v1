import { useQuery } from '@tanstack/react-query'
import { createColumnHelper } from '@tanstack/react-table'
import {
  AlertCircle,
  Bot,
  ChevronDown,
  ChevronRight,
  Cog,
  Hand,
  Lock,
  Terminal,
  Zap,
} from 'lucide-react'
import { createContext, useCallback, useContext, useMemo, useState } from 'react'

import { SummaryBar } from '@/components/app-shell'
import { CopyCommand } from '@/components/copy-command'
import { DataTable, type ColumnMeta } from '@/components/data-table'
import { ViewEmptyState } from '@/components/empty-state'
import { ResourceDiff } from '@/components/remediation-diff'
import { StepSafetyBlock, VerdictBadge } from '@/components/remediation-safety'
import { Dot, ManagedByBadge } from '@/components/status'
import { Badge, type BadgeTone } from '@/components/ui/badge'
import { Tooltip } from '@/components/ui/tooltip'
import { remediationsQuery } from '@/lib/api'
import type { OwnerInstruction, Remediation, Step } from '@/lib/types'
import { useFlash } from '@/lib/use-flash'
import { absolute, age, confidencePercent } from '@/lib/utils'

/**
 * The Remediations view. The diff is the hero, and nothing here writes.
 *
 * # Read-only, deliberately
 *
 * There is no approve button and no disabled approve button. The dashboard
 * writes nothing to the cluster in this release, and pointing at the CLI command
 * that can act is honest in a way a greyed-out control is not: a disabled button
 * reads as something broken or as a permission the reader lacks, and neither is
 * true.
 *
 * # What this screen keeps apart
 *
 * A plan mixes model prose with Dorgu's arithmetic. The rationale is rendered as
 * attributed italic prose; the guardrail verdicts get a bordered panel with a
 * monospace fact grid and a heading that says whose numbers they are. See
 * components/remediation-safety.tsx for why that separation is the point rather
 * than styling.
 */

const column = createColumnHelper<Remediation>()

/**
 * Which plans are expanded, and how to toggle one.
 *
 * This is a context rather than a parameter to the column factory, and the
 * reason is specific. TanStack Table renders a cell through `flexRender`, which
 * calls `createElement(cell)` when the cell is a function: a new arrow function
 * per render is a new component type at that position, so React unmounts and
 * remounts it. Rebuilding the columns whenever the expanded set changed
 * therefore remounted every cell of every visible row on every expand click,
 * not just the row that was clicked, tearing down any open tooltip elsewhere on
 * screen with it.
 *
 * With the state in a context the column definitions are a module-level
 * constant, so the cell functions keep their identity for the life of the page
 * and a toggle re-renders exactly the cells that read the context.
 */
interface Expansion {
  isExpanded: (id: string) => boolean
  toggle: (id: string) => void
}

const ExpansionContext = createContext<Expansion>({
  isExpanded: () => false,
  toggle: () => {},
})

const phaseTone: Record<string, BadgeTone> = {
  Pending: 'info',
  Approved: 'info',
  Applying: 'warn',
  Verifying: 'warn',
  Completed: 'ok',
  Acknowledged: 'warn',
  RolledBack: 'warn',
  Failed: 'danger',
  Rejected: 'neutral',
  Expired: 'neutral',
}

const phaseNote: Record<string, string> = {
  Pending: 'Awaiting a decision. Nothing has been applied.',
  Approved: 'A human approved this. The operator applies the persona change on its next reconcile.',
  Applying: 'The operator is applying the persona change now.',
  Verifying: 'Applied. The operator is checking whether health improved.',
  Completed: 'Applied and verified healthy.',
  Acknowledged:
    'A human approved an advisory plan. The decision is recorded and nothing was applied to your cluster, so the underlying problem is still there. This is not a success.',
  RolledBack: 'Applied, health degraded, and the change was reverted.',
  Failed: 'The operator could not apply this. It also trips a cooldown for the app.',
  Rejected: 'A human declined this plan.',
  Expired: 'The approval deadline passed with no decision.',
}

const riskTone: Record<string, BadgeTone> = {
  low: 'ok',
  medium: 'warn',
  high: 'danger',
  unknown: 'neutral',
}

function remediationVersion(row: Remediation): string {
  return [
    row.phase,
    row.strongestVerdict ?? '',
    row.guardrailCount,
    row.steps.length,
    row.appliedAt ?? '',
    row.verificationResult ?? '',
  ].join('|')
}

export function RemediationsView() {
  const { data, error } = useQuery(remediationsQuery)
  const remediations = useMemo(() => data?.remediations ?? [], [data])
  const flashed = useFlash(remediations, (row) => row.id, remediationVersion)

  // Which plans are expanded. Kept here rather than in the row so it survives an
  // SSE push: the server sends a whole new snapshot on every change, and state
  // held in a row component would collapse the plan the reader was reading.
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set())
  const toggle = useCallback((id: string) => {
    setExpanded((current) => {
      const next = new Set(current)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }, [])

  const expansion = useMemo<Expansion>(
    () => ({ isExpanded: (id) => expanded.has(id), toggle }),
    [expanded, toggle],
  )

  const rowDetail = useCallback(
    (row: Remediation) => (expanded.has(row.id) ? <PlanDetail remediation={row} /> : null),
    [expanded],
  )

  if (error) {
    return (
      <div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
        <AlertCircle className="size-6 text-danger" />
        <h3 className="text-sm font-semibold text-ink">Could not read the Remediations view</h3>
        <p className="max-w-lg text-xs text-ink-muted">{error.message}</p>
      </div>
    )
  }

  const summary = data?.summary
  const empty = (
    <ViewEmptyState
      readiness={data?.readiness}
      rowCount={remediations.length}
      view="remediations"
    />
  )

  return (
    <ExpansionContext.Provider value={expansion}>
      <SummaryBar
        items={[
          { label: 'plans', value: summary?.total ?? 0 },
          { label: 'pending', value: summary?.pending ?? 0, tone: 'info' },
          { label: 'appliable', value: summary?.appliable ?? 0, tone: 'ok' },
          { label: 'advisory', value: summary?.advisory ?? 0, tone: 'warn' },
          ...(summary && summary.guarded > 0
            ? [
                {
                  label: 'guarded',
                  value: summary.guarded,
                  tone: 'warn' as const,
                },
              ]
            : []),
          ...(summary && summary.owned > 0
            ? [
                {
                  label: 'owned workload',
                  value: summary.owned,
                  tone: 'info' as const,
                },
              ]
            : []),
          ...(summary && summary.failed > 0
            ? [
                {
                  label: 'failed',
                  value: summary.failed,
                  tone: 'danger' as const,
                },
              ]
            : []),
        ]}
        trailing={
          <div className="flex items-center gap-3">
            {data?.truncation && (
              <span className="text-[11px] text-warn">
                Showing {data.truncation.shown} of {data.truncation.total}.
              </span>
            )}
            <Tooltip content="This screen cannot approve or apply anything. Every endpoint behind it is a read, and the dashboard writes nothing to your cluster in this release. Each plan carries the CLI command that can act on it.">
              <span className="flex cursor-default items-center gap-1 text-[11px] text-ink-faint">
                <Lock className="size-3" />
                no approve here
              </span>
            </Tooltip>
          </div>
        }
      />
      <DataTable
        ariaLabel="Remediations"
        columns={columns}
        data={remediations}
        emptyState={empty}
        flashedIds={flashed}
        // The plan renders below the row's cells at the full width of the table,
        // not inside the first column: a tall cell would drag the phase badge and
        // every other column to the vertical centre of the whole plan.
        rowDetail={rowDetail}
        // Tall enough for a collapsed row; the virtualiser measures the expanded
        // ones, which is why the plan can be any height without the list jumping.
        estimatedRowHeight={64}
      />
    </ExpansionContext.Provider>
  )
}

// ---------------------------------------------------------------------------
// columns
// ---------------------------------------------------------------------------

const columns = [
  column.display({
    id: 'plan',
    header: 'Plan',
    meta: { width: 'minmax(16rem, 1.6fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <PlanCell remediation={row.original} />,
  }),

  column.accessor('phase', {
    header: 'Phase',
    meta: { width: '9rem' } satisfies ColumnMeta,
    cell: ({ row }) => <PhaseCell remediation={row.original} />,
  }),

  column.accessor('appliable', {
    header: 'Applies',
    meta: { width: '10rem' } satisfies ColumnMeta,
    cell: ({ row }) => <AppliesCell remediation={row.original} />,
  }),

  column.accessor((row) => row.strongestVerdict ?? '', {
    id: 'guardrail',
    header: 'Guardrail',
    meta: { width: '8.5rem' } satisfies ColumnMeta,
    cell: ({ row }) => <GuardrailCell remediation={row.original} />,
  }),

  column.accessor((row) => row.workload?.managedBy ?? '', {
    id: 'owner',
    header: 'Owner',
    meta: { width: '9rem' } satisfies ColumnMeta,
    cell: ({ row }) => <OwnerCell remediation={row.original} />,
  }),

  column.accessor('confidence', {
    header: 'Confidence',
    meta: { width: '8.5rem' } satisfies ColumnMeta,
    cell: ({ row }) => <ConfidenceCell remediation={row.original} />,
  }),

  column.accessor('createdAt', {
    header: 'Age',
    meta: { width: '5rem', numeric: true } satisfies ColumnMeta,
    cell: ({ row }) => (
      <Tooltip content={`Proposed ${absolute(row.original.createdAt)}`}>
        <span className="cursor-default text-ink-faint">{age(row.original.createdAt)}</span>
      </Tooltip>
    ),
  }),
]

/**
 * The plan's name, and the disclosure that opens it.
 *
 * The whole plan is already in the payload, so opening a row costs no request:
 * a remediation is a handful of steps and the cache holds all of it.
 */
function PlanCell({ remediation }: { remediation: Remediation }) {
  // Read from context rather than from a prop baked into the column factory. See
  // ExpansionContext: a column list rebuilt on every toggle gives every cell a
  // new function identity, and React remounts the whole visible table.
  const { isExpanded, toggle } = useContext(ExpansionContext)
  const expanded = isExpanded(remediation.id)

  return (
    <div className="min-w-0">
      <button
        type="button"
        onClick={() => toggle(remediation.id)}
        aria-expanded={expanded}
        className="flex w-full min-w-0 items-center gap-1 text-left hover:text-ink"
      >
        {expanded ? (
          <ChevronDown className="size-3 shrink-0 text-ink-faint" aria-hidden="true" />
        ) : (
          <ChevronRight className="size-3 shrink-0 text-ink-faint" aria-hidden="true" />
        )}
        <span className="truncate font-mono text-ink">{remediation.name}</span>
      </button>
      <div className="flex min-w-0 items-center gap-1.5 pl-4">
        <span className="truncate text-[11px] text-ink-faint">
          {remediation.namespace}/{remediation.persona.name}
        </span>
        <span className="shrink-0 text-[11px] text-ink-faint">
          {remediation.steps.length} step
          {remediation.steps.length === 1 ? '' : 's'}
        </span>
      </div>
    </div>
  )
}

function PhaseCell({ remediation }: { remediation: Remediation }) {
  const tone = phaseTone[remediation.phase] ?? 'neutral'
  return (
    <div className="min-w-0 space-y-0.5">
      <Tooltip content={phaseNote[remediation.phase]}>
        <Badge tone={tone} className="cursor-default">
          {remediation.phase}
        </Badge>
      </Tooltip>
      {/*
        Acknowledged is the one phase that looks like a success and is not: a
        human approved an advisory plan, the decision is recorded, and nothing
        changed in the cluster.
      */}
      {remediation.phase === 'Acknowledged' && (
        <div className="truncate text-[10px] text-warn">nothing was applied</div>
      )}
      {remediation.verificationResult && (
        <div className="truncate text-[10px] text-ink-faint">
          verified {remediation.verificationResult.toLowerCase()}
        </div>
      )}
    </div>
  )
}

/**
 * Whether the product can actually apply this plan.
 *
 * The column exists because of the number that gated this view: clean-room run #4
 * measured nine AI-planned remediations and zero that could change a workload,
 * and nothing on any screen said so. Operator v0.11.1 fixed the cause; this says
 * it out loud for the cases that remain.
 */
function AppliesCell({ remediation }: { remediation: Remediation }) {
  if (!remediation.appliable) {
    return (
      <Tooltip content="This plan is advisory: nothing in it can be applied for you. Carry out the steps yourself. Nothing changes in the cluster until you do.">
        <Badge tone="warn" className="cursor-default">
          <Hand className="size-3" />
          advisory
        </Badge>
      </Tooltip>
    )
  }

  if (remediation.appliableBlockedBy) {
    return (
      <div className="min-w-0 space-y-0.5">
        <Tooltip content={remediation.appliableBlockedBy}>
          <Badge tone="info" className="cursor-default">
            <Zap className="size-3" />
            persona only
          </Badge>
        </Tooltip>
        <div className="truncate text-[10px] text-ink-faint">not the Deployment</div>
      </div>
    )
  }

  return (
    <Tooltip content="This plan carries a resource change Dorgu can apply, and nothing else owns the workload, so an approved fix reaches the Deployment. Approve it with the CLI.">
      <Badge tone="ok" className="cursor-default">
        <Zap className="size-3" />
        appliable
      </Badge>
    </Tooltip>
  )
}

function GuardrailCell({ remediation }: { remediation: Remediation }) {
  if (!remediation.strongestVerdict) {
    return <span className="text-ink-faint">none</span>
  }
  return <VerdictBadge verdict={remediation.strongestVerdict} count={remediation.guardrailCount} />
}

function OwnerCell({ remediation }: { remediation: Remediation }) {
  const workload = remediation.workload
  if (!workload) {
    return (
      <Tooltip content="Dorgu has no record of the live workload behind this plan, so it cannot tell what owns it. No record is treated as owned, because absence of evidence that patching is safe is not evidence that it is.">
        <span className="cursor-default text-ink-faint italic">no record</span>
      </Tooltip>
    )
  }
  return (
    <div className="min-w-0 space-y-0.5">
      <ManagedByBadge managedBy={workload.managedBy} detail={workload.managedByDetail} />
      <div className="truncate font-mono text-[10px] text-ink-faint">{workload.name}</div>
    </div>
  )
}

/**
 * Confidence and its source together, never apart.
 *
 * "85% ai-anthropic" and "85% rule-based" are different claims by different
 * things, and the reader needs the second word to weigh the first.
 */
function ConfidenceCell({ remediation }: { remediation: Remediation }) {
  const value = Number.parseFloat(remediation.confidence)
  const tone: BadgeTone = Number.isNaN(value)
    ? 'neutral'
    : value >= 0.8
      ? 'ok'
      : value >= 0.5
        ? 'warn'
        : 'danger'

  return (
    <div className="flex min-w-0 items-center gap-1.5">
      <Badge tone={tone}>{confidencePercent(remediation.confidence)}</Badge>
      <Tooltip
        content={
          remediation.aiPlanned
            ? `Planned by ${remediation.planSource}. An AI plan is a hypothesis with a confidence attached, not a measurement. Every guardrail verdict on it is Dorgu's own arithmetic.`
            : `Planned by ${remediation.planSource || 'the rule engine'}, which is deterministic code rather than a model.`
        }
      >
        <span className="flex min-w-0 cursor-default items-center gap-1 text-[11px] text-ink-faint">
          {remediation.aiPlanned ? <Bot className="size-3" /> : <Cog className="size-3" />}
          <span className="truncate">{remediation.planSource || 'rule-based'}</span>
        </span>
      </Tooltip>
    </div>
  )
}

// ---------------------------------------------------------------------------
// the expanded plan
// ---------------------------------------------------------------------------

function PlanDetail({ remediation }: { remediation: Remediation }) {
  const workload = remediation.workload

  return (
    <div className="mt-3 mb-1 space-y-3 border-l-2 border-line-strong pl-3">
      {/* What happens to the running container. The hero, so it goes first. */}
      {(remediation.workloadChanges?.length ?? 0) > 0 && (
        <ResourceDiff
          changes={remediation.workloadChanges ?? []}
          title="Deployment change"
          subtitle={
            workload
              ? `${workload.namespace}/${workload.name}${workload.container ? ` (${workload.container})` : ''}`
              : undefined
          }
          unobserved={workload ? !workload.observed : true}
        />
      )}

      {remediation.planSummary && (
        <Prose
          heading="Plan summary"
          text={remediation.planSummary}
          attributed={remediation.aiPlanned}
        />
      )}
      {remediation.explanation && (
        <Prose
          heading="Explanation"
          text={remediation.explanation}
          attributed={remediation.aiPlanned}
        />
      )}

      <div>
        <h4 className="text-[11px] font-semibold tracking-wide text-ink uppercase">
          Plan ({remediation.steps.length} step
          {remediation.steps.length === 1 ? '' : 's'})
        </h4>
        <ol className="mt-1.5 space-y-2.5">
          {remediation.steps.map((step) => (
            <StepRow
              key={`${step.order}-${step.id}`}
              step={step}
              aiPlanned={remediation.aiPlanned}
            />
          ))}
        </ol>
      </div>

      {workload?.owned && <OwnedWorkloadPanel remediation={remediation} />}

      <div className="flex flex-wrap items-center gap-2 pt-0.5">
        <span className="text-[11px] text-ink-faint">
          <Terminal className="mr-1 inline size-3" aria-hidden="true" />
          Act on this plan with the CLI:
        </span>
        <CopyCommand command={remediation.diffCommand} />
      </div>
    </div>
  )
}

/**
 * Prose from the plan.
 *
 * Rendered as attributed italic paragraph text when a model wrote it, which is
 * what makes the bordered monospace guardrail panel below read as a different
 * kind of statement rather than as a different colour.
 */
function Prose({
  heading,
  text,
  attributed,
}: {
  heading: string
  text: string
  attributed: boolean
}) {
  return (
    <div className="min-w-0">
      <h4 className="text-[11px] font-semibold tracking-wide text-ink uppercase">
        {heading}
        {attributed && (
          <Tooltip content="Written by the model that planned this. It is reasoning, not measurement. Anything Dorgu computed appears in the guardrail panels below, which look different for that reason.">
            <span className="ml-1.5 cursor-default font-normal text-ink-faint normal-case">
              <Bot className="mr-0.5 inline size-3" aria-hidden="true" />
              model prose
            </span>
          </Tooltip>
        )}
      </h4>
      <p
        className={
          attributed
            ? 'mt-0.5 max-w-3xl text-[11px] leading-relaxed whitespace-pre-line text-ink-muted italic'
            : 'mt-0.5 max-w-3xl text-[11px] leading-relaxed whitespace-pre-line text-ink-muted'
        }
      >
        {text}
      </p>
    </div>
  )
}

function StepRow({ step, aiPlanned }: { step: Step; aiPlanned: boolean }) {
  return (
    <li className="min-w-0 space-y-1.5">
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <span className="font-mono text-[11px] text-ink-faint">[{step.order}]</span>
        <span className="font-mono text-[11px] text-ink">{step.type}</span>

        <Tooltip
          content={
            step.risk === 'unknown'
              ? 'The plan recorded no risk for this step. Unknown is not low: an unassessed change to a running workload is not a safe one, and this is the cheapest possible place to guess wrong.'
              : `The plan assessed this step as ${step.risk} risk.`
          }
        >
          <Badge tone={riskTone[step.risk] ?? 'neutral'} className="cursor-default">
            {step.risk} risk
          </Badge>
        </Tooltip>

        <Tooltip
          content={
            step.mode === 'auto'
              ? 'The operator may apply this step itself. Only a persona-update step can be auto-executable: the operator never writes a workload.'
              : 'Advisory. Recorded for a human, the CLI or a platform to apply. Nothing happens until somebody does it.'
          }
        >
          <Badge tone={step.mode === 'auto' ? 'ok' : 'warn'} className="cursor-default">
            {step.mode}
          </Badge>
        </Tooltip>

        {step.status?.phase && (
          <Badge tone={step.status.phase === 'Failed' ? 'danger' : 'neutral'}>
            {step.status.phase}
          </Badge>
        )}
      </div>

      <p className="max-w-3xl text-[11px] leading-relaxed text-ink">{step.description}</p>

      {step.rationale && (
        <p
          className={
            aiPlanned
              ? 'max-w-3xl text-[11px] leading-relaxed text-ink-muted italic'
              : 'max-w-3xl text-[11px] leading-relaxed text-ink-muted'
          }
        >
          {step.rationale}
        </p>
      )}

      {/*
        Dorgu's arithmetic, in its own panel, after any prose a model may have
        authored and before the diff that reflects it. The diff below is the
        outcome of the verdict rather than more of it.
      */}
      {(step.safety?.length ?? 0) > 0 && <StepSafetyBlock entries={step.safety ?? []} />}

      {(step.patchChanges?.length ?? 0) > 0 && (
        <ResourceDiff
          changes={step.patchChanges ?? []}
          title="ApplicationPersona change"
          subtitle="what Dorgu records, not what runs"
          subject="persona"
        />
      )}

      {step.command && (
        <div className="flex flex-wrap items-center gap-2">
          <CopyCommand command={step.command} label="Run" />
        </div>
      )}
      {/*
        A command the object carried and this screen will not offer. Saying why
        beats dropping it silently: a reader who saw it in `kubectl get -o yaml`
        would otherwise think this screen lost it.
      */}
      {step.commandWithheld && (
        <p className="flex max-w-3xl items-start gap-1.5 text-[11px] leading-relaxed text-warn">
          <Lock className="mt-0.5 size-3 shrink-0" aria-hidden="true" />
          {step.commandWithheld}
        </p>
      )}
    </li>
  )
}

/**
 * The trust moment, for a workload Dorgu will not patch.
 *
 * It names the owner, says in one line what would have gone wrong, and hands over
 * the change to make and where to make it. The reader here is the one who types
 * the number into a values file or a Git repo, somewhere Dorgu will never see, so
 * this is the last screen before a decision Dorgu cannot check.
 */
function OwnedWorkloadPanel({ remediation }: { remediation: Remediation }) {
  const workload = remediation.workload
  if (!workload) return null

  return (
    <section className="max-w-3xl rounded-md border border-info/40 bg-info-soft/40 px-2.5 py-2">
      <h4 className="flex items-center gap-1.5 text-[11px] font-semibold text-ink">
        <Dot tone="info" />
        Dorgu will not patch this Deployment
      </h4>
      <p className="mt-1 text-[11px] leading-relaxed text-ink-muted">
        {workload.ownerName} owns it. {workload.whyNotPatched}
      </p>

      {(remediation.ownerInstructions?.length ?? 0) > 0 && (
        <>
          <p className="mt-2 text-[11px] font-medium text-ink">
            Apply it where this workload’s desired state lives:
          </p>
          <ol className="mt-1 space-y-1.5">
            {(remediation.ownerInstructions ?? []).map((instruction) => (
              <InstructionRow key={instruction.order} instruction={instruction} />
            ))}
          </ol>
        </>
      )}
    </section>
  )
}

function InstructionRow({ instruction }: { instruction: OwnerInstruction }) {
  return (
    <li className="min-w-0 space-y-1">
      <div className="flex min-w-0 gap-1.5 text-[11px] leading-relaxed text-ink-muted">
        <span className="shrink-0 font-mono text-ink-faint">[{instruction.order}]</span>
        <span>{instruction.description}</span>
      </div>
      {/*
        Only read-only commands survive the filters on an owned workload, and
        those are worth having: reading is the whole of what Dorgu can still hand
        over here, and `kubectl logs` matters most on exactly the workloads it
        will not patch.
      */}
      {instruction.command && (
        <div className="pl-5">
          <CopyCommand command={instruction.command} label="Run" />
        </div>
      )}
    </li>
  )
}
