import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'

import { API_BASE, queryKeyForTopic } from './api'
import { TOPICS } from './types'

/**
 * How the browser sees the stream.
 *
 * 'connecting' covers both the first connect and every automatic reconnect,
 * because the two are the same thing to a reader: the screen may be a moment
 * behind. The state is shown in the header rather than hidden, so a stale page
 * is visibly stale instead of quietly wrong.
 */
export type StreamState = 'connecting' | 'live' | 'offline'

/**
 * useEventStream opens the one SSE connection the app has and feeds each pushed
 * snapshot straight into the Query cache.
 *
 * # Why the server pushes whole snapshots
 *
 * Each event carries the complete payload for one view, so applying it is a
 * `setQueryData` with no merge, no ordering assumption and no invalidation. That
 * removes the class of bug where a delta is applied to the wrong base and the UI
 * ends up plausibly wrong, which is worse than being visibly stale.
 *
 * # Why there is no refresh anywhere
 *
 * The browser reconnects an SSE stream on its own, and the server answers every
 * new connection with a fresh snapshot of every view. So a laptop that slept, a
 * VPN that dropped and a dashboard process that restarted all recover without
 * anything asked of the user. If a reload is ever needed, this wiring is broken.
 */
export function useEventStream(): StreamState {
  const queryClient = useQueryClient()
  const [state, setState] = useState<StreamState>('connecting')

  useEffect(() => {
    const source = new EventSource(`${API_BASE}/stream`)

    const listeners = TOPICS.map((topic) => {
      const handler = (event: MessageEvent<string>) => {
        try {
          queryClient.setQueryData(queryKeyForTopic(topic), JSON.parse(event.data))
          setState('live')
        } catch {
          // A payload this process cannot parse means the server and the SPA
          // disagree about the wire format, which a reconnect will not fix.
          // Leave the last good data on screen and report not-live.
          setState('offline')
        }
      }
      source.addEventListener(topic, handler as EventListener)
      return { topic, handler }
    })

    source.onopen = () => setState('live')
    source.onerror = () => {
      // EventSource retries by itself using the server's `retry` hint, so this
      // is 'connecting' rather than a terminal error. Reporting it as fatal
      // would put a reload button on a screen that is about to fix itself.
      setState(source.readyState === EventSource.CLOSED ? 'offline' : 'connecting')
    }

    return () => {
      for (const { topic, handler } of listeners) {
        source.removeEventListener(topic, handler as EventListener)
      }
      source.close()
    }
  }, [queryClient])

  return state
}
