import { Bot } from 'lucide-react'
import { useState } from 'react'

import { machineLabel, needsCollapse, type MachineSource } from '@/lib/machine'
import { cn } from '@/lib/utils'

/**
 * The machine text block, section 5.3 of DESIGN.md. Any text an LLM or an agent
 * wrote goes through this component: titles and summaries included, no
 * exceptions.
 *
 * It renders plain text and nothing else. The text is a React text child, so it is
 * escaped: markup in it is shown, not interpreted. It is never turned into links
 * and never parsed as markdown, because it is the one input on this page an
 * attacker who can influence a workload's logs can influence, and the reader is
 * about to decide whether a machine may change production.
 *
 * Collapsed past six lines with a "Show all" control, so one verbose model cannot
 * push Dorgu's own arithmetic off the screen.
 */
export function MachineText({
  source = { kind: 'ai' },
  text,
  label,
  className,
}: {
  source?: MachineSource
  text: string
  /** Overrides the caption when the block is about one specific thing ("Rationale"). */
  label?: string
  className?: string
}) {
  const [open, setOpen] = useState(false)
  const collapsible = needsCollapse(text)
  const caption = machineLabel(source)

  return (
    <figure
      className={cn('machine-edge max-w-[720px] rounded-r-ctl bg-machine-soft px-3 py-2', className)}
      data-testid="machine-text"
    >
      <figcaption className="flex items-center gap-1.5 text-caption font-medium tracking-[0.04em] text-machine uppercase">
        <Bot className="size-3" aria-hidden="true" />
        {label ? `${label} · ${caption}` : caption}
      </figcaption>
      <p
        className={cn(
          'mt-1 text-body whitespace-pre-wrap text-ink',
          collapsible && !open && 'line-clamp-6',
        )}
      >
        {text}
      </p>
      {collapsible && (
        <button
          type="button"
          onClick={() => setOpen((value) => !value)}
          aria-expanded={open}
          className="mt-1 rounded-pill text-small font-medium text-accent hover:underline"
        >
          {open ? 'Show less' : 'Show all'}
        </button>
      )}
    </figure>
  )
}

/**
 * A one-line form for a table cell. It keeps the orchid edge and says who wrote
 * the text, so a root cause the model wrote cannot sit in a row looking like
 * Dorgu's own finding. Full text is in the title and in the detail view.
 */
export function MachineTextInline({
  source = { kind: 'ai' },
  text,
}: {
  source?: MachineSource
  text: string
}) {
  return (
    <span
      className="machine-edge flex min-w-0 items-center gap-1.5 rounded-r-pill bg-machine-soft py-0.5 pr-1.5 pl-1.5"
      title={`${machineLabel(source)}: ${text}`}
      data-testid="machine-text-inline"
    >
      <Bot className="size-3 shrink-0 text-machine" aria-hidden="true" />
      <span className="truncate text-small text-ink">{text}</span>
      <span className="sr-only">{machineLabel(source)}</span>
    </span>
  )
}

