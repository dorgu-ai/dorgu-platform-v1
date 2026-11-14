import { expect, test } from '@playwright/test'

import { base, open, VIEWS } from './helpers'

test.describe('no network beyond the dashboard itself', () => {
  test('every request, on every view, goes to the dashboard origin', async ({ page }) => {
    const origins = new Set<string>()
    page.on('request', (req) => {
      const url = new URL(req.url())
      if (url.protocol === 'data:' || url.protocol === 'blob:') return
      origins.add(url.origin)
    })
    for (const view of VIEWS) await open(page, 'full', view.path)
    await open(page, 'full', '/remediations?review=apps%2Ffix-oom-checkout')
    expect([...origins]).toEqual([base('full')])
  })

  test('uses the bundled fonts: Inter for text and JetBrains Mono for code, from this origin', async ({ page }) => {
    const fonts: string[] = []
    page.on('response', (res) => {
      if (res.url().endsWith('.woff2')) fonts.push(new URL(res.url()).origin)
    })
    await open(page, 'full', '/overview')
    await page.evaluate(() => document.fonts.ready)
    expect(await page.evaluate(() => document.fonts.check('14px "Inter Variable"'))).toBe(true)
    expect(await page.evaluate(() => getComputedStyle(document.body).fontFamily)).toContain('Inter Variable')
    await open(page, 'full', '/incidents')
    await page.evaluate(() => document.fonts.ready)
    expect(await page.evaluate(() => document.fonts.check('12px "JetBrains Mono Variable"'))).toBe(true)
    expect(fonts.length).toBeGreaterThan(0)
    expect(new Set(fonts)).toEqual(new Set([base('full')]))
  })

  test('the built page and stylesheets reference no external origin', async ({ page }) => {
    await page.goto(`${base('full')}/`)
    const foreign = await page.evaluate(() =>
      Array.from(document.querySelectorAll('[src],[href]'))
        .map((el) => el.getAttribute('src') ?? el.getAttribute('href') ?? '')
        .filter((u) => !u.startsWith('#') && !u.startsWith('data:') && new URL(u, location.href).origin !== location.origin),
    )
    expect(foreign).toEqual([])
    const css = await page.evaluate(async () => {
      const links = Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"]'))
      return Promise.all(links.map((l) => fetch(l.href).then((r) => r.text())))
    })
    expect(css.length).toBeGreaterThan(0)
    for (const sheet of css) expect(sheet).not.toMatch(/url\(\s*["']?(?:https?:)?\/\//)
  })

  test('sends the security headers the content policy promises', async ({ page }) => {
    const res = await page.goto(`${base('full')}/`)
    const csp = res?.headers()['content-security-policy'] ?? ''
    expect(csp).toContain("script-src 'self'")
    expect(csp).toContain("font-src 'self' data:")
    expect(csp).toContain("connect-src 'self'")
  })
})
