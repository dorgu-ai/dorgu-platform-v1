// @vitest-environment node
import { Linter } from 'eslint'
import { describe, expect, it } from 'vitest'

import plugin from '../../eslint-rules/no-raw-colour.js'

const linter = new Linter()

function lint(code: string) {
  return linter.verify(code, [
    {
      files: ['**/*.tsx'],
      languageOptions: { parserOptions: { ecmaFeatures: { jsx: true } } },
      plugins: { dorgu: plugin },
      rules: { 'dorgu/no-raw-colour': 'error' },
    },
  ], { filename: 'component.tsx' })
}

describe('the raw colour lint rule', () => {
  const bad: Record<string, string> = {
    'a hex colour': `const c = '#4f46e5'`,
    'a short hex colour': `const c = '#fff'`,
    'a hex with alpha': `const c = "#4f46e5cc"`,
    'an arbitrary background': `const c = <div className="bg-[#123456]" />`,
    'an arbitrary text colour': `const c = <div className="p-2 text-[color:var(--x)]" />`,
    'a default palette colour': `const c = <div className="bg-red-500" />`,
    'a palette colour with a variant': `const c = <div className="hover:text-gray-400" />`,
    'white': `const c = <div className="bg-white" />`,
    'black': `const c = <div className="text-black" />`,
    'a colour function': `const c = 'rgb(1, 2, 3)'`,
    'a template literal': 'const c = `bg-blue-500 ${x}`',
    'oklch': `const c = 'oklch(70% 0.1 20)'`,
    'a ring colour': `const c = <div className="ring-indigo-600" />`,
  }
  for (const [name, code] of Object.entries(bad)) {
    it(`rejects ${name}`, () => {
      const messages = lint(code)
      expect(messages.length, code).toBeGreaterThan(0)
      expect(messages[0]?.ruleId).toBe('dorgu/no-raw-colour')
    })
  }

  const good: Record<string, string> = {
    'a token background': `const c = <div className="bg-surface text-ink" />`,
    'a token with opacity': `const c = <div className="border-danger/40 bg-danger-soft" />`,
    'a hover token': `const c = <div className="hover:bg-surface-hover" />`,
    'an arbitrary size': `const c = <div className="text-[13px] w-[20rem]" />`,
    'a type scale token': `const c = <div className="text-kpi text-small" />`,
    'an anchor': `const c = '#main'`,
    'an id that is not hex': `const c = '#root'`,
    'ordinary words': `const c = 'the text-blue-ish word'`,
  }
  for (const [name, code] of Object.entries(good)) {
    it(`accepts ${name}`, () => {
      expect(lint(code), code).toEqual([])
    })
  }

  it('names what it found', () => {
    expect(lint(`const c = 'bg-red-500'`)[0]?.message).toContain('bg-red-500')
    expect(lint(`const c = '#abcdef'`)[0]?.message).toContain('hex')
  })
})
