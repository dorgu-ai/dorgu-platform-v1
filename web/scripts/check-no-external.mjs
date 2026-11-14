// Fails when the built SPA could ask the network for anything.
//
// DESIGN.md section 2: no font or asset is ever fetched from the network. The
// dashboard works offline and makes no third-party requests, which is part of the
// trust story, so this is checked on the artifact that ships rather than assumed
// from the source. It reads index.html and every stylesheet and reports any
// reference to an origin: an absolute or protocol-relative URL in src, href,
// srcset or poster, and any url() or @import in CSS that is not a path on this
// origin. A data: URI is fine; it is not a request.
//
//   node scripts/check-no-external.mjs [dist-directory]
//
// The default directory is the one the Vite build writes and go:embed reads.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const HERE = dirname(fileURLToPath(import.meta.url))
const DEFAULT_DIST = resolve(HERE, '../../internal/webui/dist')

/** A URL as written in a url() or attribute, without its quotes or padding. */
const clean = (url) => url.trim().replace(/^['"]|['"]$/g, '')

/** True for a URL that names another origin: http(s):, ws(s): or protocol-relative. */
export function isExternal(url) {
  const u = clean(url)
  return /^(?:https?:|wss?:)?\/\//i.test(u) || /^(?:ftp|file):/i.test(u)
}

/** Every external reference in an HTML document: attributes that load or navigate away. */
export function externalInHtml(html) {
  const found = []
  for (const m of html.matchAll(/\b(?:src|href|poster|data|action|formaction)\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))/gi)) {
    const value = m[2] ?? m[3] ?? m[4] ?? ''
    if (isExternal(value)) found.push(value)
  }
  for (const m of html.matchAll(/\bsrcset\s*=\s*("([^"]*)"|'([^']*)')/gi)) {
    for (const part of (m[2] ?? m[3] ?? '').split(',')) {
      const url = part.trim().split(/\s+/)[0] ?? ''
      if (isExternal(url)) found.push(url)
    }
  }
  for (const m of html.matchAll(/<meta[^>]+http-equiv\s*=\s*["']?refresh["']?[^>]*content\s*=\s*["']([^"']*)["']/gi)) {
    const url = /url\s*=\s*(.+)$/i.exec(m[1] ?? '')?.[1]
    if (url && isExternal(url)) found.push(url)
  }
  for (const m of html.matchAll(/url\(([^)]*)\)/gi)) {
    if (isExternal(m[1] ?? '')) found.push(clean(m[1] ?? ''))
  }
  return [...new Set(found)]
}

/** Every external reference in a stylesheet: url() and @import. */
export function externalInCss(css) {
  const found = []
  for (const m of css.matchAll(/url\(\s*([^)]*?)\s*\)/gi)) {
    if (isExternal(m[1] ?? '')) found.push(clean(m[1] ?? ''))
  }
  for (const m of css.matchAll(/@import\s+(?:url\(\s*)?(["']?)([^"')\s;]+)\1/gi)) {
    if (isExternal(m[2] ?? '')) found.push(m[2] ?? '')
  }
  return [...new Set(found)]
}

function walk(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) out.push(...walk(path))
    else out.push(path)
  }
  return out
}

/** Checks a built dist directory and returns the problems found, empty when clean. */
export function checkDist(dist) {
  const problems = []
  let html
  try {
    html = readFileSync(join(dist, 'index.html'), 'utf8')
  } catch {
    return [`${dist}/index.html does not exist: build the SPA first (make web)`]
  }
  for (const url of externalInHtml(html)) problems.push(`index.html references ${url}`)

  const sheets = walk(dist).filter((p) => p.endsWith('.css'))
  if (sheets.length === 0) problems.push('no stylesheet was built, so the fonts are not bundled')
  for (const sheet of sheets) {
    for (const url of externalInCss(readFileSync(sheet, 'utf8'))) problems.push(`${sheet.slice(dist.length + 1)} references ${url}`)
  }
  const fonts = walk(dist).filter((p) => /\.woff2?$/.test(p))
  if (fonts.length === 0) problems.push('no font file was built: the fonts must be bundled, not fetched')
  return problems
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const dist = resolve(process.argv[2] ?? DEFAULT_DIST)
  const problems = checkDist(dist)
  if (problems.length > 0) {
    console.error('The built SPA references the network:')
    for (const p of problems) console.error(`  ${p}`)
    process.exit(1)
  }
  console.log(`ok: ${dist} references no external origin and bundles its fonts`)
}
