import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Lock, Moon, Sun } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { metaQuery } from '@/lib/api'
import type { StreamState } from '@/lib/stream'
import { cn } from '@/lib/utils'

import { Dot } from './status'
import { Button } from './ui/button'
import { Tooltip } from './ui/tooltip'

/**
 * The four views from the plan, all of them built.
 *
 * They are still driven by the server's own list rather than by this array
 * alone: `available` and `limitation` come from the meta endpoint, so a view that
 * is disabled or limited in a future build cannot drift from what the build
 * actually does. A view the server reports as unavailable renders disabled with
 * its reason on hover, which is how the two that were gated appeared before this
 * release.
 */
const NAV = [
  { id: 'apps', label: 'Apps', to: '/apps' },
  { id: 'incidents', label: 'Incidents', to: '/incidents' },
  { id: 'remediations', label: 'Remediations', to: '/remediations' },
  { id: 'cluster', label: 'Cluster', to: '/cluster' },
] as const

export function AppShell({
  streamState,
  children,
}: {
  streamState: StreamState
  children: React.ReactNode
}) {
  const { data: meta } = useQuery(metaQuery)
  const viewReasons = new Map(meta?.views.map((view) => [view.id, view]) ?? [])

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="shrink-0 border-b border-line bg-surface-raised">
        <div className="flex items-center gap-4 px-4 py-2.5">
          <div className="flex items-baseline gap-2">
            <span className="text-sm font-semibold tracking-tight text-ink">dorgu</span>
            <span className="text-[11px] text-ink-faint">dashboard</span>
          </div>

          <nav className="flex items-center gap-1" aria-label="Views">
            {NAV.map((item) => {
              const status = viewReasons.get(item.id)

              // The server decides. A build that gates a view again gets the
              // disabled treatment with no frontend change.
              if (status && !status.available) {
                return (
                  <Tooltip key={item.id} content={status.reason || 'Not built in this release.'}>
                    <span
                      aria-disabled="true"
                      className="cursor-not-allowed rounded-md px-2 py-1 text-xs text-ink-faint/60 line-through decoration-ink-faint/40"
                    >
                      {item.label}
                    </span>
                  </Tooltip>
                )
              }

              // A view that renders and still cannot do something carries its
              // limitation on hover. It is not marked in the nav: the whole
              // dashboard is read-only and the footer says so, so a badge on
              // every entry would repeat the footer rather than add to it.
              return (
                <Tooltip key={item.id} content={status?.limitation}>
                  <Link
                    to={item.to}
                    className="rounded-md px-2 py-1 text-xs text-ink-muted hover:bg-surface-sunken hover:text-ink"
                    activeProps={{ className: 'bg-surface-sunken text-ink' }}
                  >
                    {item.label}
                  </Link>
                </Tooltip>
              )
            })}
          </nav>

          <div className="ml-auto flex items-center gap-3">
            <ClusterBadge />
            <StreamBadge state={streamState} />
            <TrustBadge />
            <ThemeToggle />
          </div>
        </div>
      </header>

      <main className="flex min-h-0 flex-1 flex-col">{children}</main>

      <footer className="flex shrink-0 items-center gap-3 border-t border-line px-4 py-1.5 text-[11px] text-ink-faint">
        <span>Read-only. Uses your kubeconfig, holds no credentials, binds to localhost.</span>
        {meta?.version && <span className="ml-auto font-mono">{formatVersion(meta.version)}</span>}
      </footer>
    </div>
  )
}

/**
 * Prefixes a release version with "v" and leaves anything else alone. An
 * unstamped build reports "dev", and "vdev" reads like a typo.
 */
function formatVersion(version: string): string {
  return /^\d/.test(version) ? `v${version}` : version
}

/**
 * Which cluster this is.
 *
 * Not decoration. A dashboard that does not say which cluster it is describing is
 * one you cannot trust when you have four of them, and the context name is the
 * name the reader already thinks in.
 */
function ClusterBadge() {
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
      <span className="flex cursor-default items-center gap-1.5 font-mono text-[11px] text-ink-muted">
        <Dot tone={meta.operatorInstalled ? 'ok' : 'warn'} />
        {label}
        {meta.cluster.namespace && <span className="text-ink-faint">/{meta.cluster.namespace}</span>}
      </span>
    </Tooltip>
  )
}

const streamCopy: Record<StreamState, { label: string; tone: 'ok' | 'warn' | 'danger'; note: string }> =
  {
    live: {
      label: 'live',
      tone: 'ok',
      note: 'Connected. Changes in the cluster appear here on their own. There is no refresh button because there is nothing to refresh.',
    },
    connecting: {
      label: 'connecting',
      tone: 'warn',
      note: 'Reconnecting. The browser retries on its own and the server sends a fresh snapshot when it does, so this recovers without a reload.',
    },
    offline: {
      label: 'offline',
      tone: 'danger',
      note: 'The event stream is closed. What is on screen is the last state received. Check that the dashboard process is still running.',
    },
  }

function StreamBadge({ state }: { state: StreamState }) {
  const copy = streamCopy[state]
  return (
    <Tooltip content={copy.note}>
      <span className="flex cursor-default items-center gap-1.5 text-[11px] text-ink-muted">
        <Dot tone={copy.tone} pulse={state !== 'live'} />
        {copy.label}
      </span>
    </Tooltip>
  )
}

/** The trust line, one click from every screen rather than only in the README. */
function TrustBadge() {
  return (
    <Tooltip content="The dashboard can do exactly what you can do, and nothing more. It uses your kubeconfig, holds no credentials of its own, needs no permissions of its own, opens no port beyond localhost, and writes nothing to your cluster.">
      <span className="flex cursor-default items-center gap-1 text-[11px] text-ink-faint">
        <Lock className="size-3" />
        read-only
      </span>
    </Tooltip>
  )
}

/**
 * Theme toggle. Dark is the default because this is a tool that gets opened
 * beside a terminal, and the preference is remembered.
 */
function ThemeToggle() {
  const [dark, setDark] = useState(() => {
    const stored = localStorage.getItem('dorgu-theme')
    if (stored) return stored === 'dark'
    return !window.matchMedia('(prefers-color-scheme: light)').matches
  })

  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    localStorage.setItem('dorgu-theme', dark ? 'dark' : 'light')
  }, [dark])

  const toggle = useCallback(() => setDark((value) => !value), [])

  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={toggle}
      aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
    >
      {dark ? <Sun className="size-3.5" /> : <Moon className="size-3.5" />}
    </Button>
  )
}

/** A row of counts above a table, so the header cannot disagree with the rows. */
export function SummaryBar({
  items,
  trailing,
}: {
  items: { label: string; value: number | string; tone?: 'ok' | 'warn' | 'danger' | 'info' }[]
  trailing?: React.ReactNode
}) {
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-x-5 gap-y-2 border-b border-line px-4 py-2.5">
      {items.map((item) => (
        <span key={item.label} className="flex items-baseline gap-1.5 text-xs">
          <span
            className={cn(
              'font-semibold',
              item.tone === 'danger' && 'text-danger',
              item.tone === 'warn' && 'text-warn',
              item.tone === 'ok' && 'text-ok',
              item.tone === 'info' && 'text-info',
              !item.tone && 'text-ink',
            )}
          >
            {item.value}
          </span>
          <span className="text-ink-faint">{item.label}</span>
        </span>
      ))}
      {trailing && <div className="ml-auto">{trailing}</div>}
    </div>
  )
}
