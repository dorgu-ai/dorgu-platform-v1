import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  redirect,
} from '@tanstack/react-router'

import { NotFound, RootLayout, RoutePending, RouteError } from './components/root-layout'
import {
  appsQuery,
  clusterQuery,
  incidentsQuery,
  memoryQuery,
  metaQuery,
  overviewQuery,
  remediationsQuery,
} from './lib/api'
import { queryClient } from './query-client'

/**
 * Routes are declared in code rather than generated from the filesystem.
 *
 * The route table is four entries and will stay small; the file-based plugin
 * would add a codegen step to the build for no benefit at this size, and a build
 * step that can be skipped is a build step that will be.
 *
 * # Every view is a separate chunk
 *
 * The whole app was one 487 KB file, so opening Apps downloaded the remediation
 * diff renderer and the node table with it. Each view is now behind a dynamic
 * import.
 *
 * The loaders still run in parallel with the chunk download rather than after
 * it: `lazyRouteComponent` defers only the component, and TanStack Router starts
 * `loader` at the same time. So splitting costs nothing on the critical path,
 * and `defaultPreload: 'intent'` fetches the next view's chunk on hover.
 */
const rootRoute = createRootRoute({
  component: RootLayout,
  // Meta is loaded once here rather than per route: the header needs it on every
  // screen, and the SSE stream keeps it current from then on.
  loader: () => queryClient.ensureQueryData(metaQuery),
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  // Overview is the landing view. On a cluster with no operator it says so and
  // points at Apps, which still works: every Deployment shows up as unwatched.
  beforeLoad: () => {
    // TanStack Router's redirect() returns a control-flow object that is meant
    // to be thrown; it is not an Error and does not need to be.
    // eslint-disable-next-line @typescript-eslint/only-throw-error
    throw redirect({ to: '/overview', replace: true })
  },
})

/** Optional string search params, kept only when they are strings. */
function stringParam(search: Record<string, unknown>, key: string): { [k: string]: string } {
  const value = search[key]
  return typeof value === 'string' && value !== '' ? { [key]: value } : {}
}

const overviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/overview',
  validateSearch: (search: Record<string, unknown>) => stringParam(search, 'since') as { since?: string },
  loader: () => queryClient.ensureQueryData(overviewQuery),
  component: lazyRouteComponent(() => import('./routes/overview'), 'OverviewRoute'),
})

const memoryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/memory',
  validateSearch: (search: Record<string, unknown>) => stringParam(search, 'record') as { record?: string },
  loader: () => queryClient.ensureQueryData(memoryQuery),
  component: lazyRouteComponent(() => import('./routes/memory'), 'MemoryRoute'),
})

const appsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/apps',
  loader: () => queryClient.ensureQueryData(appsQuery),
  component: lazyRouteComponent(() => import('./routes/apps'), 'AppsView'),
})

const incidentsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/incidents',
  validateSearch: (search: Record<string, unknown>) => stringParam(search, 'lineage') as { lineage?: string },
  loader: () => queryClient.ensureQueryData(incidentsQuery),
  component: lazyRouteComponent(() => import('./routes/incidents'), 'IncidentsRoute'),
})

const remediationsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/remediations',
  validateSearch: (search: Record<string, unknown>) => stringParam(search, 'review') as { review?: string },
  loader: () => queryClient.ensureQueryData(remediationsQuery),
  component: lazyRouteComponent(() => import('./routes/remediations'), 'RemediationsRoute'),
})

const clusterRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/cluster',
  loader: () => queryClient.ensureQueryData(clusterQuery),
  component: lazyRouteComponent(() => import('./routes/cluster'), 'ClusterView'),
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  overviewRoute,
  memoryRoute,
  appsRoute,
  incidentsRoute,
  remediationsRoute,
  clusterRoute,
])

export const router = createRouter({
  routeTree,
  // The Go server serves index.html for any unclaimed path, so a deep link the
  // router does not know about lands here rather than on a 404 from the server.
  defaultNotFoundComponent: NotFound,
  // A loader that throws (a 401, a server that went away) lands on the error state
  // instead of a blank page.
  defaultErrorComponent: RouteError,
  // A loader that is still reading shows skeleton rows after 150 ms, and keeps them
  // for at least 300 ms so a fast read does not flash.
  defaultPendingComponent: RoutePending,
  defaultPendingMs: 150,
  defaultPendingMinMs: 300,
  // Route loaders prime the cache from the same query options the views use, so
  // a navigation never shows a spinner for data already in memory.
  defaultPreload: 'intent',
  scrollRestoration: true,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
