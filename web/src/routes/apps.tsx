import { useQuery } from '@tanstack/react-query'
import { createColumnHelper } from '@tanstack/react-table'
import { AlertCircle, Eye, EyeOff } from 'lucide-react'
import { useMemo } from 'react'

import { CopyCommand } from '@/components/copy-command'
import { DataTable, type ColumnMeta } from '@/components/data-table'
import { ViewEmptyState } from '@/components/empty-state'
import { HealthBadge, ManagedByBadge } from '@/components/status'
import { SummaryBar } from '@/components/app-shell'
import { Badge } from '@/components/ui/badge'
import { Tooltip } from '@/components/ui/tooltip'
import { appsQuery } from '@/lib/api'
import type { App } from '@/lib/types'
import { useFlash } from '@/lib/use-flash'
import { absolute, age, resourceLine } from '@/lib/utils'

const column = createColumnHelper<App>()

/**
 * What counts as a change for the flash highlight: the facts a reader is
 * watching. Serialising the whole row would flash everything on every push,
 * because the server sends a full snapshot each time.
 */
function appVersion(app: App): string {
  return [
    app.health.status,
    app.health.message ?? '',
    app.workload?.replicas.ready ?? '',
    app.workload?.replicas.desired ?? '',
    app.workload?.image ?? '',
    app.workload?.managedBy ?? '',
    app.incidents.open,
    app.phase ?? '',
  ].join('|')
}

