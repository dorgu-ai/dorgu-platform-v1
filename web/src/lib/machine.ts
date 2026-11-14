/**
 * What the machine text block needs to decide, kept apart from the component so it
 * is tested without rendering.
 */
export const COLLAPSE_LINES = 6

/** Roughly six lines of prose at the panel's measure, for text with no newlines. */
const COLLAPSE_CHARS = 420

export type MachineSource = { kind: 'ai' } | { kind: 'agent'; name: string }

export function machineLabel(source: MachineSource): string {
  return source.kind === 'ai'
    ? 'AI explanation · untrusted text'
    : `Written by agent ${source.name} · untrusted text`
}

export function needsCollapse(text: string): boolean {
  return text.split('\n').length > COLLAPSE_LINES || text.length > COLLAPSE_CHARS
}

/** True for a diagnosis or plan source that is a model rather than Dorgu's own rules. */
export function isModelAuthored(provider: string | undefined): boolean {
  if (!provider) return false
  return /\b(ai|llm|anthropic|openai|claude|gpt|gemini|model)\b|^ai-/i.test(provider)
}
