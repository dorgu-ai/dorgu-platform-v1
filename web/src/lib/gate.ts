import type { RemediationReason, TicketState } from "./types";

/** Why a plan expired, in words. The four reasons the operator writes. */
export const expiredNote: Record<string, string> = {
  ApprovalTimedOut: "The approval deadline passed with no decision.",
  Superseded:
    "The incident escalated to a worse shape, so this plan for the milder shape was replaced.",
  PredatesGate:
    "The plan was proposed before the gate existed, so it cannot be approved.",
  IncidentResolved:
    "The incident resolved before anyone decided. Nothing was applied and no cooldown started.",
};

export function expiredText(expired: RemediationReason): string {
  return expiredNote[expired.reason] ?? expired.message ?? expired.reason;
}

/** Applies the client clock to an issued ticket, which the server cannot do between pushes. */
export function effectiveTicketState(
  state: TicketState,
  expiresAt: string,
  now: number,
): TicketState {
  if (state !== "issued") return state;
  const at = Date.parse(expiresAt);
  return Number.isNaN(at) || now < at ? "issued" : "expired";
}
