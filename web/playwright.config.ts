import { defineConfig, devices } from '@playwright/test'

/**
 * End-to-end tests against the real dashboard server over a fixture cluster.
 *
 * No Kubernetes cluster is involved: hack/fixture-server fills the real store
 * from internal/fixtures and serves the production handlers, token checks and the
 * SPA embedded from internal/webui/dist. Build both first (`make e2e-build`, or
 * `make web && make fixture-server`).
 *
 * One server per scenario, because the five empty states are different clusters.
 * The ports are fixed and local to this machine: 127.0.0.1 only.
 */
export const SCENARIOS = {
  full: 4301,
  warming: 4302,
  'crds-missing': 4303,
  'operator-idle': 4304,
  none: 4305,
  refused: 4306,
} as const

export const TOKEN = 'fixture-token'

export default defineConfig({
  testDir: './e2e',
  // The fixtures use the real clock, so a long retry loop would only hide a flake.
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  forbidOnly: !!process.env.CI,
  use: {
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } } }],
  webServer: Object.entries(SCENARIOS).map(([scenario, port]) => ({
    command: `../bin/fixture-server -scenario ${scenario} -port ${port}`,
    port,
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  })),
})
