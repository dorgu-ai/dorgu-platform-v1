import type * as React from 'react'

import type { StreamStatus } from '@/lib/stream'
import { cn } from '@/lib/utils'

import { Sidebar } from './sidebar'
import { TopBar } from './topbar'

/**
 * The shell, section 4: a fixed left sidebar, a 48 px top bar with the cluster
 * context, the authenticated identity, the live indicator and the theme toggle,
 * and fluid content with 24 px gutters.
 */
export function AppShell({
  stream,
  children,
}: {
  stream: StreamStatus
  children: React.ReactNode
}) {
  return (
    <div className="flex h-full min-h-0">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded-ctl focus:bg-surface focus:px-3 focus:py-1.5 focus:text-small focus:text-ink"
      >
        Skip to content
      </a>
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar stream={stream} />
        <main id="main" tabIndex={-1} className="flex min-h-0 flex-1 flex-col outline-none">
          {children}
        </main>
      </div>
    </div>
  )
}

/**
 * The page title and one sentence on what the page is. The title is the page's
 * only h1: headings below it step down in order.
 */
export function PageHeader({
  title,
  description,
  trailing,
}: {
  title: string
  description?: React.ReactNode
  trailing?: React.ReactNode
}) {
  return (
    <div className="flex shrink-0 flex-wrap items-end justify-between gap-x-6 gap-y-2 px-6 pt-5 pb-3">
      <div className="min-w-0">
        <h1 className="text-h1 font-semibold text-ink">{title}</h1>
        {description && <p className="mt-0.5 max-w-[720px] text-small text-ink-muted">{description}</p>}
      </div>
      {trailing && <div className="shrink-0">{trailing}</div>}
    </div>
  )
}

/** A row of counts above a table, so the header cannot disagree with the rows. */
export function SummaryBar({
  items,
  trailing,
}: {
  items: { label: string; value: number | string; tone?: 'ok' | 'warn' | 'danger' | 'neutral' }[]
  trailing?: React.ReactNode
}) {
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-x-5 gap-y-2 border-y border-line bg-surface px-6 py-2.5">
      {items.map((item) => (
        <span key={item.label} className="flex items-baseline gap-1.5 text-small">
          <span
            className={cn(
              'font-semibold',
              item.tone === 'danger' && 'text-danger',
              item.tone === 'warn' && 'text-warn',
              item.tone === 'ok' && 'text-ok',
              (!item.tone || item.tone === 'neutral') && 'text-ink',
            )}
          >
            {item.value}
          </span>
          <span className="text-ink-muted">{item.label}</span>
        </span>
      ))}
      {trailing && <div className="ml-auto">{trailing}</div>}
    </div>
  )
}
