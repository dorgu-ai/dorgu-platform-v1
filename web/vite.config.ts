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
