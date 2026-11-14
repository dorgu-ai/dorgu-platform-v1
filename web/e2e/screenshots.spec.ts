import { mkdirSync } from 'node:fs'

import { test } from '@playwright/test'

import { open, VIEWS, type Scenario, type Theme } from './helpers'

/**
 * Screenshots for the planner to review, in both themes, one per view plus the
 * panels and the five empty states. Skipped unless SCREENSHOTS is set, so the
 * ordinary run stays fast; CI sets it and uploads the directory as an artifact.
 */
const dir = process.env.SCREENSHOT_DIR ?? 'screenshots'

test.skip(!process.env.SCREENSHOTS, 'set SCREENSHOTS=1 to capture')

const themes: Theme[] = ['light', 'dark']

for (const theme of themes) {
  test.describe(`screenshots, ${theme}`, () => {
    test.beforeAll(() => mkdirSync(`${dir}/${theme}`, { recursive: true }))

    for (const view of VIEWS) {
      test(`${view.id}`, async ({ page }) => {
        await open(page, 'full', view.path, theme)
        await page.getByRole('heading', { level: 1, name: view.heading }).waitFor()
        await page.waitForTimeout(500)
        await page.screenshot({ path: `${dir}/${theme}/${view.id}.png`, fullPage: view.id === 'overview' })
      })
    }

    const panels: [string, string][] = [
      ['review-clamped-helm', '/remediations?review=apps%2Ffix-oom-checkout'],
      ['review-refused', '/remediations?review=apps%2Frefused-frontend-limits'],
      ['review-ticketed', '/remediations?review=apps%2Fticketed-reports-cpu'],
      ['memory-detail', '/memory?record=apps%2Fcheckout-cpu-throttle'],
    ]
    for (const [name, path] of panels) {
      test(name, async ({ page }) => {
        await open(page, 'full', path, theme)
        await page.getByRole('complementary').last().waitFor()
        await page.waitForTimeout(500)
        await page.screenshot({ path: `${dir}/${theme}/${name}.png` })
      })
    }

    const empties: [Scenario, string, string][] = [
      ['warming', '/incidents', 'empty-warming'],
      ['crds-missing', '/incidents', 'empty-crds-missing'],
      ['operator-idle', '/incidents', 'empty-operator-idle'],
      ['none', '/incidents', 'empty-none'],
      ['refused', '/incidents', 'empty-refused'],
    ]
    for (const [scenario, path, name] of empties) {
      test(name, async ({ page }) => {
        await open(page, scenario, path, theme)
        await page.locator('[data-empty]').first().waitFor()
        await page.screenshot({ path: `${dir}/${theme}/${name}.png` })
      })
    }
  })
}
