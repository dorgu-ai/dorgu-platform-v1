import { expect, test, type Page } from '@playwright/test'

import { open, tokenColour } from './helpers'

const panel = (page: Page, name: string) => page.getByRole('complementary', { name: `Review ${name}` })

test.describe('the review panel (read-only)', () => {
  test('opens from the list, addressed by the URL, and closes on Escape', async ({ page }) => {
    await open(page, 'full', '/remediations')
    await page.getByRole('link', { name: 'Review fix-oom-checkout' }).click()
    await expect(page).toHaveURL(/review=apps%2Ffix-oom-checkout/)
    await expect(panel(page, 'fix-oom-checkout')).toBeVisible()
    await expect(panel(page, 'fix-oom-checkout').getByRole('heading', { level: 2 })).toBeFocused()

    await page.keyboard.press('Escape')
    await expect(panel(page, 'fix-oom-checkout')).toHaveCount(0)
    await expect(page).toHaveURL(/\/remediations$/)
  })

  test('survives a reload because the URL addresses it', async ({ page }) => {
    await open(page, 'full', '/remediations?review=apps%2Ffix-oom-checkout')
    await page.reload()
    await expect(panel(page, 'fix-oom-checkout')).toBeVisible()
  })

  test('lays out target, diff, gate, guardrails, plan and ticket, in that order', async ({ page }) => {
    await open(page, 'full', '/remediations?review=apps%2Ffix-oom-checkout')
    const headings = await panel(page, 'fix-oom-checkout').getByRole('heading', { level: 3 }).allTextContents()
    expect(headings.slice(0, 6)).toEqual(['Target', 'Diff', 'Gate', 'Guardrails', 'Plan', 'Ticket'])
  })

  test('shows the ownership badge, the diff columns, the gate strip and the guardrail meter with its ceiling', async ({ page }) => {
    await open(page, 'full', '/remediations?review=apps%2Ffix-oom-checkout')
    const p = panel(page, 'fix-oom-checkout')
    await expect(p.getByText('Helm', { exact: true })).toBeVisible()
    await expect(p.getByText('advisory only: Dorgu will not write this workload').first()).toBeVisible()
    await expect(p.getByText('Running now').first()).toBeVisible()
    await expect(p.getByText('After approval').first()).toBeVisible()
    await expect(p.getByTestId('gate-strip').locator('[data-state="passed"]')).toHaveCount(6)
    await expect(p.getByTestId('gate-strip')).toContainText('clamped')

    const meter = p.getByTestId('guardrail-meter')
    await expect(meter).toContainText('change +3100% of limit (max +100%)')
    await expect(meter).toContainText('ceiling source')
    await expect(meter).toHaveAttribute('data-tone', 'danger')
  })

  test('puts the model-written plan in a machine text block, as plain text', async ({ page }) => {
    await open(page, 'full', '/remediations?review=apps%2Ffix-oom-checkout')
    const blocks = panel(page, 'fix-oom-checkout').getByTestId('machine-text')
    await expect(blocks.first()).toContainText('AI explanation · untrusted text')
    await expect(blocks.first()).toContainText('Raise the memory limit')
    expect(await blocks.count()).toBeGreaterThanOrEqual(2)
  })

  test('a refusal stops the gate strip and the later stages are not run', async ({ page }) => {
    await open(page, 'full', '/remediations?review=apps%2Frefused-frontend-limits')
    const strip = panel(page, 'refused-frontend-limits').getByTestId('gate-strip')
    await expect(strip.locator('[data-stage="Guardrails"]')).toHaveAttribute('data-state', 'refused')
    await expect(strip.locator('[data-stage="ModePolicy"]')).toHaveAttribute('data-state', 'not-run')
    await expect(strip.locator('[data-stage="DedupCooldown"]')).toHaveAttribute('data-state', 'not-run')
    await expect(panel(page, 'refused-frontend-limits')).toContainText('Refused at approval (BlastRadius)')
  })

  test('an issued ticket shows its countdown', async ({ page }) => {
    await open(page, 'full', '/remediations?review=apps%2Fticketed-reports-cpu')
    await expect(panel(page, 'ticketed-reports-cpu')).toContainText(/Ticket issued, expires in 1\d:\d\d/)
  })

  test('a consumed ticket says consumed, and an expired plan says why', async ({ page }) => {
    await open(page, 'full', '/remediations?review=apps%2Fcompleted-checkout-cpu')
    await expect(panel(page, 'completed-checkout-cpu')).toContainText('Ticket consumed')
    await open(page, 'full', '/remediations?review=legacy%2Fexpired-legacy-memory')
    await expect(panel(page, 'expired-legacy-memory')).toContainText('Ticket expired: ApprovalTimedOut')
  })

  test('names who is looking, and offers the exact CLI command with a copy button', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await open(page, 'full', '/remediations?review=apps%2Ffix-oom-reports')
    const p = panel(page, 'fix-oom-reports')
    await expect(p.getByTestId('viewing-as')).toContainText('Viewing as alex@example.com')
    await expect(p.getByText('dorgu remediation approve fix-oom-reports -n apps')).toBeVisible()
    await expect(p.getByTestId('approve-later-note')).toContainText('Approving from the dashboard arrives in a later release')

    await p.getByRole('button', { name: /Copy: dorgu remediation approve/ }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('dorgu remediation approve fix-oom-reports -n apps')
  })

  test('has no Approve or Reject button, and nothing in the accent fill that is reserved for Approve', async ({ page }) => {
    for (const name of ['fix-oom-checkout', 'fix-oom-reports', 'ticketed-reports-cpu']) {
      await open(page, 'full', `/remediations?review=apps%2F${name}`)
      await expect(panel(page, name)).toBeVisible()
      // The copy button's name contains the word, because it copies "dorgu remediation approve ...".
      await expect(page.getByRole('button', { name: /^(?!Copy:).*\bapprove\b/i })).toHaveCount(0)
      await expect(page.getByRole('button', { name: /reject/i })).toHaveCount(0)

      const accent = await tokenColour(page, 'accent')
      const filled = await page.locator('button').evaluateAll(
        (els, colour) => els.filter((e) => getComputedStyle(e).backgroundColor === colour).length,
        accent,
      )
      expect(filled).toBe(0)
    }
  })

  test('uses the full width below the large breakpoint', async ({ page }) => {
    await page.setViewportSize({ width: 800, height: 900 })
    await open(page, 'full', '/remediations?review=apps%2Ffix-oom-checkout')
    const box = await panel(page, 'fix-oom-checkout').boundingBox()
    expect(box?.width).toBeGreaterThan(700)
  })

  test('is two columns on lg and unified below', async ({ page }) => {
    await open(page, 'full', '/remediations?review=apps%2Ffix-oom-checkout')
    await expect(page.getByText('Running now').first()).toBeVisible()
    await page.setViewportSize({ width: 900, height: 900 })
    await expect(page.getByText('Running now').first()).toBeHidden()
  })

  test('shows a proposal that is not in the list as such, not as a blank panel', async ({ page }) => {
    await open(page, 'full', '/remediations?review=apps%2Fno-such-plan')
    await expect(page.getByText(/is not in this list/)).toBeVisible()
  })
})
