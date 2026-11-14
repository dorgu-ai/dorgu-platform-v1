import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { contrastRatio, hexToRgb, luminance, parseTokens, REQUIRED_PAIRS, type Theme } from './contrast'

// vitest runs from web/, where package.json lives.
const css = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8')
const tokens = parseTokens(css)

describe('contrast arithmetic', () => {
  it('matches the WCAG reference values', () => {
    expect(contrastRatio('#000000', '#ffffff')).toBeCloseTo(21, 5)
    expect(contrastRatio('#ffffff', '#ffffff')).toBeCloseTo(1, 5)
    // The reference pair from the WCAG 2.1 understanding documents: #767676 on white is 4.54:1.
    expect(contrastRatio('#767676', '#ffffff')).toBeCloseTo(4.54, 2)
    expect(luminance('#000000')).toBe(0)
    expect(luminance('#ffffff')).toBeCloseTo(1, 10)
  })

  it('is symmetric', () => {
    expect(contrastRatio('#4f46e5', '#fafafa')).toBeCloseTo(contrastRatio('#fafafa', '#4f46e5'), 10)
  })

  it('rejects a colour it cannot read, rather than reporting a ratio for it', () => {
    expect(() => hexToRgb('#fff')).toThrow(/six digit/)
    expect(() => hexToRgb('red')).toThrow(/six digit/)
  })
})

describe('the token parser', () => {
  it('reads both themes, and dark overrides light', () => {
    expect(tokens.light.bg).toBe('#fafafa')
    expect(tokens.dark.bg).toBe('#0a0a0a')
    expect(tokens.light.accent).toBe('#4f46e5')
    expect(tokens.dark.accent).toBe('#7b7bf7')
  })

  it('has every token the design lists, in both themes', () => {
    const required = [
      'bg', 'surface', 'surface-sunken', 'surface-hover', 'line', 'line-strong', 'ink', 'ink-on', 'ink-muted',
      'ink-faint', 'accent', 'accent-on', 'accent-hover', 'accent-soft', 'danger', 'danger-soft', 'warn',
      'warn-soft', 'ok', 'ok-soft', 'machine', 'machine-soft',
    ]
    for (const theme of ['light', 'dark'] as Theme[]) {
      for (const name of required) expect(tokens[theme][name], `${theme} ${name}`).toMatch(/^#[0-9a-f]{6}$/i)
    }
  })

  it('has no info blue: informational is neutral ink', () => {
    expect(Object.keys(tokens.light).filter((k) => k.startsWith('info'))).toEqual([])
  })

  it('refuses a stylesheet with no theme blocks', () => {
    expect(() => parseTokens('body { color: red }')).toThrow(/no @theme block/)
  })
})

describe('every token pair the design requires, in both themes', () => {
  for (const theme of ['light', 'dark'] as Theme[]) {
    for (const pair of REQUIRED_PAIRS) {
      it(`${theme}: ${pair.fg} on ${pair.bg} is at least ${pair.min}:1 (${pair.use})`, () => {
        const fg = tokens[theme][pair.fg]
        const bg = tokens[theme][pair.bg]
        expect(fg, `${theme} token ${pair.fg}`).toBeDefined()
        expect(bg, `${theme} token ${pair.bg}`).toBeDefined()
        expect(contrastRatio(fg as string, bg as string)).toBeGreaterThanOrEqual(pair.min)
      })
    }
  }
})
