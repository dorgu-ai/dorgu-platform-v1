import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'

import { open, VIEWS, type Scenario, type Theme } from './helpers'

/**
 * axe over every view in both themes. A serious or critical violation fails the
 * test; moderate and minor ones are listed in the report and do not, because a
 * contrast failure in one theme is the thing this exists to catch and a missing
 * landmark hint is not a reason to block a release.
 */
async function serious(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze()
  return results.violations
    .filter((v) => v.impact === 'serious' || v.impact === 'critical')
    .map((v) => ({
      id: v.id,
      impact: v.impact,
      help: v.help,
      nodes: v.nodes.slice(0, 3).map((n) => ({ target: n.target, summary: n.failureSummary?.split('\n').slice(0, 3).join(' | ') })),
    }))
}

const themes: Theme[] = ['light', 'dark']

for (const theme of themes) {
  test.describe(`axe, ${theme} theme`, () => {
    for (const view of VIEWS) {
      test(`${view.id} has no serious or critical violation`, async ({ page }) => {
        await open(page, 'full', view.path, theme)
        await page.getByRole('heading', { level: 1, name: view.heading }).waitFor()
        // Let the first snapshot and the virtualised rows settle.
        await page.waitForTimeout(400)
        expect(await serious(page)).toEqual([])
      })
    }

    test('the review panel, for a clamped and a refused plan, has none', async ({ page }) => {
      for (const name of ['fix-oom-checkout', 'refused-frontend-limits', 'ticketed-reports-cpu', 'completed-checkout-cpu']) {
        await open(page, 'full', `/remediations?review=apps%2F${name}`, theme)
        await page.getByRole('complementary', { name: `Review ${name}` }).waitFor()
        await page.waitForTimeout(250)
        expect(await serious(page), name).toEqual([])
      }
    })

    test('the Memory detail has none', async ({ page }) => {
      await open(page, 'full', '/memory?record=apps%2Fcheckout-cpu-throttle', theme)
      await page.getByRole('complementary', { name: /Memory of/ }).waitFor()
      await page.waitForTimeout(250)
      expect(await serious(page)).toEqual([])
    })

    test('the Overview in another window has none', async ({ page }) => {
      await open(page, 'full', '/overview?since=30d', theme)
      await page.getByTestId('kpi-tile').first().waitFor()
      expect(await serious(page)).toEqual([])
    })

    const empties: [Scenario, string][] = [
      ['warming', '/incidents'],
      ['crds-missing', '/incidents'],
      ['operator-idle', '/memory'],
      ['none', '/remediations'],
      ['none', '/overview'],
      ['refused', '/incidents'],
      ['crds-missing', '/overview'],
    ]
    for (const [scenario, path] of empties) {
      test(`the ${scenario} state of ${path} has none`, async ({ page }) => {
        await open(page, scenario, path, theme)
        await page.locator('[data-empty]').first().waitFor()
        expect(await serious(page)).toEqual([])
      })
    }
  })
}
