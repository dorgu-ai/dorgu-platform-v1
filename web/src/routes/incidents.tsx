import { useQuery } from '@tanstack/react-query'
import { createColumnHelper } from '@tanstack/react-table'
import { Link, useSearch } from '@tanstack/react-router'
import { ArrowUpRight, Bot, Cog } from 'lucide-react'
import { useMemo } from 'react'

import { PageHeader, SummaryBar } from '@/components/app-shell'
import { DataTable, type ColumnMeta } from '@/components/data-table'
import { ViewEmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LineageLine, ProposalCell } from '@/components/incident-parts'
import { MachineTextInline } from '@/components/machine-text'
import { SignalBadge } from '@/components/signal-badge'
import { SeverityBadge, UnattributedBadge } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Tooltip } from '@/components/ui/tooltip'
import { incidentsQuery } from '@/lib/api'
import { isModelAuthored } from '@/lib/machine'
import type { Incident } from '@/lib/types'
import { useFlash } from '@/lib/use-flash'
import { absolute, age, confidencePercent } from '@/lib/utils'

const column = createColumnHelper<Incident>()

function incidentVersion(incident: Incident): string {
  return [
    incident.phase,
    incident.severity,
    incident.lastSeen,
    incident.occurrenceCount,
    incident.rootCause?.summary ?? '',
    incident.resolution?.outcome ?? '',
    incident.proposal?.state ?? '',
    incident.proposal?.reason ?? '',
    incident.lineage.generation,
  ].join('|')
}