const columns = [
  column.accessor((app) => app.health.status, {
    id: 'health',
    header: 'Health',
    meta: { width: '10.5rem' } satisfies ColumnMeta,
    // Sorted by how much attention it needs, not alphabetically.
    sortingFn: (a, b) =>
      healthRank(a.original.health.status) - healthRank(b.original.health.status),
    cell: ({ row }) => (
      <HealthBadge
        status={row.original.health.status}
        source={row.original.health.source}
        message={row.original.health.message}
        disagreement={row.original.health.disagreement}
      />
    ),
  }),

  column.accessor('name', {
    header: 'App',
    meta: { width: 'minmax(14rem, 1.4fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <AppNameCell app={row.original} />,
  }),

  column.accessor((app) => (app.monitored ? 1 : 0), {
    id: 'monitored',
    header: 'Watched',
    meta: { width: '11rem' } satisfies ColumnMeta,
    cell: ({ row }) => <WatchedCell app={row.original} />,
  }),

  column.accessor((app) => app.workload?.managedBy ?? '', {
    id: 'owner',
    header: 'Owner',
    meta: { width: 'minmax(9rem, 0.8fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <OwnerCell app={row.original} />,
  }),

  column.accessor((app) => app.workload?.name ?? '', {
    id: 'workload',
    header: 'Workload',
    meta: { width: 'minmax(12rem, 1.1fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <WorkloadCell app={row.original} />,
  }),

  column.accessor((app) => app.workload?.observedResources?.limits?.memory ?? '', {
    id: 'resources',
    header: 'Observed resources',
    enableSorting: false,
    meta: { width: 'minmax(11rem, 0.9fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <ResourcesCell app={row.original} />,
  }),

  column.accessor((app) => app.incidents.open, {
    id: 'incidents',
    header: 'Open',
    meta: { width: '4.5rem', numeric: true } satisfies ColumnMeta,
    cell: ({ row }) => <IncidentsCell app={row.original} />,
  }),

  column.accessor('createdAt', {
    id: 'age',
    header: 'Age',
    meta: { width: '4rem', numeric: true } satisfies ColumnMeta,
    cell: ({ row }) => (
      <span className="text-ink-faint" title={absolute(row.original.createdAt)}>
        {age(row.original.createdAt)}
      </span>
    ),
  }),
]

function healthRank(status: App['health']['status']): number {
  switch (status) {
    case 'Unhealthy':
      return 0
    case 'Degraded':
      return 1
    case 'Unknown':
      return 2
    default:
      return 3
  }
}

export function AppsView() {
  const { data, error } = useQuery(appsQuery)
  const apps = useMemo(() => data?.apps ?? [], [data])
  const flashed = useFlash(apps, (app) => app.id, appVersion)

  const empty = (
    <ViewEmptyState readiness={data?.readiness} rowCount={apps.length} view="apps" />
  )

  if (error) {
    return <LoadError message={error.message} />
  }

  const summary = data?.summary

  return (
    <>
      <SummaryBar
        items={[
          { label: 'apps', value: summary?.total ?? 0 },
          { label: 'watched', value: summary?.monitored ?? 0, tone: 'info' },
          { label: 'unwatched', value: summary?.unmonitored ?? 0, tone: 'warn' },
          { label: 'unhealthy', value: summary?.unhealthy ?? 0, tone: 'danger' },
          { label: 'degraded', value: summary?.degraded ?? 0, tone: 'warn' },
          { label: 'healthy', value: summary?.healthy ?? 0, tone: 'ok' },
        ]}
        trailing={
          summary && summary.unmonitored > 0 ? (
            <ImportPrompt namespaces={summary.unmonitoredNamespaces} count={summary.unmonitored} />
          ) : null
        }
      />
      <DataTable
        ariaLabel="Applications"
        columns={columns}
        data={apps}
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

function AppNameCell({ app }: { app: App }) {
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-1.5">
        <span className="truncate font-medium text-ink">{app.name}</span>
        {app.tier === 'critical' && (
          <Badge tone="outline" className="uppercase">
            critical
          </Badge>
        )}
      </div>
      <div className="truncate font-mono text-[11px] text-ink-faint">
        {app.namespace}
        {app.type && <span className="ml-1.5 text-ink-faint">{app.type}</span>}
        {/* The persona's spec.name is what the match chain resolves by, and it is
            often not the object name. Showing both is how a reader can follow
            the link from persona to Deployment. */}
        {app.monitored && app.appName !== app.name && (
          <span className="ml-1.5">as {app.appName}</span>
        )}
      </div>
    </div>
  )
}

/**
 * The column the whole view exists for.
 *
 * An unwatched Deployment gets the import command inline, because that is the
 * one action that changes anything about it. Listing only what Dorgu can already
 * see is how `dorgu health` came to report everything fine on a cluster with
 * three broken apps it had never been told about.
 */
function WatchedCell({ app }: { app: App }) {
  if (app.monitored) {
    return (
      <div className="flex items-center gap-1.5">
        <Badge tone="info">
          <Eye className="size-3" />
          persona
        </Badge>
        {app.phase && app.phase !== 'Active' && <Badge tone="neutral">{app.phase}</Badge>}
      </div>
    )
  }

  return (
    <Tooltip content="No ApplicationPersona covers this Deployment, so Dorgu is not watching it: no detection, no diagnosis, no remediation. Importing it takes one command and changes nothing in your cluster until you pass --apply.">
      <Badge tone="warn" className="cursor-default">
        <EyeOff className="size-3" />
        not watched
      </Badge>
    </Tooltip>
  )
}

function OwnerCell({ app }: { app: App }) {
  return (
    <div className="min-w-0 space-y-0.5">
      {app.workload ? (
        <ManagedByBadge managedBy={app.workload.managedBy} detail={app.workload.managedByDetail} />
      ) : (
        <span className="text-ink-faint">no workload</span>
      )}
      {app.ownership?.team && (
        <div className="truncate text-[11px] text-ink-muted">{app.ownership.team}</div>
      )}
    </div>
  )
}

function WorkloadCell({ app }: { app: App }) {
  const workload = app.workload
  if (!workload) {
    return (
      <div className="flex items-center gap-1.5">
        <Tooltip content={app.warnings?.join(' ')}>
          <span className="flex cursor-default items-center gap-1 text-warn">
            <AlertCircle className="size-3" />
            unresolved
          </span>
        </Tooltip>
      </div>
    )
  }

  const replicas = workload.replicas
  const short = replicas.ready === replicas.desired
  const image = workload.image?.split('/').pop() ?? ''

  return (
    <div className="min-w-0">
      <Tooltip
        content={
          workload.matchedBy
            ? `Matched to persona "${app.appName}" by ${workload.matchedBy}. Container inspected: ${workload.container ?? 'none'}.`
            : `Container inspected: ${workload.container ?? 'none'}.`
        }
      >
        <span className="cursor-default truncate font-mono text-[11px] text-ink">
          {workload.name}
        </span>
      </Tooltip>
      <div className="flex items-center gap-2 text-[11px]">
        <span className={short ? 'text-ink-faint' : 'text-warn'}>
          {replicas.ready}/{replicas.desired}
        </span>
        {image && (
          <Tooltip content={workload.image}>
            <span className="cursor-default truncate font-mono text-ink-faint">{image}</span>
          </Tooltip>
        )}
      </div>
    </div>
  )
}

/**
 * The live resource block, with absent keys named as absent.
 *
 * "This container has no memory limit" and "this container has a memory limit of
 * 0" are different facts, and only the first forbids a remediation from
 * introducing one. Showing a dash where a key is unset is how that reaches the
 * reader.
 */
function ResourcesCell({ app }: { app: App }) {
  const observed = app.workload?.observedResources
  const issues = app.health.podIssues ?? []

  if (issues.length > 0) {
    const first = issues[0]
    return (
      <div className="min-w-0 space-y-0.5">
        <Tooltip
          content={issues
            .map(
              (issue) =>
                `${issue.podName}/${issue.container}: ${issue.reason}` +
                (issue.restartCount ? ` (${issue.restartCount} restarts)` : '') +
                (issue.message ? ` ${issue.message}` : ''),
            )
            .join('\n')}
        >
          <span className="cursor-default font-mono text-[11px] text-danger">
            {first?.reason}
            {issues.length > 1 && <span className="text-ink-faint"> +{issues.length - 1}</span>}
          </span>
        </Tooltip>
        {observed && <ResourceLines observed={observed} />}
      </div>
    )
  }

  if (!observed) {
    return (
      <Tooltip content="This container sets no CPU or memory requests or limits. Dorgu will not introduce a key the workload does not already have.">
        <span className="cursor-default text-ink-faint">none set</span>
      </Tooltip>
    )
  }

  return <ResourceLines observed={observed} />
}

function ResourceLines({ observed }: { observed: NonNullable<App['workload']>['observedResources'] }) {
  const limits = resourceLine(observed?.limits)
  const requests = resourceLine(observed?.requests)

  return (
    <div className="font-mono text-[11px] leading-4">
      {/* An unset key reads as "none", not as a dash. It is a fact worth
          stating in words: a remediation may only change a key the workload
          already has. */}
      <div className="truncate text-ink-muted">
        <span className="text-ink-faint">lim</span>{' '}
        {limits || <span className="text-ink-faint">none</span>}
      </div>
      <div className="truncate text-ink-muted">
        <span className="text-ink-faint">req</span>{' '}
        {requests || <span className="text-ink-faint">none</span>}
      </div>
    </div>
  )
}

/**
 * Open incident count, with the operator's own figure shown when the two
 * disagree.
 *
 * A mismatch is a real fact about the operator, so it is surfaced rather than
 * reconciled away. The cache is the fresher of the two.
 */
function IncidentsCell({ app }: { app: App }) {
  const { open, critical, personaReported } = app.incidents
  const mismatch = personaReported !== open

  const tone = critical > 0 ? 'text-danger' : open > 0 ? 'text-warn' : 'text-ink-faint'

  return (
    <Tooltip
      content={
        mismatch
          ? `The dashboard can see ${open} unresolved incident(s); the persona's status reports ${personaReported}. The dashboard's count is the fresher of the two.`
          : critical > 0
            ? `${critical} of ${open} are critical.`
            : ''
      }
    >
      <span className={`cursor-default font-semibold ${tone}`}>
        {open}
        {mismatch && <span className="ml-0.5 align-super text-[9px] text-ink-faint">*</span>}
      </span>
    </Tooltip>
  )
}

function ImportPrompt({ namespaces, count }: { namespaces: string[]; count: number }) {
  const first = namespaces[0]
  return (
    <div className="flex items-center gap-2">
      <span className="text-[11px] text-warn">
        {count} Deployment{count === 1 ? '' : 's'} Dorgu is not watching
      </span>
      {first && <CopyCommand command={`dorgu persona import -n ${first} --all`} />}
      {namespaces.length > 1 && (
        <Tooltip content={`Also uncovered in: ${namespaces.slice(1).join(', ')}`}>
          <span className="cursor-default text-[11px] text-ink-faint">
            +{namespaces.length - 1} more namespace{namespaces.length === 2 ? '' : 's'}
          </span>
        </Tooltip>
      )}
    </div>
  )
}

function LoadError({ message }: { message: string }) {
  return (
    <div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
      <AlertCircle className="size-6 text-danger" />
      <h3 className="text-sm font-semibold text-ink">Could not read the Apps view</h3>
      <p className="max-w-lg text-xs text-ink-muted">{message}</p>
    </div>
  )
}
