import { Lock, Terminal } from 'lucide-react'

import { GateReasons, GateStrip } from '@/components/gate-strip'
import { GuardrailMeter } from '@/components/guardrail-meter'
import { MachineText } from '@/components/machine-text'
import { ResourceDiff } from '@/components/remediation-diff'
import { StepSafetyBlock } from '@/components/remediation-safety'
import { SheetSection, SideSheet } from '@/components/side-sheet'
import { Dot, ManagedByBadge } from '@/components/status'
import { TicketChip } from '@/components/ticket-chip'
import { Badge, type BadgeTone } from '@/components/ui/badge'
import { Tooltip } from '@/components/ui/tooltip'
import { APPROVE_LATER_NOTE } from '@/lib/copy'
import { collectReadings } from '@/lib/guardrail'
import type { Identity, OwnerInstruction, Remediation, Step } from '@/lib/types'

import { CopyCommand } from './copy-command'

const riskTone: Record<string, BadgeTone> = { low: 'ok', medium: 'warn', high: 'danger', unknown: 'neutral' }

/**
 * The review panel, section 5.4, in its read-only form.
 *
 * It shows what a reviewer needs to decide whether a machine may change
 * production, top to bottom: the target workload and who owns it, the diff, the
 * gate strip, the guardrail meters with their ceilings, the text a machine wrote
 * (in the machine text block), the ticket, and who is looking. The footer is the
 * exact CLI command that can act on it, not an Approve button: that action does
 * not exist yet, and a button for it would be a promise.
 *
 * Guardrail readings come from the gate's own verdict numbers, and from a step's
 * safety entries when the gate has none (a plan older than the gate). Where both
 * name the same field the gate's reading wins.
 */
export function ReviewPanel({
  remediation,
  identity,
  onClose,
}: {
  remediation: Remediation
  identity: Identity | undefined
  onClose: () => void
}) {
  const workload = remediation.workload
  const readings = collectReadings(remediation)
  const command = remediation.approveCommand ?? remediation.diffCommand

  return (
    <SideSheet
      label={`Review ${remediation.name}`}
      title={remediation.name}
      subtitle={
        <span className="flex flex-wrap items-center gap-2">
          <Badge tone="neutral">{remediation.phase}</Badge>
          <span>
            {remediation.namespace}/{remediation.persona.name}
          </span>
          <span>{remediation.aiPlanned ? `planned by ${remediation.planSource}` : 'rule-based plan'}</span>
        </span>
      }
      onClose={onClose}
      footer={<ReviewFooter command={command} approvable={remediation.approveCommand !== undefined} identity={identity} />}
    >
      <SheetSection title="Target">
        {workload ? (
          <div className="space-y-2">
            <p className="font-mono text-code text-ink">
              {workload.kind} {workload.namespace}/{workload.name}
              {workload.container ? ` (${workload.container})` : ''}
            </p>
            <ManagedByBadge managedBy={workload.managedBy} detail={workload.managedByDetail} showAdvisory />
            {workload.owned && <OwnedWorkloadNote remediation={remediation} />}
          </div>
        ) : (
          <p className="text-body text-ink-muted">
            Dorgu has no record of the live workload behind this plan, so it cannot tell what owns it. No
            record is treated as owned, because absence of evidence that patching is safe is not evidence
            that it is.
          </p>
        )}
      </SheetSection>

      {((remediation.workloadChanges?.length ?? 0) > 0 || remediation.steps.some((s) => (s.patchChanges?.length ?? 0) > 0)) && (
        <SheetSection title="Diff">
          <div className="space-y-4">
            <ResourceDiff
              changes={remediation.workloadChanges ?? []}
              title="Deployment change"
              subtitle={
                workload
                  ? `${workload.namespace}/${workload.name}${workload.container ? ` (${workload.container})` : ''}`
                  : undefined
              }
              unobserved={workload ? !workload.observed : true}
            />
            {remediation.steps.map((step) => (
              <ResourceDiff
                key={`${step.order}-${step.id}`}
                changes={step.patchChanges ?? []}
                title="ApplicationPersona change"
                subtitle={`step ${step.order}: what Dorgu records, not what runs`}
                subject="persona"
              />
            ))}
          </div>
        </SheetSection>
      )}

      <SheetSection title="Gate">
        <GateStrip gate={remediation.gate} />
        <GateReasons gate={remediation.gate} refusal={remediation.gateRefusal} />
      </SheetSection>

      <SheetSection title="Guardrails">
        {readings.length === 0 && !remediation.steps.some((s) => (s.safety?.length ?? 0) > 0) ? (
          <p className="text-body text-ink-muted">
            No guardrail measured a numeric change in this plan, so there is no ceiling to show.
          </p>
        ) : (
          <div className="space-y-4">
            {readings.map((reading) => (
              <GuardrailMeter key={reading.field} reading={reading} />
            ))}
            {remediation.steps.map(
              (step) => (step.safety?.length ?? 0) > 0 && <StepSafetyBlock key={step.id} entries={step.safety ?? []} />,
            )}
          </div>
        )}
      </SheetSection>

      <SheetSection title="Plan">
        <PlanText remediation={remediation} />
        <ol className="space-y-4">
          {remediation.steps.map((step) => (
            <StepRow key={`${step.order}-${step.id}`} step={step} aiPlanned={remediation.aiPlanned} />
          ))}
        </ol>
      </SheetSection>

      <SheetSection title="Ticket">
        <TicketChip ticket={remediation.ticket} expired={remediation.expired} />
      </SheetSection>
    </SideSheet>
  )
}

