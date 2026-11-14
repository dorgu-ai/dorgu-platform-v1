import { fileURLToPath, URL } from 'node:url'

import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

/**
 * Unit and component tests. The Playwright suite (e2e/) is separate and is not
 * picked up here: it needs a built SPA and a fixture server.
 */
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    css: false,
    coverage: {
      provider: 'v8',
      include: ['src/components/**', 'src/lib/**', 'src/design/**'],
      exclude: ['src/**/*.test.*', 'src/test/**', 'src/lib/types.ts'],
      reporter: ['text-summary', 'lcov'],
    },
  },
})
