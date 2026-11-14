import '@testing-library/jest-dom/vitest'

import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

afterEach(() => {
  if (typeof window === 'undefined') return // a node-environment test
  cleanup()
  localStorage.clear()
  document.documentElement.classList.remove('dark')
})

if (typeof window !== 'undefined') window.scrollTo = () => {}

// jsdom has no matchMedia. A test that cares replaces it; the default is a light system.
if (typeof window !== 'undefined' && !window.matchMedia) {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList
}
