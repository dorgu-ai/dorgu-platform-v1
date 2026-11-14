import { useQuery } from '@tanstack/react-query'
import {
  AlertTriangle,
  KeyRound,
  PackageOpen,
  PowerOff,
  ServerOff,
  ShieldCheck,
  Sparkles,
} from 'lucide-react'
import type * as React from 'react'

import { metaQuery } from '@/lib/api'
import { settledEmptyKind, type EmptyKind, type ViewId } from '@/lib/empty'
import type { OperatorStatus, Readiness, UnavailableResource } from '@/lib/types'

import { CopyCommand } from './copy-command'
import { SkeletonRows } from './ui/skeleton'

/**
 * The five reasons a list can be empty, which are five different screens
 * (DESIGN.md sections 5.7 and 10):
 *
 *   warming        the informers have not finished their first list
 *   crds-missing   the API server does not serve the dorgu.io resources
 *   operator-idle  the CRDs are installed and no operator replica is ready
 *   none           the operator is running and recorded nothing, or there is
 *                  genuinely nothing to list
 *   refused        the kubeconfig may not read the resource, and the screen
 *                  names the resource and the reason
 *
 * Loading and error are the two states around them. Collapsing any of these into
 * "no data" would show a blind spot as good news, which is the one lie a
 * reliability tool must not tell.
 */
export function EmptyState({
  icon,
  title,
  kind,
  children,
}: {
  icon: React.ReactNode
  title: string
  kind?: EmptyKind
  children?: React.ReactNode
}) {
  return (
    <div
      className="flex flex-col items-center gap-3 px-6 py-16 text-center"
      data-empty={kind}
      role="status"
    >
      <div className="text-ink-muted">{icon}</div>
      <h2 className="text-h2 font-semibold text-ink">{title}</h2>
      <div className="max-w-xl text-body text-ink-muted">{children}</div>
    </div>
  )
}

const skeletonColumns: Record<ViewId, number> = {
  apps: 6,
  incidents: 5,
  remediations: 6,
  cluster: 6,
  memory: 5,
  overview: 4,
}

export function ViewEmptyState({
  readiness,
  rowCount,
  view,
}: {
  readiness: Readiness | undefined
  rowCount: number
  view: ViewId
}) {
  const { data: meta } = useQuery(metaQuery)

  if (!readiness) {
    return <SkeletonRows rows={6} columns={skeletonColumns[view]} />
  }

  if ((readiness.unavailable?.length ?? 0) > 0) {
    return <ReadRefused unavailable={readiness.unavailable ?? []} />
  }

  if (!readiness.synced) {
    return (
      <div data-empty="warming" role="status">
        <p className="px-6 pt-4 text-small text-ink-muted">
          Reading your cluster. This is the initial list, not a refresh loop.
        </p>
        <SkeletonRows rows={6} columns={skeletonColumns[view]} />
      </div>
    )
  }

  if (!readiness.crdsInstalled) {
    return <OperatorNotInstalled missing={readiness.missingCRDs ?? []} view={view} />
  }

  if (rowCount === 0) {
    const kind = settledEmptyKind(view, meta?.operator)
    if (kind === 'operator-idle') return <OperatorIdle view={view} operator={meta?.operator} />
    switch (view) {
      case 'apps':
        return <NoWorkloadsAtAll />
      case 'incidents':
        return <NoIncidents />
      case 'remediations':
        return <NoRemediations />
      case 'memory':
        return <NoMemory />
      case 'cluster':
        return <NoNodes />
      case 'overview':
        return <NoActivity />
    }
  }

  return null
}

function ReadRefused({ unavailable }: { unavailable: UnavailableResource[] }) {
  return (
    <EmptyState
      kind="refused"
      icon={<KeyRound className="size-6" aria-hidden="true" />}
      title={`Dorgu could not read ${unavailable.map((entry) => entry.resource).join(', ')}`}
    >
      <p>
        The dashboard has exactly your permissions and holds no credentials of its own, so this is
        what your kubeconfig user is allowed to do rather than a limit Dorgu imposes.
      </p>
      <ul className="mt-3 space-y-1 text-left">
        {unavailable.map((entry) => (
          <li key={entry.resource}>
            <code className="font-mono text-code text-ink">{entry.resource}</code>: {entry.reason}
          </li>
        ))}
      </ul>
      <p className="mt-3">
        If your user is scoped to one namespace, restart with{' '}
        <code className="font-mono text-code text-ink">--namespace</code>. Cluster-wide resources
        such as Nodes cannot be read at all from a namespace-scoped user.
      </p>
    </EmptyState>
  )
}

