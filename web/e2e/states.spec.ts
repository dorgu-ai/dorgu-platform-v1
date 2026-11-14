import { expect, test, type Page } from '@playwright/test'

import { TOKEN } from '../playwright.config'
import { base, open, type Scenario } from './helpers'

/**
 * The five reasons a list can be empty are five different screens, on every view
 * that has the reason. Each scenario is a different cluster.
 */
const kind = (page: Page) => page.locator('[data-empty]').first()

/**
 * The state is the one named, and a reader can see it: in the viewport, not
 * pushed below the fold by the table header above it. Being in the DOM is not
 * enough, and a layout bug once hid every empty list.
 */
async function expectEmpty(page: Page, state: string) {
  await expect(kind(page)).toHaveAttribute('data-empty', state)
  await expect(kind(page)).toBeInViewport()
}

test.describe('empty states', () => {
  test('warming: skeleton rows and the initial-list sentence, never "no incidents"', async ({ page }) => {
    for (const path of ['/incidents', '/remediations', '/memory']) {
      await open(page, 'warming', path)
      await expectEmpty(page, 'warming')
      await expect(page.getByText(/Reading your cluster/)).toBeVisible()
      await expect(page.getByTestId('skeleton-rows')).toBeVisible()
      await expect(page.getByText('No incidents recorded')).toHaveCount(0)
    }
  })

  test('warming: the Overview shows the skeleton state and no figures', async ({ page }) => {
    await open(page, 'warming', '/overview')
    await expectEmpty(page, 'warming')
    await expect(page.getByTestId('kpi-tile')).toHaveCount(0)
  })

  test('CRDs missing: the operator is not installed, and the install command is offered', async ({ page }) => {
    for (const path of ['/incidents', '/remediations', '/memory', '/overview']) {
      await open(page, 'crds-missing', path)
      await expectEmpty(page, 'crds-missing')
      await expect(page.getByText('The Dorgu operator is not installed on this cluster')).toBeVisible()
      await expect(page.getByText('dorgu install')).toBeVisible()
    }
  })

  test('CRDs missing: Apps and Cluster still work, because they read live objects', async ({ page }) => {
    await open(page, 'crds-missing', '/apps')
    await expect(page.getByRole('table', { name: 'Applications' })).toContainText('checkout-podinfo')
    await open(page, 'crds-missing', '/cluster')
    await expect(page.getByRole('table', { name: /Nodes/ })).toContainText('ip-10-0-1-11')
  })

  test('operator idle: installed, nothing running, and empty is not good news', async ({ page }) => {
    for (const path of ['/incidents', '/remediations', '/memory']) {
      await open(page, 'operator-idle', path)
      await expectEmpty(page, 'operator-idle')
      await expect(page.getByText('The operator is installed and nothing is running')).toBeVisible()
      await expect(page.getByText(/0 of 1 replicas ready/)).toBeVisible()
    }
  })

  test('none: the operator is running and has recorded nothing', async ({ page }) => {
    const expectations: [string, string][] = [
      ['/incidents', 'No incidents recorded'],
      ['/remediations', 'No proposals yet'],
      ['/memory', 'The operator has recorded no incidents'],
    ]
    for (const [path, title] of expectations) {
      await open(page, 'none', path)
      await expectEmpty(page, 'none')
      await expect(page.getByText(title)).toBeVisible()
    }
  })

  test('none: the Overview says nothing was proposed and shows the figures as no data', async ({ page }) => {
    await open(page, 'none', '/overview')
    await expect(page.getByText('Nothing has been proposed in this window')).toBeVisible()
    await expect(page.getByTestId('kpi-tile').first()).toHaveAttribute('data-no-data', 'true')
  })

  test('read refused: names the resource and the reason', async ({ page }) => {
    for (const path of ['/incidents', '/remediations', '/memory', '/overview']) {
      await open(page, 'refused', path)
      await expectEmpty(page, 'refused')
      await expect(page.getByRole('heading', { name: /Dorgu could not read/ })).toBeVisible()
      await expect(page.getByText(/may not list or watch this resource/).first()).toBeVisible()
    }
    await open(page, 'refused', '/remediations')
    await expect(page.getByRole('heading', { name: /remediationactions/ })).toBeVisible()
  })

  test('each scenario is a different screen', async ({ page }) => {
    const seen = new Set<string>()
    const scenarios: Scenario[] = ['warming', 'crds-missing', 'operator-idle', 'none', 'refused']
    for (const s of scenarios) {
      await open(page, s, '/incidents')
      seen.add((await kind(page).getAttribute('data-empty')) ?? 'none-found')
    }
    expect([...seen].sort()).toEqual(['crds-missing', 'none', 'operator-idle', 'refused', 'warming'])
  })
})

test.describe('loading and error states', () => {
  test('shows a skeleton while the first read is in flight, not zeros', async ({ page }) => {
    // The stream would push a snapshot at once, so only the slow read feeds this page.
    await page.route('**/api/v1/stream', (route) => route.abort())
    await page.route('**/api/v1/overview', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1500))
      await route.continue()
    })
    await page.goto(`${base('full')}/?token=${TOKEN}`)
    await page.goto(`${base('full')}/overview`)
    await expect(page.getByTestId('route-pending')).toBeVisible()
    await expect(page.getByText('Reading your cluster.')).toBeVisible()
    await expect(page.getByTestId('kpi-tile')).toHaveCount(0)
    await expect(page.getByTestId('kpi-tile').first()).toBeVisible({ timeout: 10_000 })
  })

  test('shows an error state with what to do when a read fails', async ({ page }) => {
    await page.route('**/api/v1/memory', (route) => route.fulfill({ status: 500, contentType: 'text/plain', body: 'the cache exploded' }))
    await open(page, 'full', '/memory')
    const alert = page.getByRole('alert')
    await expect(alert).toContainText('the cache exploded')
    await expect(alert).toContainText(/Try another view|reload/)
  })

  test('says what to do when the session is not authorised', async ({ page }) => {
    await page.route('**/api/v1/incidents', (route) => route.fulfill({ status: 401, contentType: 'text/plain', body: 'unauthorised' }))
    await open(page, 'full', '/incidents')
    await expect(page.getByRole('alert')).toContainText('Open the link that `dorgu dashboard` printed in your terminal')
  })
})
