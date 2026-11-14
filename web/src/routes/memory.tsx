import { useQuery } from '@tanstack/react-query'
import { createColumnHelper } from '@tanstack/react-table'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useCallback, useMemo } from 'react'

import { PageHeader, SummaryBar } from '@/components/app-shell'
import { DataTable, type ColumnMeta } from '@/components/data-table'
import { ViewEmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LineageLine } from '@/components/incident-parts'
import { MachineText } from '@/components/machine-text'
import { SheetSection, SideSheet } from '@/components/side-sheet'
import { SignalBadge } from '@/components/signal-badge'
import { SeverityBadge } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Tooltip } from '@/components/ui/tooltip'
import { memoryQuery } from '@/lib/api'
import { isModelAuthored } from '@/lib/machine'
import type { MemoryCondition, MemoryRecord } from '@/lib/types'
import { useFlash } from '@/lib/use-flash'
import { absolute, age, confidencePercent } from '@/lib/utils'

const column = createColumnHelper<MemoryRecord>()

/** What each condition on an IncidentMemory means, for the two the proposal gate writes. */
const conditionNote: Record<string, string> = {
  ProposalRefused:
    'The live state. It is rewritten to False when the incident resolves or a later proposal answers it, so it cannot be used to count what the gate refused.',
  ProposalRefusalRecorded:
    'The durable record of a refusal. The operator never clears it, which is why `dorgu report` and this view count refusals from it.',
}

function memoryVersion(m: MemoryRecord): string {
  return [m.phase, m.lastSeen, m.occurrenceCount, m.resolution?.outcome ?? '', m.conditions.length].join('|')
}