function OperatorNotInstalled({ missing, view }: { missing: string[]; view: ViewId }) {
  return (
    <EmptyState
      kind="crds-missing"
      icon={<ServerOff className="size-6" aria-hidden="true" />}
      title="The Dorgu operator is not installed on this cluster"
    >
      <p>
        {missing.length > 0 ? (
          <>
            This cluster does not serve{' '}
            <code className="font-mono text-code text-ink">{missing.join(', ')}</code>.
          </>
        ) : (
          <>This cluster does not serve the dorgu.io custom resources.</>
        )}{' '}
        {operatorMissingConsequence[view]}
      </p>
      <div className="mt-4">
        <CopyCommand command="dorgu install" label="Install the operator" />
      </div>
    </EmptyState>
  )
}

const operatorMissingConsequence: Record<ViewId, string> = {
  apps: 'Every Deployment below is therefore unmonitored: Dorgu is not watching any of them.',
  incidents:
    'Nothing is detecting incidents yet, so this is a statement about the cluster rather than good news about your apps.',
  remediations:
    'Nothing is proposing remediations, so this is a statement about the cluster rather than a sign that nothing needs fixing.',
  memory: 'There is no incident record because nothing is writing one.',
  overview:
    'There is nothing to compute the report from, so no figure is shown: a zero here would claim a cluster that was measured and found quiet.',
  cluster:
    'The nodes, capacity and saturation on this screen are read from your cluster directly and still work. The cluster name, the environment and the add-on list come from the operator and are therefore empty.',
}

/**
 * The CRDs are installed and no operator replica is ready. The lists are empty
 * because nothing is writing to them, and that says nothing about whether the
 * cluster is healthy.
 */
function OperatorIdle({ view, operator }: { view: ViewId; operator: OperatorStatus | undefined }) {
  return (
    <EmptyState
      kind="operator-idle"
      icon={<PowerOff className="size-6" aria-hidden="true" />}
      title="The operator is installed and nothing is running"
    >
      <p>
        The dorgu.io resources exist, but the operator Deployment has {operator?.ready ?? 0} of{' '}
        {operator?.desired ?? 1} replicas ready. Nothing is detecting incidents or proposing fixes,
        so an empty {viewNoun[view]} is not good news about your apps.
      </p>
      <div className="mt-4">
        <CopyCommand command="kubectl -n dorgu-system get pods" label="Check the operator" />
      </div>
    </EmptyState>
  )
}

const viewNoun: Record<ViewId, string> = {
  apps: 'list',
  incidents: 'incident feed',
  remediations: 'proposal list',
  memory: 'incident record',
  overview: 'report',
  cluster: 'list',
}

function NoWorkloadsAtAll() {
  return (
    <EmptyState
      kind="none"
      icon={<PackageOpen className="size-6" aria-hidden="true" />}
      title="No Deployments in scope"
    >
      <p>
        There are no Deployments outside the cluster add-on namespaces, and no
        ApplicationPersonas. Deploy something, or point the dashboard at a namespace with{' '}
        <code className="font-mono text-code text-ink">--namespace</code>.
      </p>
    </EmptyState>
  )
}

function NoIncidents() {
  return (
    <EmptyState
      kind="none"
      icon={<AlertTriangle className="size-6" aria-hidden="true" />}
      title="No incidents recorded"
    >
      <p>
        The operator is running and has not recorded an IncidentMemory. Detection is on by
        default; AI diagnosis is opt-in and costs money, so an incident may appear here with no
        root cause attached.
      </p>
    </EmptyState>
  )
}

function NoRemediations() {
  return (
    <EmptyState
      kind="none"
      icon={<ShieldCheck className="size-6" aria-hidden="true" />}
      title="No proposals yet"
    >
      <p>
        A proposal follows a diagnosed incident, so an empty list here usually means nothing has
        broken in a way Dorgu recognised. Rule-based proposals run on their own; AI planning is
        opt-in.
      </p>
      <p className="mt-2">
        Nothing on this screen can approve or apply anything. The dashboard is read-only in this
        release and writes nothing to your cluster.
      </p>
    </EmptyState>
  )
}

function NoMemory() {
  return (
    <EmptyState
      kind="none"
      icon={<AlertTriangle className="size-6" aria-hidden="true" />}
      title="The operator has recorded no incidents"
    >
      <p>
        The operator is running and its incident memory is empty. A record appears here the first
        time Dorgu detects an incident, and stays after the incident resolves.
      </p>
    </EmptyState>
  )
}

function NoActivity() {
  return (
    <EmptyState
      kind="none"
      icon={<Sparkles className="size-6" aria-hidden="true" />}
      title="Nothing has been proposed in this window"
    >
      <p>
        The operator is running and no RemediationAction was created in the window. Every figure
        below is therefore no data, not zero: there is nothing to measure, which is different from
        measuring and finding none.
      </p>
    </EmptyState>
  )
}

function NoNodes() {
  return (
    <EmptyState
      kind="none"
      icon={<Sparkles className="size-6" aria-hidden="true" />}
      title="This cluster reports no nodes"
    >
      <p>
        The node list was read successfully and is empty. Without nodes there is no allocatable
        pool, so there is nothing for saturation to be a share of.
      </p>
    </EmptyState>
  )
}
