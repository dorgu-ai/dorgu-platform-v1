import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import type * as React from 'react'

import { cn } from '@/lib/utils'

/**
 * Wraps Radix's provider rather than re-exporting it. A re-assigned import is
 * not statically recognisable as a component, which breaks fast refresh for
 * every consumer of this file.
 */
export function TooltipProvider(props: TooltipPrimitive.TooltipProviderProps) {
  return <TooltipPrimitive.Provider {...props} />
}

/**
 * A tooltip is where the long form of a fact lives: the full managedBy detail,
 * a match-chain rung, an absolute timestamp. The dense table shows the short
 * form; nothing important is only in a tooltip.
 */
export function Tooltip({
  children,
  content,
  side = 'top',
}: {
  children: React.ReactNode
  content: React.ReactNode
  side?: 'top' | 'right' | 'bottom' | 'left'
}) {
  if (!content) return <>{children}</>

  return (
    <TooltipPrimitive.Root delayDuration={200}>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side={side}
          sideOffset={6}
          className={cn(
            'z-50 max-w-sm rounded-md border border-line-strong bg-surface-raised px-2.5 py-1.5',
            'text-xs leading-relaxed text-ink shadow-lg',
          )}
        >
          {content}
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  )
}
