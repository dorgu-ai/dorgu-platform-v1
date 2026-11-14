import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { ApiError } from '@/lib/api'

import { ErrorState } from './error-state'
import { NotFound, RouteError, RoutePending } from './root-layout'

describe('route states', () => {
  it('shows skeleton rows and a sentence while a view is reading, never a spinner or a zero', () => {
    render(<RoutePending />)
    expect(screen.getByTestId('route-pending')).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByText('Reading your cluster.')).toBeInTheDocument()
    expect(screen.getByTestId('skeleton-rows')).toBeInTheDocument()
  })

  it('lists the views a stale link could have meant', () => {
    render(<NotFound />)
    expect(screen.getByRole('heading', { name: 'No such view' })).toBeInTheDocument()
    expect(screen.getByText(/Overview, Incidents, Proposals, Apps, Memory and Cluster/)).toBeInTheDocument()
  })

  it('shows a route that failed to load as an alert', () => {
    render(<RouteError error={new Error('boom')} />)
    expect(screen.getByRole('alert')).toHaveTextContent('Could not load this view')
    expect(screen.getByRole('alert')).toHaveTextContent('boom')
  })
})

describe('the error state', () => {
  it('tells an unauthorised session to open the link again, and that nothing changed', () => {
    render(<ErrorState title="Could not read" error={new ApiError(401, '/x', 'This session is not authorised.')} />)
    expect(screen.getByRole('alert')).toHaveTextContent('This session is not authorised.')
    expect(screen.getByRole('alert')).toHaveTextContent('Nothing was changed. Open the link again from your terminal.')
  })

  it('tells the reader the server is not responding and that the data on screen is the last received', () => {
    render(<ErrorState title="Could not read" error={new ApiError(0, '/x', 'The dashboard server is not responding.')} />)
    expect(screen.getByRole('alert')).toHaveTextContent('last state received')
  })

  it('shows any other failure as the message it has, with what to try', () => {
    render(<ErrorState title="Could not read" error={new ApiError(500, '/x', 'the cache exploded')} />)
    expect(screen.getByRole('alert')).toHaveTextContent('the cache exploded')
    expect(screen.getByRole('alert')).toHaveTextContent('Try another view')
  })
})
