import { Outlet } from '@tanstack/react-router'

import { useEventStream } from '@/lib/stream'

import { AppShell } from './app-shell'
import { ErrorState } from './error-state'
import { SkeletonRows } from './ui/skeleton'
import { TooltipProvider } from './ui/tooltip'

/**
 * The shell every route renders inside.
 *
 * The single SSE connection for the whole app is opened here, once. Every view
 * reads the Query cache it fills, so nothing polls (the Overview's sliding window
 * is the one deliberate exception) and no view owns a subscription that could
 * leak on navigation.
 */
export function RootLayout() {
  const stream = useEventStream()

  return (
    <TooltipProvider delayDuration={200}>
      <AppShell stream={stream}>
        <Outlet />
      </AppShell>
    </TooltipProvider>
  )
}

/**
 * Shown for a path the client-side router does not know.
 *
 * The Go server serves index.html for any unclaimed path, so a stale deep link
 * lands here rather than on a bare 404, and this says which views this build
 * actually has.
 */
export function NotFound() {
  return (
    <div className="px-6 py-16 text-center text-small text-ink-muted">
      <h1 className="mb-2 text-h2 font-semibold text-ink">No such view</h1>
      <p>
        This build has Overview, Incidents, Proposals, Apps, Memory and Cluster. Pick one from the
        sidebar.
      </p>
    </div>
  )
}

/** A route that failed to load: the loader threw, usually a 401 or a server that went away. */
export function RouteError({ error }: { error: Error }) {
  return <ErrorState title="Could not load this view" error={error} />
}

/**
 * Shown while a route's loader is still reading. A route renders only once its
 * data is in the cache, so this is the loading state of every view: skeleton rows
 * in the shape of the table that is coming, and a sentence, never a spinner and
 * never a zero.
 */
export function RoutePending() {
  return (
    <div role="status" aria-busy="true" data-testid="route-pending">
      <p className="px-6 pt-5 text-small text-ink-muted">Reading your cluster.</p>
      <SkeletonRows rows={6} columns={5} />
    </div>
  )
}
