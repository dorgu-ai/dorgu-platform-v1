import { AlertTriangle, PackageOpen, ServerOff } from 'lucide-react'
import type * as React from 'react'

import type { Readiness } from '@/lib/types'

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

/**
 * ViewEmptyState renders whichever of the four screens applies, or nothing when
 * there is data to show.
 *
 * Order matters. Loading beats everything, because an empty list during a cold
 * cache is not an answer. A missing CRD beats "nothing found", because it is a
 * statement about the cluster rather than about the user's apps.
 */
export function ViewEmptyState({
  readiness,
  rowCount,
  view,
}: {
  readiness: Readiness | undefined
  rowCount: number
  view: 'apps' | 'incidents'
}) {
  if (!readiness) {
    return <SkeletonRows rows={6} columns={view === 'apps' ? 6 : 5} />
  }

  if (!readiness.synced) {
    return (
      <>
        <div className="px-4 pt-4 text-xs text-ink-muted">
          Reading your cluster. This is the initial list, not a refresh loop.
        </div>
        <SkeletonRows rows={6} columns={view === 'apps' ? 6 : 5} />
      </>
    )
  }

  if (!readiness.crdsInstalled) {
    return <OperatorNotInstalled missing={readiness.missingCRDs ?? []} view={view} />
  }

  if (rowCount === 0) {
    return view === 'apps' ? <NoWorkloadsAtAll /> : <NoIncidents />
  }

  return null
}

/**
 * "The operator is not installed" and "you have no apps" are different facts and
 * this is the one that is about the cluster. For the Apps view it is not even
 * fatal: every Deployment is simply unmonitored, which makes the whole screen the
 * onboarding prompt.
 */
function OperatorNotInstalled({ missing, view }: { missing: string[]; view: 'apps' | 'incidents' }) {
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
        {view === 'incidents'
          ? 'Nothing is detecting incidents yet, so this is a statement about the cluster rather than good news about your apps.'
          : 'Every Deployment below is therefore unmonitored: Dorgu is not watching any of them.'}
      </p>
      <div className="mt-4">
        <CopyCommand command="dorgu install" label="Install the operator" />
      </div>
    </EmptyState>
  )
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