const columns = [
  column.accessor('severity', {
    header: 'Severity',
    meta: { width: '6.5rem' } satisfies ColumnMeta,
    sortingFn: (a, b) => severityRank(a.original.severity) - severityRank(b.original.severity),
    cell: ({ row }) => <SeverityBadge severity={row.original.severity} />,
  }),

  column.accessor('signal', {
    header: 'Signal',
    meta: { width: 'minmax(11rem, 1fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <SignalCell incident={row.original} />,
  }),

  column.accessor((incident) => incident.persona.name, {
    id: 'persona',
    header: 'Persona',
    meta: { width: 'minmax(7rem, 0.6fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <PersonaCell incident={row.original} />,
  }),

  column.accessor((incident) => incident.rootCause?.summary ?? '', {
    id: 'rootCause',
    header: 'Root cause',
    enableSorting: false,
    meta: { width: 'minmax(12rem, 1.8fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <RootCauseCell incident={row.original} />,
  }),

  column.accessor((incident) => incident.rootCause?.confidence ?? '', {
    id: 'confidence',
    header: 'Confidence',
    meta: { width: '7rem' } satisfies ColumnMeta,
    cell: ({ row }) => <ConfidenceCell incident={row.original} />,
  }),

  column.accessor((incident) => incident.proposal?.state ?? '', {
    id: 'proposal',
    header: 'Proposal',
    meta: { width: 'minmax(8rem, 0.8fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <ProposalCell incident={row.original} />,
  }),

  column.accessor('phase', {
    header: 'Phase',
    meta: { width: '7rem' } satisfies ColumnMeta,
    cell: ({ row }) => <PhaseCell incident={row.original} />,
  }),

  column.accessor('lastSeen', {
    header: 'Last seen',
    meta: { width: '4.5rem', numeric: true } satisfies ColumnMeta,
    cell: ({ row }) => <LastSeenCell incident={row.original} />,
  }),
]

function severityRank(severity: Incident['severity']): number {
  switch (severity) {
    case 'critical':
      return 0
    case 'warning':
      return 1
    default:
      return 2
  }
}

/**
 * The incident feed.
 *
 * Ordered newest-first by the server, and it stays that way by default: this is a
 * feed, and the thing that just broke is the thing being looked for. A critical
 * incident from yesterday is not more urgent than a warning from ten seconds ago,
 * which is what the severity column and the counts above are for.
 */
export function IncidentsView({ lineage }: { lineage?: string }) {
  const { data, error } = useQuery(incidentsQuery)
  const all = useMemo(() => data?.incidents ?? [], [data])
  // A lineage is every incident that shares a key: the original and its -rN recurrences.
  const incidents = useMemo(
    () => (lineage ? all.filter((incident) => incident.lineage.key === lineage) : all),
    [all, lineage],
  )
  const flashed = useFlash(incidents, (incident) => incident.id, incidentVersion)

  const empty = (
    <ViewEmptyState readiness={data?.readiness} rowCount={incidents.length} view="incidents" />
  )

  if (error) {
    return (
      <>
        <PageHeader title="Incidents" />
        <ErrorState title="Could not read the Incidents view" error={error} />
      </>
    )
  }

  const summary = data?.summary

  return (
    <>
      <PageHeader
        title="Incidents"
        description="What Dorgu detected, newest first. A signal badge shows the recurrence (r1, r2) and opens its lineage."
      />
      {lineage && (
        <div className="flex shrink-0 items-center gap-3 border-t border-line bg-accent-soft px-6 py-2 text-small text-ink">
          <span>
            Showing the lineage <code className="font-mono text-code">{lineage}</code>: {incidents.length}{' '}
            incident{incidents.length === 1 ? '' : 's'}, oldest recurrence first in the name.
          </span>
          <Link to="/incidents" className="font-medium text-accent hover:underline">
            Show all incidents
          </Link>
        </div>
      )}
      <SummaryBar
        items={[
          { label: 'incidents', value: summary?.total ?? 0 },
          { label: 'open', value: summary?.open ?? 0, tone: 'warn' },
          { label: 'critical', value: summary?.critical ?? 0, tone: 'danger' },
          { label: 'warning', value: summary?.warning ?? 0, tone: 'warn' },
          { label: 'info', value: summary?.info ?? 0, tone: 'neutral' },
          { label: 'diagnosed', value: summary?.diagnosed ?? 0, tone: 'ok' },
          ...(summary && summary.unattributed > 0
            ? [{ label: 'unattributed', value: summary.unattributed, tone: 'warn' as const }]
            : []),
        ]}
        trailing={
          data?.truncation ? (
            <span className="text-small text-warn">
              Showing the {data.truncation.shown} most recent of {data.truncation.total}.
            </span>
          ) : null
        }
      />
      <DataTable
        ariaLabel="Incidents"
        columns={columns}
        data={incidents}
        emptyState={empty}
        flashedIds={flashed}
        estimatedRowHeight={52}
      />
    </>
  )
}

// ---------------------------------------------------------------------------
// cells
// ---------------------------------------------------------------------------

function SignalCell({ incident }: { incident: Incident }) {
  const escalated = (incident.signals.escalatedFrom?.length ?? 0) > 0
  return (
    <div className="min-w-0 space-y-0.5">
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <SignalBadge incident={incident} />
        {escalated && (
          <Tooltip
            content={`Escalated from ${(incident.signals.escalatedFrom ?? []).join(' > ')}${incident.signals.escalatedAt ? ` at ${absolute(incident.signals.escalatedAt)}` : ''}. The incident moved up its signal family because a worse shape of the same fault appeared.`}
          >
            <Badge tone="warn" className="cursor-default">
              <ArrowUpRight className="size-3" aria-hidden="true" />
              escalated
            </Badge>
          </Tooltip>
        )}
      </div>
      <Tooltip content={`Detected by ${incident.source}. Category: ${incident.category}.`}>
        <div className="cursor-default truncate text-small text-ink-muted">{incident.source}</div>
      </Tooltip>
      <LineageLine incident={incident} />
    </div>
  )
}

function PersonaCell({ incident }: { incident: Incident }) {
  return (
    <div className="min-w-0 space-y-0.5">
      <div className="truncate text-ink">{incident.persona.name}</div>
      <div className="flex items-center gap-1.5">
        {incident.persona.namespace && (
          <span className="truncate font-mono text-small text-ink-muted">
            {incident.persona.namespace}
          </span>
        )}
        {incident.attribution === 'unattributed' && <UnattributedBadge />}
      </div>
    </div>
  )
}

/**
 * The root cause, or the honest absence of one.
 *
 * Detection runs for free and AI diagnosis is opt-in because it costs money and
 * sends data out. So an undiagnosed incident is a normal state, and the cell says
 * "not diagnosed" rather than leaving a blank that reads like a rendering bug.
 */
function RootCauseCell({ incident }: { incident: Incident }) {
  const cause = incident.rootCause
  if (!cause) {
    return (
      <Tooltip content="This incident was detected but not diagnosed. Rule-based diagnosis runs on its own; AI diagnosis is opt-in because it costs money and sends cluster data to a provider.">
        <span className="cursor-default text-ink-muted italic">not diagnosed</span>
      </Tooltip>
    )
  }

  const contributing = cause.contributing ?? []

  return (
    <div className="min-w-0 space-y-0.5">
      {isModelAuthored(cause.provider) ? (
        <MachineTextInline text={cause.summary} />
      ) : (
        <Tooltip content={cause.summary}>
          <div className="cursor-default truncate text-ink">{cause.summary}</div>
        </Tooltip>
      )}
      {contributing.length > 0 && (
        <Tooltip
          content={contributing.map((signal) => `${signal.signal}: ${signal.detail}`).join('\n')}
        >
          <div className="cursor-default truncate text-small text-ink-muted">
            {contributing.length} contributing signal{contributing.length === 1 ? '' : 's'}
          </div>
        </Tooltip>
      )}
    </div>
  )
}

/**
 * Confidence and provider together, never apart.
 *
 * A number on its own invites more trust than it has earned. "85% ai-enhanced"
 * and "85% rule-engine" are different claims by different things, and the reader
 * needs the second word to weigh the first.
 */
function ConfidenceCell({ incident }: { incident: Incident }) {
  const cause = incident.rootCause
  // No em dash placeholder: house style, and "none" says what is meant anyway.
  if (!cause) return <span className="text-ink-muted">none</span>

  const value = Number.parseFloat(cause.confidence)
  const tone = Number.isNaN(value) ? 'neutral' : value >= 0.8 ? 'ok' : value >= 0.5 ? 'warn' : 'danger'
  const ai = isModelAuthored(cause.provider)

  return (
    <div className="min-w-0 space-y-0.5">
      <Badge tone={tone}>{confidencePercent(cause.confidence)}</Badge>
      <Tooltip
        content={
          ai
            ? `Diagnosed by ${cause.provider}. An AI diagnosis is a hypothesis with a confidence attached, not a measurement.`
            : `Diagnosed by ${cause.provider}, which is deterministic code rather than a model.`
        }
      >
        <span className="flex cursor-default items-center gap-1 text-small text-ink-muted">
          {ai ? <Bot className="size-3" /> : <Cog className="size-3" />}
          <span className="truncate">{cause.provider}</span>
        </span>
      </Tooltip>
    </div>
  )
}

/**
 * The phase, and the resolution outcome when there is one.
 *
 * "acknowledged" gets called out because it is the one outcome that does not mean
 * anything changed: a human approved an advisory plan, the decision was recorded,
 * and the incident is still open. Rendering it as a plain success would be the
 * cheerful-lie failure mode this product exists to avoid.
 */
function PhaseCell({ incident }: { incident: Incident }) {
  const outcome = incident.resolution?.outcome
  const phase = incident.phase || 'Detected'

  return (
    <div className="min-w-0 space-y-0.5">
      <Badge
        tone={
          phase === 'Resolved'
            ? outcome === 'acknowledged'
              ? 'warn'
              : 'ok'
            : phase === 'Recurring'
              ? 'danger'
              : 'neutral'
        }
      >
        {phase}
      </Badge>
      {outcome === 'acknowledged' && (
        <Tooltip content="A human approved an advisory plan. The decision is recorded and nothing was applied to your cluster, so the underlying problem is still there.">
          <div className="cursor-default truncate text-small text-warn">acknowledged only</div>
        </Tooltip>
      )}
      {incident.occurrenceCount > 1 && (
        <div className="truncate text-small text-ink-muted">
          seen {incident.occurrenceCount}&#215;
        </div>
      )}
    </div>
  )
}

function LastSeenCell({ incident }: { incident: Incident }) {
  return (
    <Tooltip
      content={`First seen ${absolute(incident.firstSeen)}\nLast seen ${absolute(incident.lastSeen)}`}
    >
      <span className="cursor-default text-ink-muted">{age(incident.lastSeen)}</span>
    </Tooltip>
  )
}

export function IncidentsRoute() {
  const { lineage } = useSearch({ strict: false })
  return <IncidentsView lineage={lineage} />
}
