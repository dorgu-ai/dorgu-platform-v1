import { afterEach, describe, expect, it, vi } from 'vitest'

import { applyTheme, isDark, nextPreference, readPreference, savePreference } from './theme'

afterEach(() => vi.restoreAllMocks())

function systemDark(dark: boolean) {
  vi.spyOn(window, 'matchMedia').mockImplementation(
    (query) =>
      ({ matches: dark && query.includes('dark'), media: query, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList,
  )
}

describe('theme selection', () => {
  it('cycles System, Light, Dark and back', () => {
    expect(nextPreference('system')).toBe('light')
    expect(nextPreference('light')).toBe('dark')
    expect(nextPreference('dark')).toBe('system')
  })

  it('follows the system unless the user picked a theme', () => {
    expect(isDark('system', true)).toBe(true)
    expect(isDark('system', false)).toBe(false)
    expect(isDark('light', true)).toBe(false)
    expect(isDark('dark', false)).toBe(true)
  })

  it('defaults to the system theme, and ignores a stored value it does not know', () => {
    expect(readPreference()).toBe('system')
    localStorage.setItem('dorgu-theme', 'sepia')
    expect(readPreference()).toBe('system')
  })

  it('persists a choice', () => {
    savePreference('dark')
    expect(localStorage.getItem('dorgu-theme')).toBe('dark')
    expect(readPreference()).toBe('dark')
  })

  it('survives storage that throws', () => {
    const broken = { getItem: () => { throw new Error('denied') }, setItem: () => { throw new Error('denied') } }
    expect(readPreference(broken)).toBe('system')
    expect(() => savePreference('dark', broken)).not.toThrow()
  })

  it('applies the dark class from the preference and the system', () => {
    systemDark(true)
    applyTheme('system')
    expect(document.documentElement).toHaveClass('dark')
    applyTheme('light')
    expect(document.documentElement).not.toHaveClass('dark')
    systemDark(false)
    applyTheme('system')
    expect(document.documentElement).not.toHaveClass('dark')
    applyTheme('dark')
    expect(document.documentElement).toHaveClass('dark')
  })
})
