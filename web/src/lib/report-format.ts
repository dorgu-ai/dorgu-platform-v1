import type { ReportDistribution } from './types'

/**
 * Formatting for the report's figures. These are ports of the helpers behind
 * `dorgu report`'s text output (internal/cli/report_render.go), so a number reads
 * the same in the terminal and on the Overview page: "no data", never 0, for a
 * figure with no sample behind it.
 */

/** A ratio as one-decimal percent, or "no data" when there is none. */
export function percent(value: number | null | undefined): string {
  if (value === null || value === undefined) return 'no data'
  return `${(value * 100).toFixed(1)}%`
}

/** A length of time: 45s, 12m05s, 3h20m, 2d4h. */
export function secondsText(seconds: number): string {
  const total = Math.round(seconds)
  const two = (n: number) => n.toString().padStart(2, '0')
  if (total < 60) return `${total}s`
  if (total < 3600) return `${Math.floor(total / 60)}m${two(total % 60)}s`
  if (total < 48 * 3600) return `${Math.floor(total / 3600)}h${two(Math.floor((total % 3600) / 60))}m`
  return `${Math.floor(total / 86400)}d${Math.floor((total % 86400) / 3600)}h`
}

/** "12m05s / 1h02m (n=4)", or "no data (n=0)" or the reason the stage is blank. */
export function distributionText(d: ReportDistribution): string {
  if (d.no_data && d.reason) return `no data (${d.reason})`
  if (d.no_data || d.p50_seconds === null || d.p90_seconds === null) return 'no data (n=0)'
  return `${secondsText(d.p50_seconds)} / ${secondsText(d.p90_seconds)} (n=${d.n})`
}

export function bandSentence(band: 'below' | 'in' | 'above' | undefined): string {
  switch (band) {
    case 'below':
      return 'below the 30-80% band: proposals may be noisy'
    case 'above':
      return 'above the 30-80% band: check for rubber-stamping'
    default:
      return 'inside the 30-80% band'
  }
}

export function sumValues(values: Record<string, number>): number {
  return Object.values(values).reduce((a, b) => a + b, 0)
}

/** The window as a sentence: "7d" is "the last 7 days", anything else is shown as given. */
export function windowLabel(since: string): string {
  const days = /^(\d+)d$/.exec(since)
  if (days) return days[1] === '1' ? 'the last day' : `the last ${days[1]} days`
  return `the last ${since}`
}

/** What an unreadable input actually affects, as `dorgu report` words it. */
export function unreadableConsequence(input: string): string {
  switch (input) {
    case 'incidents':
    case 'events':
      return 'the sections that need it say no data, never zero'
    default:
      return 'no number in this report depends on it'
  }
}

/** The human name of a heal-accounting exclusion, from its report key. */
export function exclusionLabel(key: string): string {
  return key.replaceAll('_', ' ')
}
