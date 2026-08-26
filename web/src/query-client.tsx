import { QueryClient } from '@tanstack/react-query'

/**
 * One client, created at module scope so the router's loaders and the SSE stream
 * write into the same cache.
 *
 * The defaults are deliberately inert. This app has exactly one source of
 * freshness, which is the event stream, and every retry or interval Query could
 * add on top would be a second one racing it.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: Infinity,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
      // One retry, for the case where the first fetch races the server's own
      // startup. Beyond that a failure is real and should be shown, not hidden
      // behind a retry loop that looks like a hang.
      retry: 1,
    },
  },
})
