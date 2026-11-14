import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render } from '@testing-library/react'
import type * as React from 'react'

import { TooltipProvider } from '@/components/ui/tooltip'

/**
 * Renders a component inside what the real app provides: a query client, a
 * router (the signal badge and the review links are router Links) and the tooltip
 * provider. The router has every path the components link to, each rendering
 * nothing, so a Link resolves and a click is observable through the history.
 */
export function renderWithApp(
  ui: React.ReactNode,
  { path = '/', seed }: { path?: string; seed?: (queryClient: QueryClient) => void } = {},
) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  seed?.(queryClient)

  const root = createRootRoute({
    component: () => (
      <TooltipProvider delayDuration={0}>
        <div data-testid="root">{ui}</div>
      </TooltipProvider>
    ),
  })
  const routes = ['/', '/incidents', '/remediations', '/memory', '/overview', '/apps', '/cluster'].map((p) =>
    createRoute({ getParentRoute: () => root, path: p, component: () => null }),
  )
  const history = createMemoryHistory({ initialEntries: [path] })
  const router = createRouter({ routeTree: root.addChildren(routes), history })

  const result = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { ...result, queryClient, history, router }
}
