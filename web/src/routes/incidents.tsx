import { useQuery } from '@tanstack/react-query'
import { createColumnHelper } from '@tanstack/react-table'
import { AlertCircle, Bot, Cog } from 'lucide-react'
import { useMemo } from 'react'

import { SummaryBar } from '@/components/app-shell'
import { DataTable, type ColumnMeta } from '@/components/data-table'
import { ViewEmptyState } from '@/components/empty-state'
import { SeverityBadge, UnattributedBadge } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Tooltip } from '@/components/ui/tooltip'
import { incidentsQuery } from '@/lib/api'
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
  ].join('|')
}

const columns = [
  column.accessor('severity', {
    header: 'Severity',
    meta: { width: '7.5rem' } satisfies ColumnMeta,
    sortingFn: (a, b) => severityRank(a.original.severity) - severityRank(b.original.severity),
    cell: ({ row }) => <SeverityBadge severity={row.original.severity} />,
  }),

  column.accessor('signal', {
    header: 'Signal',
    meta: { width: 'minmax(11rem, 0.9fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <SignalCell incident={row.original} />,
  }),

  column.accessor((incident) => incident.persona.name, {
    id: 'persona',
    header: 'Persona',
    meta: { width: 'minmax(10rem, 0.8fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <PersonaCell incident={row.original} />,
  }),

  column.accessor((incident) => incident.rootCause?.summary ?? '', {
    id: 'rootCause',
    header: 'Root cause',
    enableSorting: false,
    meta: { width: 'minmax(18rem, 2fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <RootCauseCell incident={row.original} />,
  }),

  column.accessor((incident) => incident.rootCause?.confidence ?? '', {
    id: 'confidence',
    header: 'Confidence',
    meta: { width: '8.5rem' } satisfies ColumnMeta,
    cell: ({ row }) => <ConfidenceCell incident={row.original} />,
  }),

  column.accessor('phase', {
    header: 'Phase',
    meta: { width: '8.5rem' } satisfies ColumnMeta,
    cell: ({ row }) => <PhaseCell incident={row.original} />,
  }),

  column.accessor('lastSeen', {
    header: 'Last seen',
    meta: { width: '6rem', numeric: true } satisfies ColumnMeta,
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
export function IncidentsView() {
  const { data, error } = useQuery(incidentsQuery)
  const incidents = useMemo(() => data?.incidents ?? [], [data])
  const flashed = useFlash(incidents, (incident) => incident.id, incidentVersion)

  const empty = (
    <ViewEmptyState readiness={data?.readiness} rowCount={incidents.length} view="incidents" />
  )

  if (error) {
    return (
      <div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
        <AlertCircle className="size-6 text-danger" />
        <h3 className="text-sm font-semibold text-ink">Could not read the Incidents view</h3>
        <p className="max-w-lg text-xs text-ink-muted">{error.message}</p>
      </div>
    )
  }

  const summary = data?.summary

  return (
    <>
      <SummaryBar
        items={[
          { label: 'incidents', value: summary?.total ?? 0 },
          { label: 'open', value: summary?.open ?? 0, tone: 'warn' },
          { label: 'critical', value: summary?.critical ?? 0, tone: 'danger' },
          { label: 'warning', value: summary?.warning ?? 0, tone: 'warn' },
          { label: 'info', value: summary?.info ?? 0, tone: 'info' },
          { label: 'diagnosed', value: summary?.diagnosed ?? 0, tone: 'ok' },
          ...(summary && summary.unattributed > 0
            ? [{ label: 'unattributed', value: summary.unattributed, tone: 'warn' as const }]
            : []),
        ]}
        trailing={
          data?.truncation ? (
            <span className="text-[11px] text-warn">
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
  return (
    <div className="min-w-0">
      <div className="truncate font-mono text-ink">{incident.signal}</div>
      <Tooltip content={`Detected by ${incident.source}. Category: ${incident.category}.`}>
        <div className="cursor-default truncate text-[11px] text-ink-faint">{incident.source}</div>
      </Tooltip>
    </div>
  )
}

function PersonaCell({ incident }: { incident: Incident }) {
  return (
    <div className="min-w-0 space-y-0.5">
      <div className="truncate text-ink">{incident.persona.name}</div>
      <div className="flex items-center gap-1.5">
        {incident.persona.namespace && (
          <span className="truncate font-mono text-[11px] text-ink-faint">
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
        <span className="cursor-default text-ink-faint italic">not diagnosed</span>
      </Tooltip>
    )
  }

  const contributing = cause.contributing ?? []

  return (
    <div className="min-w-0">
      <Tooltip content={cause.summary}>
        <div className="cursor-default truncate text-ink">{cause.summary}</div>
      </Tooltip>
      {contributing.length > 0 && (
        <Tooltip
          content={contributing.map((signal) => `${signal.signal}: ${signal.detail}`).join('\n')}
        >
          <div className="cursor-default truncate text-[11px] text-ink-faint">
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
  if (!cause) return <span className="text-ink-faint">none</span>

  const value = Number.parseFloat(cause.confidence)
  const tone = Number.isNaN(value) ? 'neutral' : value >= 0.8 ? 'ok' : value >= 0.5 ? 'warn' : 'danger'
  const ai = cause.provider.includes('ai')

  return (
    <div className="flex items-center gap-1.5">
      <Badge tone={tone}>{confidencePercent(cause.confidence)}</Badge>
      <Tooltip
        content={
          ai
            ? `Diagnosed by ${cause.provider}. An AI diagnosis is a hypothesis with a confidence attached, not a measurement.`
            : `Diagnosed by ${cause.provider}, which is deterministic code rather than a model.`
        }
      >
        <span className="flex cursor-default items-center gap-1 text-[11px] text-ink-faint">
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
          <div className="cursor-default truncate text-[11px] text-warn">acknowledged only</div>
        </Tooltip>
      )}
      {incident.occurrenceCount > 1 && (
        <div className="truncate text-[11px] text-ink-faint">
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
      <span className="cursor-default text-ink-faint">{age(incident.lastSeen)}</span>
    </Tooltip>
  )
}
