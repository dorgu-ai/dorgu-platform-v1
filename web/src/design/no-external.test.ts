// @vitest-environment node
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { afterEach, describe, expect, it } from 'vitest'

import { checkDist, externalInCss, externalInHtml, isExternal } from '../../scripts/check-no-external.mjs'

describe('what counts as external', () => {
  it('flags every way of naming another origin', () => {
    for (const u of ['https://fonts.googleapis.com/css', 'http://x.example/a.js', '//cdn.example/a.js', 'HTTPS://X.EXAMPLE', 'wss://x/stream', 'ftp://x', 'file:///etc/passwd', '"https://x.example"']) {
      expect(isExternal(u), u).toBe(true)
    }
  })

  it('accepts paths on this origin and data URIs', () => {
    for (const u of ['/assets/a.js', './files/inter.woff2', 'assets/a.css', 'data:font/woff2;base64,AAAA', '#main', '']) {
      expect(isExternal(u), u).toBe(false)
    }
  })
})

describe('HTML and CSS scanning', () => {
  it('finds an external script, stylesheet, image, preload and srcset', () => {
    const html = `<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter">
      <script src='//cdn.example/x.js'></script><img src=https://x.example/a.png srcset="/a.png 1x, https://x.example/b.png 2x">
      <meta http-equiv="refresh" content="0; url=https://evil.example">`
    const found = externalInHtml(html)
    expect(found).toContain('https://fonts.googleapis.com/css2?family=Inter')
    expect(found).toContain('//cdn.example/x.js')
    expect(found).toContain('https://x.example/a.png')
    expect(found).toContain('https://x.example/b.png')
    expect(found).toContain('https://evil.example')
  })

  it('finds an inline style url', () => {
    expect(externalInHtml('<div style="background:url(https://x.example/a.png)">')).toEqual(['https://x.example/a.png'])
  })

  it('passes a document that only loads from its own origin', () => {
    expect(externalInHtml('<script type="module" src="/assets/index-abc.js"></script><link rel="stylesheet" href="/assets/index.css"><a href="#main">')).toEqual([])
  })

  it('finds url() and @import in CSS', () => {
    const css = `@import url("https://fonts.example/a.css"); @import 'https://x.example/b.css';
      @font-face { src: url(https://x.example/f.woff2) } .a { background: url( "//y.example/i.png" ) }`
    expect(externalInCss(css).sort()).toEqual(
      ['//y.example/i.png', 'https://fonts.example/a.css', 'https://x.example/b.css', 'https://x.example/f.woff2'].sort(),
    )
  })

  it('passes bundled fonts and data URIs', () => {
    expect(externalInCss('@font-face{src:url(./files/inter.woff2) format("woff2")} .a{background:url(data:image/png;base64,AA)}')).toEqual([])
  })
})

describe('checking a built directory', () => {
  const dirs: string[] = []
  afterEach(() => {
    for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true })
  })

  function dist(files: Record<string, string>) {
    const dir = mkdtempSync(join(tmpdir(), 'dist-'))
    dirs.push(dir)
    for (const [name, body] of Object.entries(files)) {
      mkdirSync(join(dir, name, '..'), { recursive: true })
      writeFileSync(join(dir, name), body)
    }
    return dir
  }

  const ok = {
    'index.html': '<script type="module" src="/assets/i.js"></script><link rel="stylesheet" href="/assets/i.css">',
    'assets/i.css': '@font-face{src:url(./inter.woff2)}',
    'assets/inter.woff2': 'x',
  }

  it('passes a self-contained build', () => {
    expect(checkDist(dist(ok))).toEqual([])
  })

  it('fails an external font stylesheet in index.html', () => {
    const d = dist({ ...ok, 'index.html': '<link href="https://fonts.googleapis.com/css2" rel="stylesheet">' })
    expect(checkDist(d)).toEqual(['index.html references https://fonts.googleapis.com/css2'])
  })

  it('fails an external url in the CSS', () => {
    const d = dist({ ...ok, 'assets/i.css': '@font-face{src:url(https://fonts.gstatic.com/a.woff2)}' })
    expect(checkDist(d)[0]).toContain('assets/i.css references https://fonts.gstatic.com/a.woff2')
  })

  it('fails when the fonts are not bundled at all', () => {
    const rest: Record<string, string> = { ...ok }
    delete rest['assets/inter.woff2']
    expect(checkDist(dist(rest))).toEqual(['no font file was built: the fonts must be bundled, not fetched'])
  })

  it('fails when there is no stylesheet, and when there is no build', () => {
    expect(checkDist(dist({ 'index.html': '<p>', 'f.woff2': 'x' }))).toContain('no stylesheet was built, so the fonts are not bundled')
    expect(checkDist(join(tmpdir(), 'does-not-exist-dist'))[0]).toContain('build the SPA first')
  })
})
