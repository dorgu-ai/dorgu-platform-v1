/**
 * Contrast arithmetic and the token parser behind the automated check DESIGN.md
 * section 3.5 asks for: text at least 4.5:1, large text and UI boundaries at least
 * 3:1, in both themes, for every pair the design uses.
 *
 * The tokens are read from src/styles.css, the file the browser reads, so a value
 * changed there is checked and a value changed only in a document is not trusted.
 */

export type Theme = 'light' | 'dark'

export function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '')
  if (!/^[0-9a-fA-F]{6}$/.test(h)) throw new Error(`not a six digit hex colour: ${hex}`)
  return [0, 2, 4].map((i) => Number.parseInt(h.slice(i, i + 2), 16)) as [number, number, number]
}

/** Relative luminance, WCAG 2.x. */
export function luminance(hex: string): number {
  const [r, g, b] = hexToRgb(hex).map((v) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }) as [number, number, number]
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

export function contrastRatio(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number]
  return (hi + 0.05) / (lo + 0.05)
}

export type Tokens = Record<string, string>

/** The colour custom properties declared in a block, without their `--color-` prefix. */
function colours(block: string): Tokens {
  const out: Tokens = {}
  for (const m of block.matchAll(/--color-([a-z-]+):\s*(#[0-9a-fA-F]{6})\s*;/g)) {
    out[m[1] as string] = m[2] as string
  }
  return out
}

/** Light tokens from `@theme`, dark tokens from `.dark`, which overrides them. */
export function parseTokens(css: string): Record<Theme, Tokens> {
  const theme = /@theme\s*\{([\s\S]*?)\n\}/.exec(css)?.[1]
  const dark = /\n\.dark\s*\{([\s\S]*?)\n\}/.exec(css)?.[1]
  if (!theme || !dark) throw new Error('styles.css has no @theme block or no .dark block')
  const light = colours(theme)
  return { light, dark: { ...light, ...colours(dark) } }
}

export interface RequiredPair {
  fg: string
  bg: string
  /** Minimum ratio: 4.5 for text, 3 for large text and UI boundaries. */
  min: 4.5 | 3
  /** Where the design uses the pair, so a failure says what it breaks. */
  use: string
}

const surfaces = ['bg', 'surface', 'surface-sunken', 'surface-hover'] as const

/**
 * Every pair the design relies on. Status colours are text on their own soft
 * background and on the surfaces; ink-faint is only for placeholders and disabled
 * controls, never information, so it is held to the 3:1 of a UI element.
 */
export const REQUIRED_PAIRS: RequiredPair[] = [
  ...surfaces.map((bg) => ({ fg: 'ink', bg, min: 4.5 as const, use: 'primary text' })),
  ...surfaces.map((bg) => ({ fg: 'ink-muted', bg, min: 4.5 as const, use: 'secondary text and provenance' })),
  { fg: 'ink-faint', bg: 'surface', min: 3, use: 'placeholders, disabled controls and input borders' },
  { fg: 'ink-faint', bg: 'bg', min: 3, use: 'input borders on the page background' },
  { fg: 'ink-on', bg: 'ink', min: 4.5, use: 'primary button text' },
  { fg: 'accent-on', bg: 'accent', min: 4.5, use: 'accent fill text' },
  { fg: 'accent-on', bg: 'accent-hover', min: 4.5, use: 'accent fill on hover' },
  { fg: 'accent', bg: 'bg', min: 4.5, use: 'links on the page' },
  { fg: 'accent', bg: 'surface', min: 4.5, use: 'links on cards' },
  { fg: 'accent', bg: 'accent-soft', min: 4.5, use: 'accent text on a selected row' },
  { fg: 'ink', bg: 'accent-soft', min: 4.5, use: 'selected nav and row text' },
  { fg: 'ink-muted', bg: 'accent-soft', min: 4.5, use: 'secondary text on a selected row' },
  { fg: 'accent', bg: 'bg', min: 3, use: 'the focus ring against the page' },
  { fg: 'accent', bg: 'surface', min: 3, use: 'the focus ring against a card' },
  { fg: 'accent', bg: 'accent-soft', min: 3, use: 'the focus ring on a selected row' },
  ...(['danger', 'warn', 'ok'] as const).flatMap((s) => [
    ...(['surface', 'bg', 'surface-sunken'] as const).map((bg) => ({ fg: s, bg, min: 4.5 as const, use: `${s} text` })),
    { fg: s, bg: `${s}-soft`, min: 4.5 as const, use: `${s} badge text` },
    { fg: 'ink', bg: `${s}-soft`, min: 4.5 as const, use: `ink on the ${s} soft background` },
    { fg: 'ink-muted', bg: `${s}-soft`, min: 4.5 as const, use: `secondary text on the ${s} soft background` },
  ]),
  { fg: 'machine', bg: 'machine-soft', min: 4.5, use: 'machine text caption' },
  { fg: 'ink', bg: 'machine-soft', min: 4.5, use: 'machine text body' },
  { fg: 'ink-muted', bg: 'machine-soft', min: 4.5, use: 'machine text secondary' },
  { fg: 'machine', bg: 'surface', min: 4.5, use: 'machine accents on a card' },
]
