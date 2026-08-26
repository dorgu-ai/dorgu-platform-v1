import { cva, type VariantProps } from 'class-variance-authority'
import type * as React from 'react'

import { cn } from '@/lib/utils'

/**
 * Badges carry every status in this UI, so the variants are the semantic
 * vocabulary rather than a colour palette. Nothing decorative uses danger, warn
 * or ok.
 */
const badgeVariants = cva(
  'inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium leading-4 whitespace-nowrap',
  {
    variants: {
      tone: {
        neutral: 'border-line bg-surface-sunken text-ink-muted',
        outline: 'border-line-strong bg-transparent text-ink-muted',
        danger: 'border-danger/40 bg-danger-soft text-danger',
        warn: 'border-warn/40 bg-warn-soft text-warn',
        ok: 'border-ok/40 bg-ok-soft text-ok',
        info: 'border-info/40 bg-info-soft text-info',
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
