import { expect, test } from '@playwright/test'

import { open } from './helpers'

test.describe('each view renders from the fixture cluster', () => {
  test('Overview: the KPI tiles carry provenance and the window, and nothing shows a bare zero for unknown', async ({ page }) => {
    await open(page, 'full', '/overview')
    const tiles = page.getByTestId('kpi-tile')
    await expect(tiles).toHaveCount(8)
    for (const tile of await tiles.all()) await expect(tile).toContainText(/from .*, the last 7 days/)

    const safe = page.getByRole('heading', { level: 3, name: 'Safe heals' }).locator('xpath=ancestor::*[@data-testid="kpi-tile"]')
    await expect(safe).toContainText('1')
    const approval = page.getByRole('heading', { level: 3, name: 'Approval rate' }).locator('xpath=ancestor::*[@data-testid="kpi-tile"]')
    await expect(approval).toContainText('60.0%')

    await expect(page.getByRole('list', { name: 'Open incidents' }).getByRole('listitem')).toHaveCount(5)
    await expect(page.getByRole('list', { name: 'Pending and expired proposals' })).toContainText('expired: ApprovalTimedOut')
    await expect(page.getByTestId('deferred-note')).toContainText('not recorded on cluster objects')
  })

  test('Overview: switching the window asks for that window', async ({ page }) => {
    await open(page, 'full', '/overview')
    await page.getByRole('navigation', { name: 'Report window' }).getByRole('link', { name: '24h' }).click()
    await expect(page).toHaveURL(/since=24h/)
    await expect(page.getByTestId('kpi-tile').first()).toContainText('the last day')
    await expect(page.getByRole('navigation', { name: 'Report window' }).getByRole('link', { name: '24h' })).toHaveAttribute('aria-current', 'page')
  })

  test('Incidents: signal badge with recurrence, escalation, and refusals kept apart from deferrals', async ({ page }) => {
    await open(page, 'full', '/incidents')
    const table = page.getByRole('table', { name: 'Incidents' })
    await expect(table.getByRole('rowgroup')).toHaveCount(8)

    const recurring = table.getByRole('rowgroup').filter({ hasText: 'CrashLoopBackOff' }).first()
    await expect(recurring.getByRole('link', { name: /CrashLoopBackOff r2: show the lineage frontend-crashloop/ })).toBeVisible()
    await expect(recurring).toContainText('escalated')
    await expect(recurring).toContainText('refused')
    await expect(recurring).toContainText('refusal recorded')

    await expect(table).toContainText('deferred, not refused')
    await expect(table).toContainText('unattributed')
  })

  test('Incidents: a signal badge opens its lineage and the banner clears it', async ({ page }) => {
    await open(page, 'full', '/incidents')
    await page.getByRole('link', { name: /CrashLoopBackOff r2: show the lineage/ }).click()
    await expect(page).toHaveURL(/lineage=frontend-crashloop$/)
    await expect(page.getByText(/Showing the lineage/)).toBeVisible()
    await expect(page.getByRole('table', { name: 'Incidents' }).getByRole('rowgroup')).toHaveCount(2)
    await page.getByRole('link', { name: 'Show all incidents' }).click()
    await expect(page.getByRole('table', { name: 'Incidents' }).getByRole('rowgroup')).toHaveCount(8)
  })

  test('Incidents: text a model wrote is in a machine text block, and a rule engine finding is not', async ({ page }) => {
    await open(page, 'full', '/incidents')
    const ai = page.getByTestId('machine-text-inline').first()
    await expect(ai).toBeVisible()
    await expect(ai).toHaveAttribute('title', /AI explanation · untrusted text/)
    await expect(page.getByText('Container exceeded its memory limit.')).toBeVisible()
    await expect(page.getByTestId('machine-text-inline').filter({ hasText: 'Container exceeded its memory limit.' })).toHaveCount(0)
  })

  test('Proposals: every plan with its phase, owner and guardrail', async ({ page }) => {
    await open(page, 'full', '/remediations')
    const table = page.getByRole('table', { name: 'Proposals' })
    await expect(table.getByRole('rowgroup')).toHaveCount(9)
    for (const phase of ['Pending', 'Ticketed', 'Completed', 'Failed', 'Expired', 'Rejected']) {
      await expect(table).toContainText(phase)
    }
    await expect(table).toContainText('Helm')
    await expect(table).toContainText('Argo CD')
    await expect(table).toContainText('Unmanaged')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Proposals and actions')
  })

  test('Apps: the Deployments Dorgu is blind to are listed, with the command that fixes it', async ({ page }) => {
    await open(page, 'full', '/apps')
    const table = page.getByRole('table', { name: 'Applications' })
    await expect(table).toContainText('legacy-billing')
    await expect(table).toContainText(/not watched|unmonitored/i)
    await expect(table).not.toContainText('coredns') // add-on namespaces are skipped
    await expect(table).toContainText('checkout')
  })

  test('Memory: every record, open or closed, and the detail keeps the live refusal and the durable record apart', async ({ page }) => {
    await open(page, 'full', '/memory')
    const table = page.getByRole('table', { name: 'Incident memory' })
    await expect(table.getByRole('rowgroup')).toHaveCount(8)

    await table.getByRole('link', { name: 'Open checkout-cpu-throttle' }).click()
    const sheet = page.getByRole('complementary', { name: 'Memory of checkout-cpu-throttle' })
    await expect(sheet).toBeVisible()
    const conditions = sheet.getByTestId('conditions')
    await expect(conditions).toContainText('ProposalRefused')
    await expect(conditions).toContainText('False')
    await expect(conditions).toContainText('ProposalRefusalRecorded')
    await expect(conditions).toContainText('Validate_StepCommand')
    await expect(conditions).toContainText('never clears it')
    await expect(sheet.getByTestId('machine-text')).toContainText('CPU requests too low for peak traffic.')
    await expect(sheet.getByRole('link', { name: 'completed-checkout-cpu' })).toBeVisible()
  })

  test('Memory: a proposal link goes to its review panel', async ({ page }) => {
    await open(page, 'full', '/memory?record=apps%2Fcheckout-cpu-throttle')
    await page.getByRole('link', { name: 'completed-checkout-cpu' }).click()
    await expect(page).toHaveURL(/\/remediations\?review=apps%2Fcompleted-checkout-cpu/)
    await expect(page.getByRole('complementary', { name: 'Review completed-checkout-cpu' })).toBeVisible()
  })

  test('Cluster: nodes, saturation computed from live objects, and the add-ons', async ({ page }) => {
    await open(page, 'full', '/cluster')
    await expect(page.getByRole('heading', { level: 1, name: 'Cluster' })).toBeVisible()
    const nodes = page.getByRole('table', { name: /Nodes/ })
    await expect(nodes).toContainText('ip-10-0-1-11')
    await expect(nodes).toContainText('ip-10-0-2-27')
    await expect(page.getByText('observed').first()).toBeVisible()
    await expect(page.getByText('coredns')).toBeVisible()
  })

  test('the Memory detail closes on Escape', async ({ page }) => {
    await open(page, 'full', '/memory?record=apps%2Fcheckout-cpu-throttle')
    await expect(page.getByRole('complementary', { name: /Memory of/ })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('complementary', { name: /Memory of/ })).toHaveCount(0)
    await expect(page).toHaveURL(/\/memory$/)
  })
})
