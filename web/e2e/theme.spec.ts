import { expect, test } from '@playwright/test'

import type { Page } from '@playwright/test'

import { bodyBackground, open, tokenColour } from './helpers'

const html = (page: Page) => page.locator('html')
const toggle = (page: Page) => page.getByRole('button', { name: /^Theme:/ })

test.describe('the theme toggle', () => {
  test('starts on System and follows the operating system, in both directions', async ({ page }) => {
    await open(page, 'full', '/overview', 'system')
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.reload()
    await expect(toggle(page)).toHaveAttribute('data-theme-preference', 'system')
    await expect(html(page)).toHaveClass(/dark/)

    await page.emulateMedia({ colorScheme: 'light' })
    await expect(html(page)).not.toHaveClass(/dark/)
  })

  test('cycles System, Light, Dark, System and remembers the choice across a reload', async ({ page }) => {
    await open(page, 'full', '/overview', 'system')
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.reload()

    await toggle(page).click()
    await expect(toggle(page)).toHaveAttribute('data-theme-preference', 'light')
    await expect(html(page)).not.toHaveClass(/dark/) // the user chose light over a dark system

    await toggle(page).click()
    await expect(toggle(page)).toHaveAttribute('data-theme-preference', 'dark')
    await expect(html(page)).toHaveClass(/dark/)

    await page.emulateMedia({ colorScheme: 'light' })
    await page.reload()
    await expect(toggle(page)).toHaveAttribute('data-theme-preference', 'dark')
    await expect(html(page)).toHaveClass(/dark/) // stored, so the light system is ignored

    await toggle(page).click()
    await expect(toggle(page)).toHaveAttribute('data-theme-preference', 'system')
    await expect(html(page)).not.toHaveClass(/dark/)
  })

  test('applies the saved theme before first paint, with no flash', async ({ page }) => {
    await open(page, 'full', '/overview', 'dark')
    const first = await page.evaluate(() => performance.getEntriesByType('paint').length)
    expect(first).toBeGreaterThan(0)
    await expect(html(page)).toHaveClass(/dark/)
    expect(await bodyBackground(page)).toBe(await tokenColour(page, 'bg'))
  })

  test('uses the design tokens in each theme, and the two themes differ', async ({ page }) => {
    await open(page, 'full', '/overview', 'light')
    const lightBg = await bodyBackground(page)
    expect(lightBg).toBe(await tokenColour(page, 'bg'))
    await toggle(page).click() // Light -> Dark
    await expect(html(page)).toHaveClass(/dark/)
    const darkBg = await bodyBackground(page)
    expect(darkBg).toBe(await tokenColour(page, 'bg'))
    expect(darkBg).not.toBe(lightBg)
  })

  test('is a visible control with a name that says what it will do', async ({ page }) => {
    await open(page, 'full', '/overview', 'system')
    await expect(toggle(page)).toHaveAccessibleName('Theme: System. Switch to Light.')
  })
})
