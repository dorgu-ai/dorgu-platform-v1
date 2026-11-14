import { X } from 'lucide-react'
import { useEffect, useRef } from 'react'
import type * as React from 'react'

import { Button } from './ui/button'

/**
 * The right-hand sheet that holds a detail panel (section 5.4): a side sheet on
 * `lg` and up, the full page below it.
 *
 * It is not a modal. The list behind it stays readable and reachable, which is
 * the point of a review that may need a second look at the incident it answers,
 * so there is no focus trap and no scrim. Escape closes it, focus moves to its
 * heading when it opens, and it slides in over 200 ms (an instant change under
 * `prefers-reduced-motion`).
 */
export function SideSheet({
  title,
  subtitle,
  onClose,
  children,
  footer,
  label,
}: {
  title: string
  subtitle?: React.ReactNode
  onClose: () => void
  children: React.ReactNode
  footer?: React.ReactNode
  label: string
}) {
  const heading = useRef<HTMLHeadingElement>(null)

  useEffect(() => {
    heading.current?.focus()
  }, [title])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <aside
      aria-label={label}
      data-testid="side-sheet"
      className="sheet-in fixed top-12 right-0 bottom-0 left-14 z-30 flex flex-col border-l border-line bg-surface shadow-lg lg:left-auto lg:w-[min(760px,70vw)]"
    >
      <header className="flex shrink-0 items-start gap-3 border-b border-line bg-accent-soft px-6 py-3">
        <div className="min-w-0 flex-1">
          <h2
            ref={heading}
            tabIndex={-1}
            className="truncate font-mono text-h2 font-semibold text-ink outline-none"
          >
            {title}
          </h2>
          {subtitle && <div className="mt-0.5 text-small text-ink-muted">{subtitle}</div>}
        </div>
        <Button variant="ghost" size="icon" onClick={onClose} aria-label="Close the panel">
          <X className="size-4" aria-hidden="true" />
        </Button>
      </header>
      <div tabIndex={0} className="min-h-0 flex-1 space-y-5 overflow-y-auto px-6 py-4 outline-offset-[-2px]">{children}</div>
      {footer && <footer className="shrink-0 border-t border-line bg-surface-sunken px-6 py-3">{footer}</footer>}
    </aside>
  )
}

/** A titled block inside a sheet. The title is an h3: the sheet's own title is the h2. */
export function SheetSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="space-y-2">
      <h3 className="text-caption font-medium tracking-[0.04em] text-ink-muted uppercase">{title}</h3>
      {children}
    </section>
  )
}
