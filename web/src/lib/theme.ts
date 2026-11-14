/**
 * Theme selection, section 8 of DESIGN.md: `.dark` follows `prefers-color-scheme`
 * unless the user picked a theme, which is stored in localStorage. The toggle
 * cycles System, Light, Dark.
 *
 * public/theme-init.js applies the same rule before first paint; keep them in step.
 */
export type ThemePreference = 'system' | 'light' | 'dark'

export const THEME_STORAGE_KEY = 'dorgu-theme'

export const THEME_ORDER: readonly ThemePreference[] = ['system', 'light', 'dark']

export function readPreference(storage: Pick<Storage, 'getItem'> = localStorage): ThemePreference {
  try {
    const value = storage.getItem(THEME_STORAGE_KEY)
    if (value === 'light' || value === 'dark' || value === 'system') return value
  } catch {
    // Storage can throw in a locked-down browser; the system theme is the safe answer.
  }
  return 'system'
}

export function savePreference(
  preference: ThemePreference,
  storage: Pick<Storage, 'setItem'> = localStorage,
): void {
  try {
    storage.setItem(THEME_STORAGE_KEY, preference)
  } catch {
    // The theme still applies for this page load.
  }
}

export function nextPreference(current: ThemePreference): ThemePreference {
  const index = THEME_ORDER.indexOf(current)
  return THEME_ORDER[(index + 1) % THEME_ORDER.length] ?? 'system'
}

export function isDark(preference: ThemePreference, systemPrefersDark: boolean): boolean {
  return preference === 'dark' || (preference === 'system' && systemPrefersDark)
}

export function applyTheme(preference: ThemePreference, root: HTMLElement = document.documentElement): void {
  const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  root.classList.toggle('dark', isDark(preference, systemDark))
}
