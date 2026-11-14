import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import type * as React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useEventStream } from './stream'

class FakeEventSource {
  static instances: FakeEventSource[] = []
  static CLOSED = 2
  readyState = 0
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  listeners = new Map<string, EventListener>()
  closed = false
  url: string
  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }
  addEventListener(type: string, fn: EventListener) {
    this.listeners.set(type, fn)
  }
  removeEventListener(type: string) {
    this.listeners.delete(type)
  }
  close() {
    this.closed = true
  }
  emit(type: string, data: string) {
    this.listeners.get(type)?.({ data } as unknown as Event)
  }
}

let client: QueryClient
const wrapper = ({ children }: { children: React.ReactNode }) => (
  <QueryClientProvider client={client}>{children}</QueryClientProvider>
)

beforeEach(() => {
  FakeEventSource.instances = []
  client = new QueryClient()
  vi.stubGlobal('EventSource', FakeEventSource)
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const source = () => FakeEventSource.instances[0] as FakeEventSource

describe('the event stream', () => {
  it('opens one connection to the stream endpoint', () => {
    renderHook(() => useEventStream(), { wrapper })
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(source().url).toBe('/api/v1/stream')
  })

  it('starts connecting, with no last-live time', () => {
    const { result } = renderHook(() => useEventStream(), { wrapper })
    expect(result.current).toEqual({ state: 'connecting', lastLiveAt: undefined })
  })

  it('goes live on open and writes each pushed snapshot straight into the cache', () => {
    const { result } = renderHook(() => useEventStream(), { wrapper })
    act(() => source().onopen?.())
    expect(result.current.state).toBe('live')

    act(() => source().emit('overview', JSON.stringify({ window: '7d', report: null })))
    expect(client.getQueryData(['overview'])).toEqual({ window: '7d', report: null })
  })

  it('subscribes to the overview and memory topics too', () => {
    renderHook(() => useEventStream(), { wrapper })
    expect([...source().listeners.keys()].sort()).toEqual(['apps', 'cluster', 'incidents', 'memory', 'meta', 'overview', 'remediations'])
  })

  it('remembers when a live stream dropped, so the bar can say stale since', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-14T14:02:00'))
    const { result } = renderHook(() => useEventStream(), { wrapper })
    act(() => source().onopen?.())
    act(() => source().onerror?.())
    expect(result.current.state).toBe('connecting')
    expect(result.current.lastLiveAt).toBe(Date.parse('2026-11-14T14:02:00'))
  })

  it('reports offline when the browser gives up, and keeps the last data', () => {
    const { result } = renderHook(() => useEventStream(), { wrapper })
    act(() => source().onopen?.())
    act(() => source().emit('apps', JSON.stringify({ apps: [] })))
    act(() => {
      source().readyState = FakeEventSource.CLOSED
      source().onerror?.()
    })
    expect(result.current.state).toBe('offline')
    expect(client.getQueryData(['apps'])).toEqual({ apps: [] })
  })

  it('goes offline, leaving the last good data, on a payload it cannot parse', () => {
    const { result } = renderHook(() => useEventStream(), { wrapper })
    act(() => source().onopen?.())
    act(() => source().emit('apps', JSON.stringify({ apps: ['good'] })))
    act(() => source().emit('apps', '{not json'))
    expect(result.current.state).toBe('offline')
    expect(client.getQueryData(['apps'])).toEqual({ apps: ['good'] })
  })

  it('closes the connection on unmount', () => {
    const { unmount } = renderHook(() => useEventStream(), { wrapper })
    unmount()
    expect(source().closed).toBe(true)
  })
})
