import type * as React from 'react'

import { cn } from '@/lib/utils'

/**
 * Cards and panels, section 5.2. Borders, not shadows: the one soft shadow in the
 * system is for overlays.
 *
 * `highlighted` is the item currently under review: a 1 px accent border and an
 * accent-soft header.
 */
export function Card({
  className,
  highlighted = false,
  ...props
}: React.ComponentProps<'div'> & { highlighted?: boolean }) {
  return (
    <div
      className={cn(
        'rounded-ctl border bg-surface',
        highlighted ? 'border-accent' : 'border-line',
        className,
      )}
      {...props}
    />
  )
}

export function CardHeader({
  className,
  highlighted = false,
  ...props
}: React.ComponentProps<'div'> & { highlighted?: boolean }) {
  return (
    <div
      className={cn('border-b border-line px-4 py-3', highlighted && 'bg-accent-soft', className)}
      {...props}
    />
  )
}

export function CardTitle({ className, ...props }: React.ComponentProps<'h2'>) {
  return <h2 className={cn('text-h3 font-semibold text-ink', className)} {...props} />
}

export function CardBody({ className, ...props }: React.ComponentProps<'div'>) {
  return <div className={cn('p-4', className)} {...props} />
}
