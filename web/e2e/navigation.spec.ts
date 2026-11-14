import { spawn } from 'node:child_process'

import { expect, test } from '@playwright/test'

import { TOKEN } from '../playwright.config'
import { open, VIEWS, waitForPort } from './helpers'

test.describe('navigation', () => {
  test('lands on the Overview', async ({ page }) => {
    await open(page, 'full', '/')
    await expect(page).toHaveURL(/\/overview$/)
    await expect(page.getByRole('heading', { level: 1, name: 'Overview' })).toBeVisible()
  })

  test('lists the views in the designed order, with Agents disabled and its reason in words', async ({ page }) => {
    await open(page, 'full')
    const nav = page.getByRole('navigation', { name: 'Views' })
    await expect(nav.locator('[data-nav]')).toHaveCount(7)
    const ids = await nav.locator('[data-nav]').evaluateAll((els) => els.map((e) => e.getAttribute('data-nav')))
    expect(ids).toEqual(['overview', 'incidents', 'remediations', 'apps', 'memory', 'cluster', 'agents'])

    const agents = nav.locator('[data-nav="agents"]')
    await expect(agents).toHaveAttribute('aria-disabled', 'true')
    await expect(agents).toContainText('Connected agents arrive with the agent interface')
    await expect(agents.getByRole('link')).toHaveCount(0)
  })

  for (const view of VIEWS) {
    test(`opens ${view.id} from the sidebar and marks it current`, async ({ page }) => {
      await open(page, 'full')
      const label = view.id === 'remediations' ? 'Proposals' : view.heading
      await page.getByRole('navigation', { name: 'Views' }).getByRole('link', { name: label }).click()
      await expect(page).toHaveURL(new RegExp(`${view.path}$`))
      await expect(page.getByRole('heading', { level: 1, name: view.heading })).toBeVisible()
      await expect(page.locator(`[data-nav="${view.id}"]`)).toHaveAttribute('aria-current', 'page')
    })
  }

  test('keeps the /remediations route, so links to it still work', async ({ page }) => {
    await open(page, 'full', '/remediations')
    await expect(page.getByRole('heading', { level: 1, name: 'Proposals and actions' })).toBeVisible()
  })

  test('says which views exist for a path it does not know', async ({ page }) => {
    await open(page, 'full', '/nothing-here')
    await expect(page.getByRole('heading', { name: 'No such view' })).toBeVisible()
    await expect(page.getByText(/This build has Overview, Incidents, Proposals, Apps, Memory and Cluster/)).toBeVisible()
  })

  test('skips to the content from the keyboard', async ({ page }) => {
    await open(page, 'full')
    await page.keyboard.press('Tab')
    await expect(page.getByRole('link', { name: 'Skip to content' })).toBeFocused()
  })

  test('shows the cluster context, the identity and Live in the top bar', async ({ page }) => {
    await open(page, 'full')
    await expect(page.getByTestId('cluster-context')).toContainText('fixture-cluster')
    await expect(page.getByTestId('identity')).toContainText('alex@example.com')
    await expect(page.getByTestId('live-indicator')).toContainText('Live')
  })

  test('says Stale since a time when the stream drops, and Live again when it comes back', async ({ page }) => {
    // Its own server, so the test can really stop it: the other specs share theirs.
    const port = 4307
    const start = () => spawn('../bin/fixture-server', ['-scenario', 'full', '-port', String(port)], { stdio: 'ignore' })
    let server = start()
    try {
      await waitForPort(port)
      const url = `http://127.0.0.1:${port}`
      await page.emulateMedia({ reducedMotion: 'reduce' })
      await page.goto(`${url}/?token=${TOKEN}`)
      await page.goto(`${url}/overview`)
      await expect(page.getByTestId('live-indicator')).toHaveAttribute('data-state', 'live')

      server.kill('SIGKILL')
      await expect(page.getByTestId('live-indicator')).toContainText(/Stale since \d\d:\d\d/, { timeout: 20_000 })
      // What was on screen stays: stale data is labelled, not blanked.
      await expect(page.getByTestId('kpi-tile').first()).toBeVisible()

      server = start()
      await waitForPort(port)
      await expect(page.getByTestId('live-indicator')).toContainText('Live', { timeout: 40_000 })
    } finally {
      server.kill('SIGKILL')
    }
  })

  test('collapses the sidebar to icons and remembers it across a reload', async ({ page }) => {
    await open(page, 'full')
    await page.getByRole('button', { name: 'Collapse the sidebar' }).click()
    await expect(page.getByRole('complementary', { name: 'Primary' })).toHaveCSS('width', '56px')
    await page.reload()
    await expect(page.getByRole('button', { name: 'Expand the sidebar' })).toBeVisible()
  })

  test('refuses a request with no session: the SPA says what to do', async ({ browser }) => {
    const context = await browser.newContext()
    const page = await context.newPage()
    await page.goto('http://127.0.0.1:4301/overview')
    await expect(page.getByRole('alert')).toContainText(/not authorised|Could not load/, { timeout: 15_000 })
    await context.close()
  })
})
