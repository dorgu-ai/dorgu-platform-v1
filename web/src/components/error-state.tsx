import { AlertCircle } from 'lucide-react'

import { ApiError } from '@/lib/api'

/**
 * The error state every view shares: an inline alert with a soft status
 * background, an icon, a sentence and what to do (section 5.7).
 *
 * An ApiError already carries a sentence written for the reader (a 401 says to
 * open the link the CLI printed), so its message is shown as is. Anything else is
 * shown as the message it has, and never hidden behind "something went wrong".
 */
export function ErrorState({ title, error }: { title: string; error: Error }) {
  const unauthorised = error instanceof ApiError && error.status === 401
  const unreachable = error instanceof ApiError && error.status === 0

  return (
    <div className="px-6 py-10" role="alert">
      <div className="mx-auto flex max-w-[720px] items-start gap-3 rounded-ctl border border-danger/40 bg-danger-soft p-4">
        <AlertCircle className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden="true" />
        <div className="min-w-0 space-y-1">
          <h2 className="text-h3 font-semibold text-ink">{title}</h2>
          <p className="text-body text-ink">{error.message}</p>
          <p className="text-small text-ink-muted">
            {unauthorised
              ? 'Nothing was changed. Open the link again from your terminal.'
              : unreachable
                ? 'The data on screen, if any, is the last state received. Start the dashboard again and reload.'
                : 'The rest of the dashboard is unaffected. Try another view, or reload this one.'}
          </p>
        </div>
      </div>
    </div>
  )
}
