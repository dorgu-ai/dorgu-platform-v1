import { cva, type VariantProps } from 'class-variance-authority'
import type * as React from 'react'

import { cn } from '@/lib/utils'

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-1.5 rounded-md text-xs font-medium transition-colors disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        default: 'bg-surface-raised border border-line text-ink hover:border-line-strong',
        ghost: 'text-ink-muted hover:bg-surface-raised hover:text-ink',
        subtle: 'bg-surface-sunken border border-line text-ink-muted hover:text-ink',
      },
      size: {
        sm: 'h-6 px-2',
        md: 'h-8 px-3',
        icon: 'h-6 w-6',
      },
    },
    defaultVariants: { variant: 'default', size: 'sm' },
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
