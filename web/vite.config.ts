import { fileURLToPath, URL } from 'node:url'

import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

/**
 * The build writes straight into internal/webui/dist, which is the directory
 * go:embed reads. There is no copy step and no second place the bundle can be
 * stale in: `make web && make build` produces a binary with this exact output
 * inside it.
 */
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    // Source maps are off. They would roughly double the size of a binary a
    // user downloads, and the trust story is better served by a small artifact
    // than by a stack trace nobody has asked for yet.
    sourcemap: false,
    rollupOptions: {
      output: {
        /**
         * Vendor code is split by how often it changes, not by which view
         * happens to import it first.
         *
         * Route-level splitting alone left the shared dependencies in a chunk
         * named after whichever module rollup reached first, which made an 81 KB
         * bundle of TanStack Table read as `empty-state-*.js`. Naming them
         * deliberately does two things: the build output says what is actually
         * large, and a chunk that only changes when a dependency is upgraded
         * keeps its filename hash across releases, so a user who has one view's
         * chunk cached does not re-download React because a label changed.
         *
         * React and the router stay together in one chunk on purpose. Both are
         * needed before anything can paint, so splitting them buys two requests
         * for one dependency graph.
         */
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined

          if (/[\\/]node_modules[\\/](react|react-dom|scheduler)[\\/]/.test(id)) {
            return 'vendor-react'
          }

          // Table and virtual are split away from the router and the query
          // client on purpose. Nothing renders a table before a route does, and
          // both are reached only through the lazily imported views, so keeping
          // them here takes roughly 50 KB off the first paint. The router and
          // the query client are needed before anything can paint at all.
          if (id.includes('@tanstack/react-table') || id.includes('@tanstack/react-virtual')) {
            return 'vendor-table'
          }
          if (id.includes('@tanstack')) return 'vendor-router'

          if (id.includes('@radix-ui') || id.includes('lucide-react')) return 'vendor-ui'
          return 'vendor'
        },
      },
    },
  },
  server: {
    port: 5173,
    // `npm run dev` talks to a dashboard started separately on its default
    // port, so the SPA can be hot-reloaded against a real cluster without
    // rebuilding the Go binary each time.
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:7171',
        changeOrigin: false,
      },
    },
  },
})