function ReviewFooter({
  command,
  approvable,
  identity,
}: {
  command: string
  approvable: boolean
  identity: Identity | undefined
}) {
  return (
    <div className="space-y-2">
      <p className="text-small text-ink-muted" data-testid="viewing-as">
        {identity?.available ? (
          <>
            Viewing as <code className="font-mono text-code text-ink">{identity.username}</code>. The
            dashboard can do exactly what you can do.
          </>
        ) : (
          <>Your identity is not available ({identity?.reason ?? 'unknown'}).</>
        )}
      </p>
      <div className="flex flex-wrap items-center gap-3">
        <span className="flex items-center gap-1.5 text-small text-ink-muted">
          <Terminal className="size-3.5" aria-hidden="true" />
          {approvable ? 'Approve with the CLI:' : 'Read this plan in a terminal:'}
        </span>
        <CopyCommand command={command} />
      </div>
      <p className="flex items-start gap-1.5 text-small text-ink-muted" data-testid="approve-later-note">
        <Lock className="mt-0.5 size-3 shrink-0" aria-hidden="true" />
        {APPROVE_LATER_NOTE}
      </p>
    </div>
  )
}

/**
 * The plan's own prose. A model's is a machine text block, with the caption that
 * says so; a rule-based plan's is Dorgu's code speaking and is plain text.
 */
function PlanText({ remediation }: { remediation: Remediation }) {
  const blocks = [
    { label: 'Plan summary', text: remediation.planSummary },
    { label: 'Explanation', text: remediation.explanation },
  ].filter((b): b is { label: string; text: string } => Boolean(b.text))

  return (
    <div className="space-y-3">
      {blocks.map((b) =>
        remediation.aiPlanned ? (
          <MachineText key={b.label} label={b.label} text={b.text} />
        ) : (
          <p key={b.label} className="max-w-[720px] text-body whitespace-pre-line text-ink">
            <span className="font-medium">{b.label}. </span>
            {b.text}
          </p>
        ),
      )}
    </div>
  )
}

function StepRow({ step, aiPlanned }: { step: Step; aiPlanned: boolean }) {
  return (
    <li className="min-w-0 space-y-2">
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <span className="font-mono text-small text-ink-muted">[{step.order}]</span>
        <span className="font-mono text-small text-ink">{step.type}</span>
        <Tooltip
          content={
            step.risk === 'unknown'
              ? 'The plan recorded no risk for this step. Unknown is not low: an unassessed change to a running workload is not a safe one.'
              : `The plan assessed this step as ${step.risk} risk.`
          }
        >
          <Badge tone={riskTone[step.risk] ?? 'neutral'} className="cursor-default">
            {step.risk} risk
          </Badge>
        </Tooltip>
        <Tooltip
          content={
            step.mode === 'auto'
              ? 'The operator may apply this step itself. Only a persona-update step can be auto-executable: the operator never writes a workload.'
              : 'Advisory. Recorded for a human, the CLI or a platform to apply. Nothing happens until somebody does it.'
          }
        >
          <Badge tone={step.mode === 'auto' ? 'ok' : 'warn'} className="cursor-default">
            {step.mode}
          </Badge>
        </Tooltip>
        {step.status?.phase && (
          <Badge tone={step.status.phase === 'Failed' ? 'danger' : 'neutral'}>{step.status.phase}</Badge>
        )}
      </div>

      {aiPlanned ? (
        <MachineText label={`Step ${step.order}`} text={[step.description, step.rationale].filter(Boolean).join('\n\n')} />
      ) : (
        <div className="max-w-[720px] space-y-1 text-body text-ink">
          <p>{step.description}</p>
          {step.rationale && <p className="text-ink-muted">{step.rationale}</p>}
        </div>
      )}

      {step.command && <CopyCommand command={step.command} label="Run" />}
      {step.commandWithheld && (
        <p className="flex max-w-[720px] items-start gap-1.5 text-small text-warn">
          <Lock className="mt-0.5 size-3 shrink-0" aria-hidden="true" />
          {step.commandWithheld}
        </p>
      )}
    </li>
  )
}

function OwnedWorkloadNote({ remediation }: { remediation: Remediation }) {
  const workload = remediation.workload
  if (!workload) return null
  return (
    <div className="max-w-[720px] rounded-ctl border border-line-strong bg-surface-sunken p-3" data-testid="owned-note">
      <p className="flex items-center gap-1.5 text-h3 font-semibold text-ink">
        <Dot tone="warn" />
        Advisory only: Dorgu will not patch this Deployment
      </p>
      <p className="mt-1 text-body text-ink-muted">
        {workload.ownerName} owns it. {workload.whyNotPatched} No step of this plan is auto-executable;
        approving it records the decision and changes nothing in the cluster.
        {workload.ownershipSource === 'gate'
          ? ' Ownership is as resolved by the operator from the live Deployment.'
          : ' Ownership is as written in the plan; the operator\u2019s gate has not recorded one.'}
      </p>
      {(remediation.ownerInstructions?.length ?? 0) > 0 && (
        <>
          <p className="mt-2 text-body font-medium text-ink">Apply it where this workload&apos;s desired state lives:</p>
          <ol className="mt-1 space-y-2">
            {(remediation.ownerInstructions ?? []).map((instruction) => (
              <InstructionRow key={instruction.order} instruction={instruction} />
            ))}
          </ol>
        </>
      )}
    </div>
  )
}

function InstructionRow({ instruction }: { instruction: OwnerInstruction }) {
  return (
    <li className="min-w-0 space-y-1">
      <div className="flex min-w-0 gap-1.5 text-body text-ink-muted">
        <span className="shrink-0 font-mono text-ink-muted">[{instruction.order}]</span>
        <span>{instruction.description}</span>
      </div>
      {instruction.command && (
        <div className="pl-5">
          <CopyCommand command={instruction.command} label="Run" />
        </div>
      )}
    </li>
  )
}
