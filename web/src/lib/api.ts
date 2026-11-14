import { queryOptions } from '@tanstack/react-query'

import type {
  AppsPayload,
  ClusterPayload,
  IncidentsPayload,
  MemoryPayload,
  Meta,
  OverviewPayload,
  RemediationsPayload,
  Topic,
} from './types'

export const API_BASE = '/api/v1'

/** An API failure that carries enough to say what went wrong on screen. */
export class ApiError extends Error {
  readonly status: number
  readonly path: string

  constructor(status: number, path: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.path = path
  }
}

async function get<T>(path: string): Promise<T> {
  let response: Response
  try {
    response = await fetch(`${API_BASE}${path}`, {
      headers: { Accept: 'application/json' },
    })
  } catch {
    // The dashboard is on localhost, so a network failure here almost always
    // means the process stopped. Saying that is more useful than "failed to
    // fetch".
    throw new ApiError(0, path, 'The dashboard server is not responding. Is it still running?')
  }

  if (response.status === 401) {
    // The session cookie is missing or the dashboard was restarted, which
    // issues a new token. The fix is the link the dashboard printed.
    throw new ApiError(
      401,
      path,
      'This session is not authorised. Open the link that `dorgu dashboard` printed in your terminal; it carries a new token each time the dashboard starts.',
    )
  }
  if (!response.ok) {
    const body = await response.text().catch(() => '')
    throw new ApiError(
      response.status,
      path,
      body.trim() || `${response.status} ${response.statusText}`,
    )
  }
  return (await response.json()) as T
}

/**
 * Query options shared by the loaders and the SSE stream.
 *
 * Nothing polls. `staleTime: Infinity` and the disabled refetch triggers are not
 * a caching tweak: the server pushes a complete snapshot of each view on connect
 * and on every change, so a refetch could only ever return what the cache
 * already holds. If a refetch were ever needed, the stream would be broken.
 */
const live = {
  staleTime: Infinity,
  gcTime: Infinity,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
  refetchOnMount: false,
  refetchInterval: false,
} as const

export const appsQuery = queryOptions({
  queryKey: ['apps'] as const,
  queryFn: () => get<AppsPayload>('/apps'),
  ...live,
})

export const incidentsQuery = queryOptions({
  queryKey: ['incidents'] as const,
  queryFn: () => get<IncidentsPayload>('/incidents'),
  ...live,
})

export const remediationsQuery = queryOptions({
  queryKey: ['remediations'] as const,
  queryFn: () => get<RemediationsPayload>('/remediations'),
  ...live,
})

export const clusterQuery = queryOptions({
  queryKey: ['cluster'] as const,
  queryFn: () => get<ClusterPayload>('/cluster'),
  ...live,
})

export const memoryQuery = queryOptions({
  queryKey: ['memory'] as const,
  queryFn: () => get<MemoryPayload>('/memory'),
  ...live,
})

/** The window the SSE stream keeps current. Other windows are fetched by overviewWindowQuery. */
export const DEFAULT_WINDOW = '7d'

/** The windows the Overview offers. Any `since` the server parses also works by URL. */
export const WINDOWS = ['24h', '7d', '30d'] as const

export const overviewQuery = queryOptions({
  queryKey: ['overview'] as const,
  queryFn: () => get<OverviewPayload>('/overview'),
  ...live,
  // The stream pushes on a change, and the window also slides with the clock, so
  // a quiet cluster still gets a fresh report once a minute.
  refetchInterval: 60_000,
})

/**
 * Any other window. The stream pushes only the default, so these refetch on a
 * timer: the window slides with the clock, and a report for "24h" that was true a
 * minute ago is not the report for now.
 */
export const overviewWindowQuery = (since: string) =>
  queryOptions({
    queryKey: ['overview-window', since] as const,
    queryFn: () => get<OverviewPayload>(`/overview?since=${encodeURIComponent(since)}`),
    refetchInterval: 30_000,
    staleTime: 15_000,
  })

export const metaQuery = queryOptions({
  queryKey: ['meta'] as const,
  queryFn: () => get<Meta>('/meta'),
  ...live,
})

/** The query key an SSE topic writes into. Keeps the two in one place. */
export function queryKeyForTopic(topic: Topic): readonly [Topic] {
  return [topic] as const
}
