import { cn } from '@/lib/utils'
import type { HealthSource, HealthStatus, ManagedBy, Severity } from '@/lib/types'

import { Badge, type BadgeTone } from './ui/badge'
import { Tooltip } from './ui/tooltip'

// ---------------------------------------------------------------------------
// health
// ---------------------------------------------------------------------------

const healthTone: Record<HealthStatus, BadgeTone> = {
  Healthy: 'ok',
  Degraded: 'warn',
  Unhealthy: 'danger',
  Unknown: 'neutral',
}

/**
 * Explains where a health reading came from.
 *
 * The distinction is load-bearing and is therefore on screen rather than in a
 * code comment: a persona status is the operator's verdict, and an observed
 * status is what the dashboard read off the Deployment and its Pods a moment
 * ago. Presenting the second as the first would be claiming the operator is
 * watching an app it is not.
 */
const healthSourceNote: Record<HealthSource, string> = {
  'persona-status': 'Reported by the Dorgu operator.',
  observed: 'Read by the dashboard from live Deployment and Pod state, not from the operator.',
  none: 'Nothing to derive a health reading from.',
}

export function HealthBadge({
  status,
  source,
  message,
  disagreement,
}: {
  status: HealthStatus
  source: HealthSource
  message?: string
  disagreement?: string
}) {
  const note = [disagreement, message, healthSourceNote[source]].filter(Boolean).join(' ')

  return (
    <div className="flex min-w-0 flex-col items-start gap-0.5">
      <Tooltip content={note}>
        <Badge tone={healthTone[status]} className="cursor-default">
          <Dot tone={healthTone[status]} />
          {status}
          {source === 'observed' && <span className="text-ink-muted">observed</span>}
        </Badge>
      </Tooltip>
      {/*
        The disagreement is called out on the row, not only in the tooltip. A
        stale verdict is a fact about the operator, and a reader deciding whether
        to trust this screen needs it without hovering.
      */}
      {disagreement && (
        <Tooltip content={disagreement}>
          <span className="cursor-default truncate text-caption text-warn">
            operator says otherwise
          </span>
        </Tooltip>
      )}
    </div>
  )
}

const dotColour: Record<BadgeTone, string> = {
  ok: 'bg-ok',
  warn: 'bg-warn',
  danger: 'bg-danger',
  accent: 'bg-accent',
  machine: 'bg-machine',
  'warn-outline': 'bg-warn',
  neutral: 'bg-ink-faint',
  outline: 'bg-ink-faint',
}

export function Dot({ tone }: { tone: BadgeTone }) {
  return (
    <span
      aria-hidden="true"
      className={cn('inline-block size-1.5 shrink-0 rounded-full', dotColour[tone])}
    />
  )
}

// ---------------------------------------------------------------------------
// severity
// ---------------------------------------------------------------------------

const severityTone: Record<Severity, BadgeTone> = {
  critical: 'danger',
  warning: 'warn',
  info: 'neutral',
}

export function SeverityBadge({ severity }: { severity: Severity }) {
  const tone = severityTone[severity] ?? 'neutral'
  return (
    <Badge tone={tone} className="uppercase tracking-wide">
      <Dot tone={tone} />
      {severity}
    </Badge>
  )
}

// ---------------------------------------------------------------------------
// ownership
// ---------------------------------------------------------------------------

/**
 * What each owner means for what Dorgu will do.
 *
 * Only 'unmanaged' permits the CLI to patch a Deployment. Everything else,
 * including 'unknown', means Dorgu explains and does not write. Saying so here
 * is how a user learns which of their apps can be healed before they try, rather
 * than from a refusal afterwards.
 */
const managedByNote: Record<ManagedBy, string> = {
  helm: 'Owned by a Helm release. Dorgu will explain a fix rather than patch the Deployment, because patching it would make the next helm upgrade fail on a field-manager conflict.',
  argocd:
    'Owned by an Argo CD application. Dorgu will explain a fix rather than patch the Deployment; Argo CD would revert it or report it as drift.',
  flux: 'Reconciled by a Flux controller. Dorgu will explain a fix rather than patch the Deployment.',
  kustomize:
    'Declared through a kustomize overlay. Dorgu will explain a fix rather than patch the Deployment.',
  unmanaged:
    'Nothing reconciles this workload, so Dorgu can apply an approved fix to it with your credentials.',
  unknown:
    'The owner could not be determined. Dorgu treats unknown as owned and will explain rather than write, because absence of evidence that patching is safe is not evidence that it is.',
}

const managedByLabel: Record<ManagedBy, string> = {
  helm: 'Helm',
  argocd: 'Argo CD',
  flux: 'Flux',
  kustomize: 'Kustomize',
  unmanaged: 'Unmanaged',
  unknown: 'Unknown owner',
}

/** The sentence that goes with every owner Dorgu will not write. */
export const ADVISORY_ONLY = 'advisory only: Dorgu will not write this workload'

/**
 * The ownership badge, section 5.3. `Unmanaged` is neutral; Helm, Argo CD, Flux
 * and Kustomize are warn outline, and carry the advisory sentence. An unknown
 * owner is treated as owned, so it carries the sentence too but is not coloured
 * as a fact about a named owner.
 */
export function ManagedByBadge({
  managedBy,
  detail,
  showAdvisory = false,
}: {
  managedBy: ManagedBy
  detail?: string
  showAdvisory?: boolean
}) {
  const note = [detail, managedByNote[managedBy] ?? ''].filter(Boolean).join(' ')
  const writable = managedBy === 'unmanaged'
  const named = managedBy !== 'unmanaged' && managedBy !== 'unknown'

  return (
    <span className="inline-flex flex-wrap items-center gap-1.5">
      <Tooltip content={note}>
        <Badge tone={named ? 'warn-outline' : 'neutral'} className="cursor-default">
          {managedByLabel[managedBy] ?? managedBy}
        </Badge>
      </Tooltip>
      {showAdvisory && !writable && <span className="text-small text-ink-muted">{ADVISORY_ONLY}</span>}
    </span>
  )
}

// ---------------------------------------------------------------------------
// attribution
// ---------------------------------------------------------------------------

/**
 * An unattributed incident is a real outage Dorgu can see but cannot diagnose or
 * remediate, because no persona claimed the signals. It gets its own badge
 * instead of being folded into a neighbouring app.
 */
export function UnattributedBadge() {
  return (
    <Tooltip content="No ApplicationPersona claimed these signals, so this incident is recorded against the workload itself. Dorgu can see it but cannot diagnose or remediate it. Run `dorgu persona import` on that workload to change that.">
      <Badge tone="warn" className="cursor-default">
        unattributed
      </Badge>
    </Tooltip>
  )
}
