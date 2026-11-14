import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { createColumnHelper } from '@tanstack/react-table'
import { Bot, Cog, Hand, Lock, Zap } from 'lucide-react'
import { useCallback, useMemo } from 'react'

import { PageHeader, SummaryBar } from '@/components/app-shell'
import { DataTable, type ColumnMeta } from '@/components/data-table'
import { ViewEmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ReviewPanel } from '@/components/review-panel'
import { VerdictBadge } from '@/components/remediation-safety'
import { ManagedByBadge } from '@/components/status'
import { Badge, type BadgeTone } from '@/components/ui/badge'
import { Tooltip } from '@/components/ui/tooltip'
import { metaQuery, remediationsQuery } from '@/lib/api'
import type { Remediation } from '@/lib/types'
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

const phaseTone: Record<string, BadgeTone> = {
  Pending: 'neutral',
  Approved: 'neutral',
  Ticketed: 'neutral',
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
  Ticketed: 'Approved, re-checked by the operator’s gate and cleared. An apply ticket was issued and is the only thing that may be applied.',
  Applying: 'The operator is applying the persona change now.',
  Verifying: 'Applied. The operator is checking whether health improved.',
  Completed: 'Applied and verified healthy.',
  Acknowledged:
    'A human approved an advisory plan. The decision is recorded and nothing was applied to your cluster, so the underlying problem is still there. This is not a success.',
  RolledBack: 'Applied, health degraded, and the change was reverted.',
  Failed: 'The operator could not apply this. It also trips a cooldown for the app.',
  Rejected: 'A human declined this plan.',
  Expired: 'Terminal and nothing was applied. The reason is shown when the proposal is opened.',
}

function remediationVersion(row: Remediation): string {
  return [
    row.phase,
    row.strongestVerdict ?? '',
    row.guardrailCount,
    row.steps.length,
    row.appliedAt ?? '',
    row.verificationResult ?? '',
    row.ticket?.state ?? '',
    row.gate?.result ?? '',
    row.expired?.reason ?? '',
    row.gateRefusal?.reason ?? '',
    row.workload?.managedBy ?? '',
  ].join('|')
}

export function RemediationsView({ review }: { review?: string }) {
  const { data, error } = useQuery(remediationsQuery)
  const identity = useQuery(metaQuery).data?.identity
  const navigate = useNavigate()
  const remediations = useMemo(() => data?.remediations ?? [], [data])
  const flashed = useFlash(remediations, (row) => row.id, remediationVersion)
  const reviewing = remediations.find((row) => row.id === review)

  // The panel is addressed by the URL, so a review can be linked and survives an
  // SSE push: the server sends a whole new snapshot on every change, and state
  // held in a component would be lost with it.
  const close = useCallback(() => {
    void navigate({ to: '/remediations', search: {}, replace: true })
  }, [navigate])

  if (error) {
    return (
      <>
        <PageHeader title="Proposals and actions" />
        <ErrorState title="Could not read the Proposals view" error={error} />
      </>
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
    <>
      <PageHeader
        title="Proposals and actions"
        description="What Dorgu proposed, what the gate decided and what happened next. Open a proposal to review it: this screen is read-only and approving arrives in a later release."
      />
      <SummaryBar
        items={[
          { label: 'plans', value: summary?.total ?? 0 },
          { label: 'pending', value: summary?.pending ?? 0, tone: 'neutral' },
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
                  tone: 'neutral' as const,
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
              <span className="text-small text-warn">
                Showing {data.truncation.shown} of {data.truncation.total}.
              </span>
            )}
            <Tooltip content="This screen cannot approve or apply anything. Every endpoint behind it is a read, and the dashboard writes nothing to your cluster in this release. Each plan carries the CLI command that can act on it.">
              <span className="flex cursor-default items-center gap-1 text-small text-ink-muted">
                <Lock className="size-3" />
                no approve here
              </span>
            </Tooltip>
          </div>
        }
      />
      <DataTable
        ariaLabel="Proposals"
        columns={columns}
        data={remediations}
        emptyState={empty}
        flashedIds={flashed}
        selectedId={review}
        estimatedRowHeight={64}
      />
      {review && !reviewing && data && (
        <p className="px-6 py-2 text-small text-warn" role="status">
          The proposal {review} is not in this list. It may have been deleted or be outside the shown window.
        </p>
      )}
      {reviewing && <ReviewPanel remediation={reviewing} identity={identity} onClose={close} />}
    </>
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
        <span className="cursor-default text-ink-muted">{age(row.original.createdAt)}</span>
      </Tooltip>
    ),
  }),
]

/** The plan's name, which opens the review panel. */
function PlanCell({ remediation }: { remediation: Remediation }) {
  return (
    <div className="min-w-0">
      <Link
        to="/remediations"
        search={{ review: remediation.id }}
        aria-label={`Review ${remediation.name}`}
        className="block truncate rounded-pill font-mono text-ink hover:underline"
      >
        {remediation.name}
      </Link>
      <div className="flex min-w-0 items-center gap-1.5">
        <span className="truncate text-small text-ink-muted">
          {remediation.namespace}/{remediation.persona.name}
        </span>
        <span className="shrink-0 text-small text-ink-muted">
          {remediation.steps.length} step{remediation.steps.length === 1 ? '' : 's'}
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
        <div className="truncate text-caption text-warn">nothing was applied</div>
      )}
      {remediation.expired && (
        <div className="truncate text-caption text-ink-muted">{remediation.expired.reason}</div>
      )}
      {remediation.verificationResult && (
        <div className="truncate text-caption text-ink-muted">
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
          <Badge tone="neutral" className="cursor-default">
            <Zap className="size-3" />
            persona only
          </Badge>
        </Tooltip>
        <div className="truncate text-caption text-ink-muted">not the Deployment</div>
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
    return <span className="text-ink-muted">none</span>
  }
  return <VerdictBadge verdict={remediation.strongestVerdict} count={remediation.guardrailCount} />
}

function OwnerCell({ remediation }: { remediation: Remediation }) {
  const workload = remediation.workload
  if (!workload) {
    return (
      <Tooltip content="Dorgu has no record of the live workload behind this plan, so it cannot tell what owns it. No record is treated as owned, because absence of evidence that patching is safe is not evidence that it is.">
        <span className="cursor-default text-ink-muted italic">no record</span>
      </Tooltip>
    )
  }
  return (
    <div className="min-w-0 space-y-0.5">
      <ManagedByBadge managedBy={workload.managedBy} detail={workload.managedByDetail} />
      <div className="truncate font-mono text-caption text-ink-muted">{workload.name}</div>
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
        <span className="flex min-w-0 cursor-default items-center gap-1 text-small text-ink-muted">
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

export function RemediationsRoute() {
  const { review } = useSearch({ strict: false })
  return <RemediationsView review={review} />
}
