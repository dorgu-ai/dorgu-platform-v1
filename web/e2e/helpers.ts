import { createConnection } from 'node:net'

import type { Page } from '@playwright/test'

import { SCENARIOS, TOKEN } from '../playwright.config'

export type Scenario = keyof typeof SCENARIOS
export type Theme = 'light' | 'dark'

export const VIEWS = [
  { id: 'overview', path: '/overview', heading: 'Overview' },
  { id: 'incidents', path: '/incidents', heading: 'Incidents' },
  { id: 'remediations', path: '/remediations', heading: 'Proposals and actions' },
  { id: 'apps', path: '/apps', heading: 'Apps' },
  { id: 'memory', path: '/memory', heading: 'Memory' },
  { id: 'cluster', path: '/cluster', heading: 'Cluster' },
] as const

export const base = (scenario: Scenario) => `http://127.0.0.1:${SCENARIOS[scenario]}`

/**
 * Opens a view on a scenario's server in a theme. The token goes in the URL once,
 * the way `dorgu dashboard` hands it over; the server turns it into a session
 * cookie and the SPA never sees it again.
 */
export async function open(page: Page, scenario: Scenario, path = '/overview', theme: Theme | 'system' = 'light') {
  await page.addInitScript((t) => {
    if (t !== 'system') localStorage.setItem('dorgu-theme', t)
  }, theme)
  await page.emulateMedia({ colorScheme: theme === 'dark' ? 'dark' : 'light', reducedMotion: 'reduce' })
  await page.goto(`${base(scenario)}/?token=${TOKEN}`)
  await page.goto(`${base(scenario)}${path}`)
  await page.getByRole('main').waitFor()
}

/** Waits until the stream is connected, so a test does not race the first snapshot. */
export async function live(page: Page) {
  await page.getByTestId('live-indicator').and(page.locator('[data-state="live"]')).waitFor()
}

/**
 * The computed colour a design token resolves to, as the browser reports it. A
 * probe element is used so the test never writes a colour down: the token is the
 * source of truth and the test checks the page uses it.
 */
export async function tokenColour(page: Page, token: string): Promise<string> {
  return page.evaluate((name) => {
    const probe = document.createElement('div')
    probe.style.backgroundColor = `var(--color-${name})`
    document.body.appendChild(probe)
    const colour = getComputedStyle(probe).backgroundColor
    probe.remove()
    return colour
  }, token)
}

export const bodyBackground = (page: Page) => page.evaluate(() => getComputedStyle(document.body).backgroundColor)

/** Resolves when something is listening on the local port, or rejects after the timeout. */
export async function waitForPort(port: number, timeoutMs = 20_000): Promise<void> {
  const deadline = Date.now() + timeoutMs
  for (;;) {
    const open = await new Promise<boolean>((resolve) => {
      const socket = createConnection({ port, host: '127.0.0.1' }, () => {
        socket.end()
        resolve(true)
      })
      socket.on('error', () => resolve(false))
    })
    if (open) return
    if (Date.now() > deadline) throw new Error(`nothing listened on ${port} within ${timeoutMs} ms`)
    await new Promise((r) => setTimeout(r, 100))
  }
}
