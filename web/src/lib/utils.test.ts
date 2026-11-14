import { afterEach, describe, expect, it, vi } from 'vitest'

import { absolute, age, clockTime, cn, confidencePercent, copyText, recurrenceLabel, resourceLine } from './utils'

afterEach(() => vi.useRealTimers())

describe('cn', () => {
  it('lets a later colour override an earlier one but keeps a type-scale token', () => {
    // The bug this guards: tailwind-merge read text-kpi as a colour and dropped it.
    expect(cn('text-kpi font-semibold', 'text-ink')).toBe('text-kpi font-semibold text-ink')
    expect(cn('text-ink', 'text-danger')).toBe('text-danger')
    expect(cn('text-small', 'text-body')).toBe('text-body')
    expect(cn('px-2', 'px-4')).toBe('px-4')
  })
})

describe('time and text helpers', () => {
  it('formats an age like kubectl', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-14T12:00:00Z'))
    expect(age('2026-11-14T11:59:30Z')).toBe('30s')
    expect(age('2026-11-14T11:15:00Z')).toBe('45m')
    expect(age('2026-11-14T06:00:00Z')).toBe('6h')
    expect(age('2026-11-10T12:00:00Z')).toBe('4d')
    expect(age(undefined)).toBe('')
    expect(age('not a time')).toBe('')
  })

  it('keeps an unparseable time visible rather than blank', () => {
    expect(absolute('yesterday')).toBe('yesterday')
    expect(absolute(undefined)).toBe('')
    expect(absolute('2026-11-14T12:00:00Z')).not.toBe('')
  })

  it('formats a clock time', () => {
    expect(clockTime(Date.parse('2026-11-14T14:02:00')).replace(/\D/g, '')).toBe('1402')
  })

  it('labels a recurrence: no suffix is r1, -r2 is r2', () => {
    expect(recurrenceLabel(0)).toBe('r1')
    expect(recurrenceLabel(1)).toBe('r1')
    expect(recurrenceLabel(2)).toBe('r2')
    expect(recurrenceLabel(11)).toBe('r11')
  })

  it('renders confidence and resources', () => {
    expect(confidencePercent('0.88')).toBe('88%')
    expect(confidencePercent('high')).toBe('high')
    expect(resourceLine({ cpu: '100m', memory: '128Mi' })).toBe('cpu 100m  mem 128Mi')
    expect(resourceLine(undefined)).toBe('')
  })

  it('reports whether a copy worked', async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
    expect(await copyText('x')).toBe(true)
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('no')) } })
    expect(await copyText('x')).toBe(false)
  })
})
