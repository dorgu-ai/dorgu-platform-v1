import { useQuery } from '@tanstack/react-query'
import { User } from 'lucide-react'

import { metaQuery } from '@/lib/api'
import type { StreamStatus } from '@/lib/stream'
import { clockTime } from '@/lib/utils'

import { Dot } from './status'
import { ThemeToggle } from './theme-toggle'
import { Tooltip } from './ui/tooltip'

/**
 * The top bar, 48 px, section 4: the cluster context, the authenticated identity,
 * the live or stale indicator, and the theme toggle.
 */
export function TopBar({ stream }: { stream: StreamStatus }) {
  return (
    <header className="flex h-12 shrink-0 items-center gap-4 border-b border-line bg-surface px-6">
      <ClusterContext />
      <div className="ml-auto flex items-center gap-4">
        <Identity />
        <LiveIndicator stream={stream} />
        <ThemeToggle />
      </div>
    </header>
  )
}

/**
 * Which cluster this is. Not decoration: a dashboard that does not say which
 * cluster it describes is one you cannot trust when you have four of them.
 */
function ClusterContext() {
  const { data: meta } = useQuery(metaQuery)
  if (!meta) return null

  const label = meta.cluster.context || (meta.cluster.inCluster ? 'in-cluster' : meta.cluster.server)
  const detail = [
    `API server: ${meta.cluster.server}`,
    meta.cluster.namespace ? `Scoped to namespace: ${meta.cluster.namespace}` : 'All namespaces',
    meta.operatorInstalled
      ? 'Dorgu operator CRDs are installed.'
      : `Missing dorgu.io resources: ${(meta.missingCRDs ?? []).join(', ')}`,
  ].join('\n')

  return (
    <Tooltip content={<span className="whitespace-pre-line">{detail}</span>}>
      <span className="flex cursor-default items-center gap-2 font-mono text-code text-ink" data-testid="cluster-context">
        <Dot tone={meta.operatorInstalled ? 'ok' : 'warn'} />
        <span className="sr-only">
          {meta.operatorInstalled ? 'Operator installed. ' : 'Operator not installed. '}Cluster:
        </span>
        {label}
        {meta.cluster.namespace && <span className="text-ink-muted">/{meta.cluster.namespace}</span>}
      </span>
    </Tooltip>
  )
}

/** The identity the cluster authenticated this kubeconfig as: /api/v1/meta, from SelfSubjectReview. */
function Identity() {
  const { data: meta } = useQuery(metaQuery)
  if (!meta) return null
  const identity = meta.identity
  const detail = identity.available
    ? [
        `Authenticated as ${identity.username}`,
        ...(identity.groups?.length ? [`Groups: ${identity.groups.join(', ')}`] : []),
      ].join('\n')
    : `Identity not available: ${identity.reason ?? 'unknown'}`

  return (
    <Tooltip content={<span className="whitespace-pre-line">{detail}</span>}>
      <span className="flex cursor-default items-center gap-1.5 text-small text-ink-muted" data-testid="identity">
        <User className="size-3.5" aria-hidden="true" />
        {identity.available ? (
          <span className="font-mono text-ink">{identity.username}</span>
        ) : (
          'identity unavailable'
        )}
      </span>
    </Tooltip>
  )
}

/**
 * "Live" with an ok dot, or "Stale since 14:02" in warn when the SSE stream
 * drops. Never "live" on faith: what is on screen is the last state received, and
 * the indicator says when that was.
 */
export function LiveIndicator({ stream }: { stream: StreamStatus }) {
  if (stream.state === 'live') {
    return (
      <Tooltip content="Connected. Changes in the cluster appear here on their own. There is no refresh button because there is nothing to refresh.">
        <span className="flex cursor-default items-center gap-1.5 text-small text-ink-muted" data-testid="live-indicator" data-state="live">
          <Dot tone="ok" />
          Live
        </span>
      </Tooltip>
    )
  }

  const since = stream.lastLiveAt
  const text = since ? `Stale since ${clockTime(since)}` : 'Connecting'
  return (
    <Tooltip
      content={
        stream.state === 'offline'
          ? 'The event stream is closed. What is on screen is the last state received. Check that the dashboard process is still running.'
          : 'Reconnecting. The browser retries on its own and the server sends a fresh snapshot when it does, so this recovers without a reload.'
      }
    >
      <span className="flex cursor-default items-center gap-1.5 text-small text-warn" data-testid="live-indicator" data-state={stream.state}>
        <Dot tone="warn" />
        {text}
      </span>
    </Tooltip>
  )
}
