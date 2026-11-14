import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChevronsLeft, ChevronsRight, Lock } from 'lucide-react'
import { useState } from 'react'

import { metaQuery } from '@/lib/api'
import { formatVersion, NAV } from '@/lib/nav'
import type { ViewStatus } from '@/lib/types'
import { cn } from '@/lib/utils'

import { Button } from './ui/button'
import { Tooltip } from './ui/tooltip'

const STORAGE_KEY = 'dorgu-sidebar'

function readCollapsed(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'collapsed'
  } catch {
    return false
  }
}

export function Sidebar() {
  const { data: meta } = useQuery(metaQuery)
  const [collapsed, setCollapsed] = useState(readCollapsed)
  const statuses = new Map<string, ViewStatus>(meta?.views.map((view) => [view.id, view]) ?? [])

  const toggle = () => {
    setCollapsed((value) => {
      try {
        localStorage.setItem(STORAGE_KEY, value ? 'expanded' : 'collapsed')
      } catch {
        // The sidebar still collapses for this page load.
      }
      return !value
    })
  }

  return (
    <aside
      className={cn(
        'flex shrink-0 flex-col border-r border-line bg-surface-sunken transition-[width]',
        // Below md the sidebar is always icons: the content needs the room.
        collapsed ? 'w-14' : 'w-14 md:w-[232px]',
      )}
      aria-label="Primary"
    >
      <div className="flex h-12 shrink-0 items-center gap-2 border-b border-line px-4">
        <span className="text-h3 font-semibold tracking-tight text-ink">dorgu</span>
        <span className={cn('text-small text-ink-muted', collapsed ? 'hidden' : 'max-md:hidden')}>
          dashboard
        </span>
      </div>

      <nav aria-label="Views" className="flex-1 space-y-0.5 overflow-y-auto p-2">
        {NAV.map((item) => {
          const status = statuses.get(item.id)
          // Until meta arrives every item is assumed available; the server only
          // ever says "no" to the ones it cannot render.
          const unavailable = item.to === undefined || (status !== undefined && !status.available)
          const Icon = item.icon
          const label = status?.label || item.title
          const textClass = collapsed ? 'hidden' : 'max-md:hidden'

          if (unavailable) {
            const reason = status?.reason || 'Not in this release.'
            return (
              <Tooltip key={item.id} content={reason} side="right">
                <div
                  aria-disabled="true"
                  data-nav={item.id}
                  className="flex cursor-not-allowed items-start gap-2.5 rounded-ctl px-2.5 py-1.5 text-ink-muted"
                >
                  <Icon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                  <span className={cn('min-w-0', textClass)}>
                    <span className="block text-body">{label}</span>
                    <span className="block text-caption text-ink-muted">{reason}</span>
                  </span>
                  <span className="sr-only">Disabled. {reason}</span>
                </div>
              </Tooltip>
            )
          }

          return (
            <Tooltip key={item.id} content={collapsed ? label : status?.limitation} side="right">
              <Link
                to={item.to}
                data-nav={item.id}
                aria-label={label}
                className={cn(
                  'relative flex items-center gap-2.5 rounded-ctl px-2.5 py-1.5 text-body text-ink-muted hover:bg-surface-hover hover:text-ink',
                )}
                activeProps={{
                  className:
                    'bg-accent-soft text-ink before:absolute before:inset-y-1 before:left-0 before:w-0.5 before:rounded-full before:bg-accent',
                  'aria-current': 'page',
                }}
              >
                <Icon className="size-4 shrink-0" aria-hidden="true" />
                <span className={textClass}>{label}</span>
              </Link>
            </Tooltip>
          )
        })}
      </nav>

      <div className="shrink-0 space-y-2 border-t border-line p-3">
        <Tooltip content="The dashboard can do exactly what you can do, and nothing more. It uses your kubeconfig, holds no credentials of its own, binds to localhost, and in this release writes nothing to your cluster.">
          <p
            className={cn(
              'flex cursor-default items-start gap-1.5 text-small text-ink-muted',
              collapsed ? 'justify-center' : 'max-md:justify-center',
            )}
          >
            <Lock className="mt-0.5 size-3 shrink-0" aria-hidden="true" />
            <span className={textHidden(collapsed)}>Read-only. Uses your kubeconfig, holds no credentials.</span>
          </p>
        </Tooltip>
        <div className="flex items-center justify-between">
          <span className={cn('font-mono text-small text-ink-muted', textHidden(collapsed))}>
            {meta?.version ? formatVersion(meta.version) : ''}
          </span>
          <Button
            variant="ghost"
            size="icon"
            onClick={toggle}
            className="max-md:hidden"
            aria-label={collapsed ? 'Expand the sidebar' : 'Collapse the sidebar'}
          >
            {collapsed ? <ChevronsRight className="size-3.5" /> : <ChevronsLeft className="size-3.5" />}
          </Button>
        </div>
      </div>
    </aside>
  )
}

function textHidden(collapsed: boolean): string {
  return collapsed ? 'hidden' : 'max-md:hidden'
}
