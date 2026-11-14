import { clsx, type ClassValue } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

/**
 * The type scale tokens from DESIGN.md section 2. tailwind-merge does not know
 * them, so without this it reads `text-kpi` as a text colour and drops it when a
 * colour class such as `text-ink` follows: the KPI figure rendered at body size.
 */
export const FONT_SIZE_TOKENS = ['kpi', 'h1', 'h2', 'h3', 'body', 'table', 'small', 'caption', 'code']

const twMerge = extendTailwindMerge({
  extend: { classGroups: { 'font-size': [{ text: FONT_SIZE_TOKENS }] } },
})

/** Merges class names, letting a caller override a component's own utilities. */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Formats a timestamp as a short relative age, the way kubectl's AGE column
 * does. Absolute times are kept in the title attribute rather than dropped: a
 * relative age is easier to scan and useless in an incident writeup.
 */
export function age(timestamp: string | undefined): string {
  if (!timestamp) return ''
  const then = Date.parse(timestamp)
  if (Number.isNaN(then)) return ''

  const seconds = Math.max(0, Math.round((Date.now() - then) / 1000))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.round(minutes / 60)
  if (hours < 48) return `${hours}h`
  return `${Math.round(hours / 24)}d`
}

/** The full timestamp, for a title attribute. */
export function absolute(timestamp: string | undefined): string {
  if (!timestamp) return ''
  const parsed = Date.parse(timestamp)
  return Number.isNaN(parsed) ? timestamp : new Date(parsed).toLocaleString()
}

/**
 * Renders a confidence string as a percentage.
 *
 * The server passes the decimal string through unchanged because it is a claim
 * the diagnosis made. Presentation rounds it; nothing upstream does.
 */
export function confidencePercent(confidence: string): string {
  const value = Number.parseFloat(confidence)
  if (Number.isNaN(value)) return confidence
  return `${Math.round(value * 100)}%`
}

/** Formats a resource block as one scannable line, marking absent keys. */
export function resourceLine(values: { cpu?: string; memory?: string } | undefined): string {
  if (!values) return ''
  const parts: string[] = []
  if (values.cpu) parts.push(`cpu ${values.cpu}`)
  if (values.memory) parts.push(`mem ${values.memory}`)
  return parts.join('  ')
}

/**
 * Copies text, reporting whether it worked.
 *
 * navigator.clipboard needs a secure context, and http://localhost counts as one
 * in every current browser. The textarea fallback is there for the case where it
 * does not, because a copy button that silently does nothing is worse than no
 * button.
 */
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    try {
      const area = document.createElement('textarea')
      area.value = text
      area.setAttribute('readonly', '')
      area.style.position = 'fixed'
      area.style.opacity = '0'
      document.body.appendChild(area)
      area.select()
      const ok = document.execCommand('copy')
      document.body.removeChild(area)
      return ok
    } catch {
      return false
    }
  }
}

/** "14:02" in the reader's own clock: the moment the stream was last known good. */
export function clockTime(at: number): string {
  return new Date(at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })
}

/**
 * An incident with no -rN suffix is the first occurrence and reads r1; the suffix
 * -r2 is the second, so a generation of 0 or 1 is r1 and N is rN.
 */
export function recurrenceLabel(generation: number): string {
  return `r${Math.max(1, generation)}`
}

/** "12:41", or "1:02:09" past an hour. Text only: the countdown never animates. */
export function countdown(msRemaining: number): string {
  const total = Math.max(0, Math.floor(msRemaining / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const two = (n: number) => n.toString().padStart(2, '0')
  return h > 0 ? `${h}:${two(m)}:${two(s)}` : `${m}:${two(s)}`
}
