import { AlertTriangle, KeyRound, PackageOpen, ServerOff, ShieldCheck, Sparkles } from 'lucide-react'
import type * as React from 'react'

import type { Readiness, UnavailableResource } from '@/lib/types'

import { CopyCommand } from './copy-command'
import { SkeletonRows } from './ui/skeleton'

/**
 * The empty states double as onboarding, which is the whole point of them.
 *
 * An empty list has four different causes and they are four different screens.
 * The clean-room run found `dorgu health` presenting "I cannot see any of your
 * apps" as health; a table that renders zero rows identically no matter why is
 * the same bug with better typography.
 */
export function EmptyState({
  icon,
  title,
  children,
}: {
  icon: React.ReactNode
  title: string
  children?: React.ReactNode
}) {
  return (
    <div className="flex flex-col items-center gap-3 px-6 py-16 text-center">
      <div className="text-ink-faint">{icon}</div>
      <h3 className="text-sm font-semibold text-ink">{title}</h3>
      <div className="max-w-xl text-xs leading-relaxed text-ink-muted">{children}</div>
    </div>
  )
}

/** The views that have an empty state of their own. */
export type ViewId = 'apps' | 'incidents' | 'remediations' | 'cluster'

/** How many skeleton columns each view's table has, so the layout does not jump. */
const skeletonColumns: Record<ViewId, number> = {
  apps: 6,
  incidents: 5,
  remediations: 6,
  cluster: 6,
}

/**
 * ViewEmptyState renders whichever of the five screens applies, or nothing when
 * there is data to show.
 *
 * Order matters, and it is the order of how much each cause overrides the next:
 *
 *  1. a refused read, because it is the only cause the reader has to act on and
 *     the only one that will never resolve on its own
 *  2. loading, because an empty list during a cold cache is not an answer
 *  3. a missing CRD, because it is a statement about the cluster rather than
 *     about the user's apps
 *  4. genuinely nothing found
 *
 * The refused read is checked before loading on purpose. It arrives with
 * synced=false, so testing loading first would show a skeleton that never
 * resolves, telling the reader the dashboard is still reading something it has
 * already given up on.
 */
export function ViewEmptyState({
  readiness,
  rowCount,
  view,
}: {
  readiness: Readiness | undefined
  rowCount: number
  view: ViewId
}) {
  if (!readiness) {
    return <SkeletonRows rows={6} columns={skeletonColumns[view]} />
  }

  if ((readiness.unavailable?.length ?? 0) > 0) {
    return <ReadRefused unavailable={readiness.unavailable ?? []} />
  }

  if (!readiness.synced) {
    return (
      <>
        <div className="px-4 pt-4 text-xs text-ink-muted">
          Reading your cluster. This is the initial list, not a refresh loop.
        </div>
        <SkeletonRows rows={6} columns={skeletonColumns[view]} />
      </>
    )
  }

  if (!readiness.crdsInstalled) {
    return <OperatorNotInstalled missing={readiness.missingCRDs ?? []} view={view} />
  }

  if (rowCount === 0) {
    switch (view) {
      case 'apps':
        return <NoWorkloadsAtAll />
      case 'incidents':
        return <NoIncidents />
      case 'remediations':
        return <NoRemediations />
      case 'cluster':
        return <NoNodes />
    }
  }

  return null
}

/**
 * The read was refused. This is the one empty state that is about the reader's
 * permissions rather than about their cluster, and the one that will not fix
 * itself.
 *
 * It exists because a rejected watch used to leave every affected view on a
 * loading skeleton forever. The most likely cause is RBAC, and a
 * namespace-scoped kubeconfig cannot list Nodes at all, so the Cluster view
 * reaches this by design rather than by accident.
 */
