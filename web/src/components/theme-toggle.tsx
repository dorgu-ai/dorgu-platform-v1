import { Monitor, Moon, Sun } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import {
  applyTheme,
  nextPreference,
  readPreference,
  savePreference,
  type ThemePreference,
} from '@/lib/theme'

import { Button } from './ui/button'
import { Tooltip } from './ui/tooltip'

const copy: Record<ThemePreference, { label: string; icon: typeof Sun }> = {
  system: { label: 'System', icon: Monitor },
  light: { label: 'Light', icon: Sun },
  dark: { label: 'Dark', icon: Moon },
}

/**
 * The theme toggle: System, Light, Dark, in that order, remembered across loads.
 * System follows the operating system and keeps following it while the page is
 * open, which is the one case a stored preference cannot cover.
 */
export function ThemeToggle() {
  const [preference, setPreference] = useState<ThemePreference>(() => readPreference())

  useEffect(() => {
    applyTheme(preference)
    if (preference !== 'system') return
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => applyTheme('system')
    media.addEventListener('change', onChange)
    return () => media.removeEventListener('change', onChange)
  }, [preference])

  const cycle = useCallback(() => {
    setPreference((current) => {
      const next = nextPreference(current)
      savePreference(next)
      return next
    })
  }, [])

  const { label, icon: Icon } = copy[preference]
  const upcoming = copy[nextPreference(preference)].label

  return (
    <Tooltip content={`Theme: ${label}. Click for ${upcoming}.`}>
      <Button
        variant="ghost"
        size="sm"
        onClick={cycle}
        aria-label={`Theme: ${label}. Switch to ${upcoming}.`}
        data-theme-preference={preference}
      >
        <Icon className="size-3.5" aria-hidden="true" />
        <span className="max-lg:sr-only">{label}</span>
      </Button>
    </Tooltip>
  )
}
