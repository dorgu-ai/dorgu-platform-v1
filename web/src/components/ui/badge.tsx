import { cva, type VariantProps } from 'class-variance-authority'
import type * as React from 'react'

import { cn } from '@/lib/utils'

/**
 * Badges carry every status in this UI, so the variants are the semantic
 * vocabulary rather than a colour palette. Nothing decorative uses danger, warn
 * or ok, and there is no info blue: anything informational is neutral ink.
 *
 * Every tone is paired with a word or an icon by its callers. Colour is never the
 * only signal (WCAG 1.4.1).
 */
const badgeVariants = cva(
  'inline-flex items-center gap-1 rounded-pill border px-1.5 py-0.5 text-caption leading-[14px] font-medium whitespace-nowrap',
  {
    variants: {
      tone: {
        neutral: 'border-line bg-surface-sunken text-ink-muted',
        outline: 'border-line-strong bg-transparent text-ink-muted',
        danger: 'border-danger/40 bg-danger-soft text-danger',
        warn: 'border-warn/40 bg-warn-soft text-warn',
        ok: 'border-ok/40 bg-ok-soft text-ok',
        // "You can act here": an issued ticket, a selected state.
        accent: 'border-accent bg-transparent text-accent',
        // "A machine wrote this."
        machine: 'border-machine/40 bg-machine-soft text-machine',
        // A warning that is a fact about ownership rather than health.
        'warn-outline': 'border-warn bg-transparent text-warn',
      },
    },
    defaultVariants: { tone: 'neutral' },
  },
)

export type BadgeTone = NonNullable<VariantProps<typeof badgeVariants>['tone']>

export function Badge({
  className,
  tone,
  ...props
}: React.ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
  return <span className={cn(badgeVariants({ tone }), className)} {...props} />
}