function ReadRefused({ unavailable }: { unavailable: UnavailableResource[] }) {
  return (
    <EmptyState
      icon={<KeyRound className="size-6" />}
      title={`Dorgu could not read ${unavailable.map((entry) => entry.resource).join(', ')}`}
    >
      <p>
        The dashboard has exactly your permissions and holds no credentials of its own, so this is
        what your kubeconfig user is allowed to do rather than a limit Dorgu imposes.
      </p>
      <ul className="mt-3 space-y-1 text-left">
        {unavailable.map((entry) => (
          <li key={entry.resource}>
            <code className="font-mono text-ink">{entry.resource}</code>: {entry.reason}
          </li>
        ))}
      </ul>
      <p className="mt-3">
        If your user is scoped to one namespace, restart with{' '}
        <code className="font-mono text-ink">--namespace</code>. Cluster-wide resources such as
        Nodes cannot be read at all from a namespace-scoped user.
      </p>
    </EmptyState>
  )
}

/**
 * "The operator is not installed" and "you have no apps" are different facts and
 * this is the one that is about the cluster. For the Apps view it is not even
 * fatal: every Deployment is simply unmonitored, which makes the whole screen the
 * onboarding prompt.
 */
function OperatorNotInstalled({ missing, view }: { missing: string[]; view: ViewId }) {
  return (
    <EmptyState
      icon={<ServerOff className="size-6" />}
      title="The Dorgu operator is not installed on this cluster"
    >
      <p>
        {missing.length > 0 ? (
          <>
            This cluster does not serve{' '}
            <code className="font-mono text-ink">{missing.join(', ')}</code>.
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

/**
 * What a missing operator costs each view. Apps and Cluster keep working, because
 * both are computed from live objects; Incidents and Remediations have nothing to
 * show, which is a statement about the cluster rather than good news.
 */
const operatorMissingConsequence: Record<ViewId, string> = {
  apps: 'Every Deployment below is therefore unmonitored: Dorgu is not watching any of them.',
  incidents:
    'Nothing is detecting incidents yet, so this is a statement about the cluster rather than good news about your apps.',
  remediations:
    'Nothing is proposing remediations, so this is a statement about the cluster rather than a sign that nothing needs fixing.',
  cluster:
    'The nodes, capacity and saturation on this screen are read from your cluster directly and still work. The cluster name, the environment and the add-on list come from the operator and are therefore empty.',
}

function NoWorkloadsAtAll() {
  return (
    <EmptyState icon={<PackageOpen className="size-6" />} title="No Deployments in scope">
      <p>
        There are no Deployments outside the cluster add-on namespaces, and no
        ApplicationPersonas. Deploy something, or point the dashboard at a namespace with{' '}
        <code className="font-mono text-ink">--namespace</code>.
      </p>
    </EmptyState>
  )
}

/**
 * The one genuinely good empty state, and it still says what it is based on.
 * "No incidents" only means something if you know what was being watched.
 */
function NoIncidents() {
  return (
    <EmptyState icon={<AlertTriangle className="size-6" />} title="No incidents recorded">
      <p>
        The operator is installed and has not recorded an IncidentMemory. Detection is on by
        default; AI diagnosis is opt-in and costs money, so an incident may appear here with no
        root cause attached.
      </p>
    </EmptyState>
  )
}

/**
 * A genuinely good empty state, and it still says what it is based on.
 *
 * "No remediations" only means something if you know a remediation follows an
 * incident, so the sentence names the chain rather than leaving the reader to
 * wonder whether the feature works.
 */
function NoRemediations() {
  return (
    <EmptyState icon={<ShieldCheck className="size-6" />} title="No remediations proposed">
      <p>
        A remediation follows a diagnosed incident, so an empty list here usually means nothing has
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

/**
 * No nodes and no refusal is a strange cluster rather than an error, so this says
 * what was actually read instead of guessing at a cause.
 */
function NoNodes() {
  return (
    <EmptyState icon={<Sparkles className="size-6" />} title="This cluster reports no nodes">
      <p>
        The node list was read successfully and is empty. Without nodes there is no allocatable pool,
        so there is nothing for saturation to be a share of.
      </p>
    </EmptyState>
  )
}
