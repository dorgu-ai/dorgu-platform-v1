import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  redirect,
} from '@tanstack/react-router'

import { NotFound, RootLayout } from './components/root-layout'
import { appsQuery, clusterQuery, incidentsQuery, metaQuery, remediationsQuery } from './lib/api'
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
  // Apps is the landing view because it is the one that works on a cluster with
  // no operator installed: every Deployment shows up as unwatched, which makes
  // the first screen the onboarding path.
  beforeLoad: () => {
    // TanStack Router's redirect() returns a control-flow object that is meant
    // to be thrown; it is not an Error and does not need to be.
    // eslint-disable-next-line @typescript-eslint/only-throw-error
    throw redirect({ to: '/apps', replace: true })
  },
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
  loader: () => queryClient.ensureQueryData(incidentsQuery),
  component: lazyRouteComponent(() => import('./routes/incidents'), 'IncidentsView'),
})

const remediationsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/remediations',
  loader: () => queryClient.ensureQueryData(remediationsQuery),
  component: lazyRouteComponent(() => import('./routes/remediations'), 'RemediationsView'),
})

const clusterRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/cluster',
  loader: () => queryClient.ensureQueryData(clusterQuery),
  component: lazyRouteComponent(() => import('./routes/cluster'), 'ClusterView'),
})

const routeTree = rootRoute.addChildren([
  indexRoute,
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
