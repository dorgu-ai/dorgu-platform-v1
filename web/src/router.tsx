import { createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'

import { NotFound, RootLayout } from './components/root-layout'
import { appsQuery, incidentsQuery, metaQuery } from './lib/api'
import { queryClient } from './query-client'
import { AppsView } from './routes/apps'
import { IncidentsView } from './routes/incidents'

/**
 * Routes are declared in code rather than generated from the filesystem.
 *
 * The route table is four entries and will stay small; the file-based plugin
 * would add a codegen step to the build for no benefit at this size, and a build
 * step that can be skipped is a build step that will be.
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
  component: AppsView,
})

const incidentsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/incidents',
  loader: () => queryClient.ensureQueryData(incidentsQuery),
  component: IncidentsView,
})

const routeTree = rootRoute.addChildren([indexRoute, appsRoute, incidentsRoute])

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
