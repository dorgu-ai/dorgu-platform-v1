import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'

import { ThemeToggle } from './theme-toggle'

const toggle = () =>
  render(
    <TooltipProvider>
      <ThemeToggle />
    </TooltipProvider>,
  )

describe('the theme toggle', () => {
  it('starts on System and cycles System, Light, Dark, System, persisting each', async () => {
    toggle()
    const button = () => screen.getByRole('button')
    expect(button()).toHaveAttribute('data-theme-preference', 'system')
    expect(button()).toHaveAccessibleName('Theme: System. Switch to Light.')

    await userEvent.click(button())
    expect(button()).toHaveAttribute('data-theme-preference', 'light')
    expect(localStorage.getItem('dorgu-theme')).toBe('light')
    expect(document.documentElement).not.toHaveClass('dark')

    await userEvent.click(button())
    expect(button()).toHaveAttribute('data-theme-preference', 'dark')
    expect(localStorage.getItem('dorgu-theme')).toBe('dark')
    expect(document.documentElement).toHaveClass('dark')

    await userEvent.click(button())
    expect(button()).toHaveAttribute('data-theme-preference', 'system')
    expect(localStorage.getItem('dorgu-theme')).toBe('system')
  })

  it('restores a stored choice', () => {
    localStorage.setItem('dorgu-theme', 'dark')
    toggle()
    expect(screen.getByRole('button')).toHaveAttribute('data-theme-preference', 'dark')
    expect(document.documentElement).toHaveClass('dark')
  })
})
