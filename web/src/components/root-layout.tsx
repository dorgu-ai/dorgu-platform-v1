import { Outlet } from '@tanstack/react-router'

import { AppShell } from './app-shell'
import { TooltipProvider } from './ui/tooltip'
import { useEventStream } from '@/lib/stream'

/**
 * The shell every route renders inside.
 *
 * The single SSE connection for the whole app is opened here, once. Every view
 * reads the Query cache it fills, so nothing polls and no view owns a
 * subscription that could leak on navigation.
 */
export function RootLayout() {
  const streamState = useEventStream()

  return (
    <TooltipProvider delayDuration={200}>
      <AppShell streamState={streamState}>
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
    <div className="px-6 py-16 text-center text-xs text-ink-muted">
      <p className="mb-2 text-sm font-semibold text-ink">No such view</p>
      <p>
        This build has Apps and Incidents. Remediations and Cluster are not built yet, and the
        header says why.
      </p>
    </div>
  )
}