const columns = [
  column.accessor('name', {
    header: 'Incident',
    meta: { width: 'minmax(14rem, 1.4fr)' } satisfies ColumnMeta,
    cell: ({ row }) => (
      <div className="min-w-0">
        <Link
          to="/memory"
          search={{ record: row.original.id }}
          aria-label={`Open ${row.original.name}`}
          className="block truncate rounded-pill font-mono text-ink hover:underline"
        >
          {row.original.name}
        </Link>
        <span className="truncate text-small text-ink-muted">{row.original.namespace}</span>
      </div>
    ),
  }),
  column.accessor('signal', {
    header: 'Signal',
    meta: { width: 'minmax(11rem, 0.9fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <SignalBadge incident={row.original} />,
  }),
  column.accessor('severity', {
    header: 'Severity',
    meta: { width: '7.5rem' } satisfies ColumnMeta,
    cell: ({ row }) => <SeverityBadge severity={row.original.severity} />,
  }),
  column.accessor('phase', {
    header: 'Phase',
    meta: { width: '8.5rem' } satisfies ColumnMeta,
    cell: ({ row }) => <Badge tone={row.original.phase === 'Resolved' ? 'ok' : 'neutral'}>{row.original.phase || 'Detected'}</Badge>,
  }),
  column.accessor((m) => m.resolution?.outcome ?? '', {
    id: 'outcome',
    header: 'Outcome',
    meta: { width: '9rem' } satisfies ColumnMeta,
    cell: ({ row }) => {
      const outcome = row.original.resolution?.outcome
      return outcome ? (
        <Badge tone={outcome === 'resolved' ? 'ok' : outcome === 'failed' || outcome === 'rollback' ? 'danger' : 'warn'}>
          {outcome}
        </Badge>
      ) : (
        <span className="text-ink-muted">open</span>
      )
    },
  }),
  column.accessor((m) => m.refusalRecord?.reason ?? '', {
    id: 'refusal',
    header: 'Refusal record',
    meta: { width: 'minmax(10rem, 1fr)' } satisfies ColumnMeta,
    cell: ({ row }) =>
      row.original.refusalRecord ? (
        <Tooltip content={row.original.refusalRecord.message}>
          <span className="cursor-default truncate font-mono text-code text-ink">
            {row.original.refusalRecord.reason}
          </span>
        </Tooltip>
      ) : (
        <span className="text-ink-muted">none</span>
      ),
  }),
  column.accessor('lastSeen', {
    header: 'Last seen',
    meta: { width: '6rem', numeric: true } satisfies ColumnMeta,
    cell: ({ row }) => (
      <Tooltip content={`First seen ${absolute(row.original.firstSeen)}\nLast seen ${absolute(row.original.lastSeen)}`}>
        <span className="cursor-default text-ink-muted">{age(row.original.lastSeen)}</span>
      </Tooltip>
    ),
  }),
]

/** Every IncidentMemory, open or closed, with its conditions. */
export function MemoryView({ record }: { record?: string }) {
  const { data, error } = useQuery(memoryQuery)
  const navigate = useNavigate()
  const memories = useMemo(() => data?.memories ?? [], [data])
  const flashed = useFlash(memories, (m) => m.id, memoryVersion)
  const selected = memories.find((m) => m.id === record)

  const close = useCallback(() => {
    void navigate({ to: '/memory', search: {}, replace: true })
  }, [navigate])

  if (error) {
    return (
      <>
        <PageHeader title="Memory" />
        <ErrorState title="Could not read the Memory view" error={error} />
      </>
    )
  }

  const summary = data?.summary
  return (
    <>
      <PageHeader
        title="Memory"
        description="Every incident the operator has recorded, open or closed, with the conditions it keeps for the life of the incident."
      />
      <SummaryBar
        items={[
          { label: 'recorded', value: summary?.total ?? 0 },
          { label: 'open', value: summary?.open ?? 0, tone: 'warn' },
          { label: 'resolved', value: summary?.resolved ?? 0, tone: 'ok' },
          { label: 'with a refusal record', value: summary?.refused ?? 0, tone: 'neutral' },
          { label: 'diagnosed', value: summary?.withRootCause ?? 0, tone: 'neutral' },
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
        ariaLabel="Incident memory"
        columns={columns}
        data={memories}
        emptyState={<ViewEmptyState readiness={data?.readiness} rowCount={memories.length} view="memory" />}
        flashedIds={flashed}
        selectedId={record}
        estimatedRowHeight={52}
      />
      {record && !selected && data && (
        <p className="px-6 py-2 text-small text-warn" role="status">
          The record {record} is not in this list. It may have been deleted.
        </p>
      )}
      {selected && <MemoryDetail record={selected} onClose={close} />}
    </>
  )
}

function MemoryDetail({ record, onClose }: { record: MemoryRecord; onClose: () => void }) {
  const cause = record.rootCause
  return (
    <SideSheet
      label={`Memory of ${record.name}`}
      title={record.name}
      subtitle={
        <span className="flex flex-wrap items-center gap-2">
          <SeverityBadge severity={record.severity} />
          <SignalBadge incident={record} link={false} />
          <span>{record.namespace}</span>
        </span>
      }
      onClose={onClose}
    >
      <SheetSection title="Detection">
        <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-body">
          <dt className="text-ink-muted">Detected by</dt>
          <dd>{record.source}</dd>
          <dt className="text-ink-muted">First seen</dt>
          <dd>{absolute(record.firstSeen)}</dd>
          <dt className="text-ink-muted">Last seen</dt>
          <dd>{absolute(record.lastSeen)}</dd>
          <dt className="text-ink-muted">Occurrences</dt>
          <dd>{record.occurrenceCount}</dd>
          <dt className="text-ink-muted">Persona</dt>
          <dd className="font-mono text-code">
            {record.persona.namespace ? `${record.persona.namespace}/` : ''}
            {record.persona.name}
            {!record.persona.exists && <span className="ml-2 font-sans text-warn">no such persona</span>}
          </dd>
        </dl>
        <LineageLine incident={record} />
      </SheetSection>

      <SheetSection title="Root cause">
        {!cause ? (
          <p className="text-body text-ink-muted">This incident was detected but not diagnosed.</p>
        ) : isModelAuthored(cause.provider) ? (
          <div className="space-y-2">
            <MachineText text={cause.summary} />
            <p className="text-small text-ink-muted">
              Confidence {confidencePercent(cause.confidence)}, diagnosed by {cause.provider}.
            </p>
          </div>
        ) : (
          <div className="space-y-1">
            <p className="max-w-[720px] text-body text-ink">{cause.summary}</p>
            <p className="text-small text-ink-muted">
              Confidence {confidencePercent(cause.confidence)}, diagnosed by {cause.provider}, which is
              deterministic code rather than a model.
            </p>
          </div>
        )}
      </SheetSection>

      <SheetSection title="Resolution">
        {record.resolution ? (
          <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-body">
            <dt className="text-ink-muted">Outcome</dt>
            <dd>{record.resolution.outcome ?? 'recorded without an outcome'}</dd>
            <dt className="text-ink-muted">Action</dt>
            <dd>{record.resolution.action}</dd>
            {record.resolutionDuration && (
              <>
                <dt className="text-ink-muted">Took</dt>
                <dd>{record.resolutionDuration}</dd>
              </>
            )}
            {record.resolution.remediationName && (
              <>
                <dt className="text-ink-muted">Proposal</dt>
                <dd>
                  <Link
                    to="/remediations"
                    search={{ review: `${record.resolution.remediationNamespace ?? record.namespace}/${record.resolution.remediationName}` }}
                    className="font-mono text-code text-accent hover:underline"
                  >
                    {record.resolution.remediationName}
                  </Link>
                </dd>
              </>
            )}
          </dl>
        ) : (
          <p className="text-body text-ink-muted">Not resolved. The incident is {record.phase || 'Detected'}.</p>
        )}
      </SheetSection>

      <SheetSection title="Conditions">
        <ConditionsTable conditions={record.conditions} />
      </SheetSection>
    </SideSheet>
  )
}

function ConditionsTable({ conditions }: { conditions: MemoryCondition[] }) {
  if (conditions.length === 0) {
    return <p className="text-body text-ink-muted">The operator has written no conditions on this incident.</p>
  }
  return (
    <table className="w-full max-w-[760px] text-table" data-testid="conditions">
      <caption className="sr-only">Status conditions on this incident</caption>
      <thead className="text-left text-small text-ink-muted">
        <tr>
          <th scope="col" className="pb-1 font-medium">Condition</th>
          <th scope="col" className="pb-1 font-medium">Status</th>
          <th scope="col" className="pb-1 font-medium">Reason</th>
          <th scope="col" className="pb-1 text-right font-medium">Changed</th>
        </tr>
      </thead>
      <tbody className="divide-y divide-line align-top">
        {conditions.map((c) => (
          <tr key={c.type}>
            <th scope="row" className="py-1.5 pr-3 text-left font-mono text-code font-normal text-ink">
              {c.type}
              {conditionNote[c.type] && (
                <span className="mt-0.5 block font-sans text-small text-ink-muted">{conditionNote[c.type]}</span>
              )}
            </th>
            <td className="py-1.5 pr-3 text-ink">{c.status}</td>
            <td className="py-1.5 pr-3">
              <span className="font-mono text-code text-ink">{c.reason}</span>
              {c.message && <span className="mt-0.5 block text-small text-ink-muted">{c.message}</span>}
            </td>
            <td className="py-1.5 text-right text-ink-muted">{c.lastTransitionTime ? age(c.lastTransitionTime) : ''}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

export function MemoryRoute() {
  const { record } = useSearch({ strict: false })
  return <MemoryView record={record} />
}
