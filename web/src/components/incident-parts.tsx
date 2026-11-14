import type { Incident } from "@/lib/types";
import { absolute } from "@/lib/utils";

import { Badge, type BadgeTone } from "./ui/badge";

/**
 * The incident-side halves of the gate's record: the proposal outcome for an
 * incident and its lineage. The plan-side half (the gate strip, the ticket) is in
 * gate-strip.tsx and ticket-chip.tsx.
 */

const proposalTone: Record<string, BadgeTone> = {
  refused: "danger",
  deferred: "warn",
  proposed: "ok",
  cleared: "neutral",
};

/** A deferral is not a refusal, and the label says so rather than relying on the colour. */
const proposalLabel: Record<string, string> = {
  refused: "refused",
  deferred: "deferred, not refused",
  proposed: "proposed",
  cleared: "cleared",
};

const proposalNote: Record<string, string> = {
  refused: "The gate refused to turn this diagnosis into a plan.",
  deferred:
    "The gate stood down this cycle because a proposal is outstanding or cooling down. This is not a refusal.",
  proposed: "A remediation was proposed.",
  cleared:
    "An earlier refusal no longer applies because the incident resolved.",
};

/** Proposed, refused or deferred, and the durable refusal record. */
export function ProposalCell({ incident }: { incident: Incident }) {
  const { proposal, refusalRecord } = incident;
  if (!proposal && !refusalRecord) {
    return <span className="text-ink-muted">none</span>;
  }
  return (
    <div className="min-w-0 space-y-0.5">
      {proposal && (
        <>
          <Badge tone={proposalTone[proposal.state] ?? "neutral"}>
            {proposalLabel[proposal.state] ?? proposal.state}
          </Badge>
          <div
            className="truncate text-small text-ink-muted"
            title={proposal.message ?? proposalNote[proposal.state]}
          >
            {proposal.state === "refused"
              ? `${proposal.stage ? `${proposal.stage}: ` : ""}${proposal.guardrail}`
              : proposalNote[proposal.state]}
          </div>
        </>
      )}
      {refusalRecord && (
        <div
          className="truncate text-small text-ink-muted"
          title={`${refusalRecord.message ?? ""} (recorded ${absolute(refusalRecord.at)})`}
        >
          refusal recorded: {refusalRecord.reason}
        </div>
      )}
    </div>
  );
}

/** Recurrence lineage (-rN) and the signal family trail. */
export function LineageLine({ incident }: { incident: Incident }) {
  const { lineage, signals } = incident;
  const parts: string[] = [];
  if (lineage.generation > 0) parts.push(`recurrence r${lineage.generation}`);
  if (lineage.flapping) parts.push("flapping");
  if (signals.rank && signals.family) {
    parts.push(`${signals.rank} of ${signals.family.length} in family`);
  }
  if ((signals.escalatedFrom?.length ?? 0) > 0) {
    parts.push(`from ${(signals.escalatedFrom ?? []).join(" > ")}`);
  }
  if (parts.length === 0) return null;
  const detail = [
    `Lineage ${lineage.key}`,
    lineage.recurrenceOf ? `Recurs ${lineage.recurrenceOf}` : "",
    lineage.recurrencesFolded
      ? `${lineage.recurrencesFolded} recurrences folded into this incident`
      : "",
    signals.family ? `Family: ${signals.family.join(" < ")}` : "",
    signals.containers?.length
      ? `Containers: ${signals.containers.join(", ")}`
      : "",
    signals.lastResolution
      ? `Previous resolution: ${signals.lastResolution}`
      : "",
  ]
    .filter(Boolean)
    .join("\n");
  return (
    <div className="truncate text-small text-ink-muted" title={detail}>
      {parts.join(" · ")}
    </div>
  );
}
