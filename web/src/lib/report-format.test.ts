import { describe, expect, it } from 'vitest'

import {
  bandSentence, distributionText, exclusionLabel, percent, secondsText, sumValues, unreadableConsequence, windowLabel,
} from './report-format'

describe('report formatting matches dorgu report', () => {
  it('prints no data, never 0, for a missing ratio', () => {
    expect(percent(null)).toBe('no data')
    expect(percent(undefined)).toBe('no data')
    expect(percent(0)).toBe('0.0%')
    expect(percent(8 / 11)).toBe('72.7%')
  })

  it('formats a length of time as the CLI does', () => {
    expect(secondsText(45)).toBe('45s')
    expect(secondsText(12 * 60 + 5)).toBe('12m05s')
    expect(secondsText(3 * 3600 + 20 * 60)).toBe('3h20m')
    expect(secondsText(3 * 86400 + 4 * 3600)).toBe('3d4h')
    expect(secondsText(44.6)).toBe('45s')
  })

  it('formats a distribution, a missing one, and one that says why it is missing', () => {
    expect(distributionText({ n: 4, p50_seconds: 60, p90_seconds: 3540, no_data: false })).toBe('1m00s / 59m00s (n=4)')
    expect(distributionText({ n: 0, p50_seconds: null, p90_seconds: null, no_data: true })).toBe('no data (n=0)')
    expect(
      distributionText({ n: 0, p50_seconds: null, p90_seconds: null, no_data: true, reason: 'the incidents could not be read' }),
    ).toBe('no data (the incidents could not be read)')
  })

  it('words the approval band', () => {
    expect(bandSentence('below')).toContain('noisy')
    expect(bandSentence('above')).toContain('rubber-stamping')
    expect(bandSentence('in')).toContain('inside')
    expect(bandSentence(undefined)).toContain('inside')
  })

  it('words the window, and the consequence of an unreadable input', () => {
    expect(windowLabel('7d')).toBe('the last 7 days')
    expect(windowLabel('1d')).toBe('the last day')
    expect(windowLabel('36h')).toBe('the last 36h')
    expect(unreadableConsequence('events')).toContain('never zero')
    expect(unreadableConsequence('incidents')).toContain('never zero')
    expect(unreadableConsequence('personas')).toContain('no number in this report depends on it')
  })

  it('sums and labels', () => {
    expect(sumValues({ a: 1, b: 2 })).toBe(3)
    expect(sumValues({})).toBe(0)
    expect(exclusionLabel('refused_by_gate')).toBe('refused by gate')
  })
})
