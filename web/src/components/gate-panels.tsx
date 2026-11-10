import { useState } from "react";

import { effectiveTicketState, expiredText } from "@/lib/gate";
import type { GateVerdict, Incident, Remediation } from "@/lib/types";
import { absolute } from "@/lib/utils";

import { Badge, type BadgeTone } from "./ui/badge";

/*
 * Read-only panels for what the operator's gate recorded (operator v0.13.x).
 * Deliberately plain markup: the visual treatment belongs to the design system.
 * Everything rendered here is operator-written data, never model text.
 */

const verdictTone: Record<GateVerdict["verdict"], BadgeTone> = {
  pass: "ok",
  clamp: "warn",
  refuse: "danger",
};

function VerdictList({ verdicts }: { verdicts: GateVerdict[] }) {
  const stages = Array.from(new Set(verdicts.map((v) => v.stage)));
  return (
    <ul>
      {stages.map((stage) => (
        <li key={stage}>
          <span className="font-mono text-[11px] text-ink-muted">{stage}</span>
          <ul>
            {verdicts
              .filter((v) => v.stage === stage)
              .map((v) => (
                <li
                  key={`${v.stage}/${v.guardrail}`}
                  className="flex flex-wrap items-baseline gap-1.5 text-[11px]"
                >
                  <Badge tone={verdictTone[v.verdict] ?? "neutral"}>
                    {v.verdict}
                  </Badge>
                  <span className="font-mono text-ink">{v.guardrail}</span>
                  {v.reason && (
                    <span className="text-ink-muted">{v.reason}</span>
                  )}
                  {v.numbers && Object.keys(v.numbers).length > 0 && (
                    <span className="font-mono text-ink-faint">
                      {Object.entries(v.numbers)
                        .map(([key, value]) => `${key}=${value}`)
                        .join(" ")}
                    </span>
                  )}
                </li>
              ))}
          </ul>
        </li>
      ))}
    </ul>
  );
}

/** Gate verdicts per stage, the apply ticket, and why a plan expired or was refused. */
export function GatePanel({ remediation }: { remediation: Remediation }) {
  const { gate, ticket, expired, gateRefusal } = remediation;
  // Read once at mount: an issued ticket may have run out since the last
  // server push, and the panel is only open while someone is looking at it.
  const [now] = useState(() => Date.now());
  if (!gate && !ticket && !expired && !gateRefusal) return null;

  const ticketState = ticket
    ? effectiveTicketState(ticket.state, ticket.expiresAt, now)
    : undefined;

  return (
    <section aria-label="Operator gate" className="max-w-3xl space-y-1.5">
      <h4 className="text-[11px] font-semibold tracking-wide text-ink uppercase">
        Operator gate (the operator’s record, not the plan’s)
      </h4>

      {gate && (
        <div>
          <div className="text-[11px] text-ink-muted">
            {gate.phase === "Proposal" ? "At proposal" : "At approval re-check"}
            : {gate.result}
            {" · "}
            {absolute(gate.evaluatedAt)}
          </div>
          {(gate.verdicts?.length ?? 0) > 0 && (
            <VerdictList verdicts={gate.verdicts ?? []} />
          )}
        </div>
      )}

      {ticket && ticketState && (
        <div className="text-[11px]">
          <span className="text-ink-muted">Apply ticket: </span>
          <Badge
            tone={
              ticketState === "issued"
                ? "info"
                : ticketState === "consumed"
                  ? "ok"
                  : "neutral"
            }
          >
            {ticketState}
          </Badge>{" "}
          <span className="text-ink-faint">
            {ticket.issuedAt ? `issued ${absolute(ticket.issuedAt)}, ` : ""}
            {ticketState === "consumed" ? "used" : "expires"}{" "}
            {absolute(ticket.expiresAt)}
          </span>
          {(ticket.verdicts?.length ?? 0) > 0 && (
            <VerdictList verdicts={ticket.verdicts ?? []} />
          )}
        </div>
      )}

      {expired && (
        <div className="text-[11px]">
          <Badge tone="neutral">Expired: {expired.reason}</Badge>{" "}
          <span className="text-ink-muted">{expiredText(expired)}</span>
        </div>
      )}

      {gateRefusal && (
        <div className="text-[11px]">
          <Badge tone="danger">Refused at approval: {gateRefusal.reason}</Badge>{" "}
          <span className="text-ink-muted">
            {gateRefusal.message ??
              "The gate refused this plan when it was approved. Nothing was applied."}
          </span>
        </div>
      )}
    </section>
  );
}

const proposalTone: Record<string, BadgeTone> = {
  refused: "danger",
  deferred: "info",
  proposed: "ok",
  cleared: "neutral",
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
    return <span className="text-ink-faint">none</span>;
  }
  return (
    <div className="min-w-0 space-y-0.5">
      {proposal && (
        <>
          <Badge tone={proposalTone[proposal.state] ?? "neutral"}>
            {proposal.state}
          </Badge>
          <div
            className="truncate text-[11px] text-ink-faint"
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
          className="truncate text-[11px] text-ink-faint"
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
    <div className="truncate text-[11px] text-ink-faint" title={detail}>
      {parts.join(" · ")}
    </div>
  );
}
