import { Check, Copy } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { copyText } from '@/lib/utils'

import { Button } from './ui/button'

/**
 * A command shown as text, with a copy button that reports whether it worked.
 *
 * The command is always readable, not hidden behind the button: someone reading
 * over a shoulder or pasting into a runbook needs to see it, and a copy that
 * silently failed would otherwise be indistinguishable from one that worked.
 */
export function CopyCommand({
  command,
  label,
}: {
  command: string
  label?: string
}) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')

  useEffect(() => {
    if (state === 'idle') return
    const timer = setTimeout(() => setState('idle'), 1800)
    return () => clearTimeout(timer)
  }, [state])

  const copy = useCallback(() => {
    void copyText(command).then((ok) => setState(ok ? 'copied' : 'failed'))
  }, [command])

  return (
    <div className="inline-flex max-w-full items-center gap-1.5 rounded-md border border-line bg-surface-sunken py-1 pr-1 pl-2">
      {label && <span className="shrink-0 text-[11px] text-ink-faint">{label}</span>}
      <code className="truncate font-mono text-[11px] text-ink">{command}</code>
      <Button
        variant="ghost"
        size="icon"
        onClick={copy}
        aria-label={state === 'copied' ? 'Copied' : `Copy: ${command}`}
        title={state === 'failed' ? 'Could not copy. Select the text instead.' : 'Copy'}
      >
        {state === 'copied' ? (
          <Check className="size-3 text-ok" />
        ) : (
          <Copy className={state === 'failed' ? 'size-3 text-danger' : 'size-3'} />
        )}
      </Button>
    </div>
  )
}
