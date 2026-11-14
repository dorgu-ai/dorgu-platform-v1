import { cva, type VariantProps } from 'class-variance-authority'
import type * as React from 'react'

import { cn } from '@/lib/utils'

/**
 * Buttons, section 5.1 of DESIGN.md.
 *
 * There is deliberately no "approve" variant. The accent fill is reserved for the
 * one consequential action on a screen, Approve and its confirm step, and this
 * release cannot approve anything: the dashboard writes nothing to the cluster.
 * The variant is added in the release that adds the action, so the accent button
 * cannot appear on a screen before the thing it stands for exists.
 */
const buttonVariants = cva(
  'inline-flex items-center justify-center gap-1.5 rounded-ctl text-small font-medium whitespace-nowrap transition-colors disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        // The ordinary primary action: solid ink.
        primary: 'bg-ink text-ink-on hover:bg-ink/85',
        secondary:
          'border border-line-strong bg-surface text-ink hover:bg-surface-hover',
        ghost: 'text-ink-muted hover:bg-surface-hover hover:text-ink',
        // Reserved. Fill only appears inside its own confirm dialog.
        destructive: 'border border-danger bg-surface text-danger hover:bg-danger-soft',
      },
      size: {
        md: 'h-8 px-3',
        sm: 'h-7 px-2.5',
        icon: 'size-7',
      },
    },
    defaultVariants: { variant: 'secondary', size: 'sm' },
  },
)

export function Button({
  className,
  variant,
  size,
  ...props
}: React.ComponentProps<'button'> & VariantProps<typeof buttonVariants>) {
  return (
    <button type="button" className={cn(buttonVariants({ variant, size }), className)} {...props} />
  )
}
