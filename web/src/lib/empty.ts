import type { OperatorStatus } from './types'

/**
 * The five reasons a list can be empty (DESIGN.md sections 5.7 and 10), and which
 * views there are. The components that render them are in components/empty-state.
 */
export type EmptyKind = 'warming' | 'crds-missing' | 'operator-idle' | 'none' | 'refused'

export type ViewId = 'apps' | 'incidents' | 'remediations' | 'cluster' | 'memory' | 'overview'

/** Which empty kind a settled, readable, installed view with no rows is. Pure, so it is tested alone. */
export function settledEmptyKind(view: ViewId, operator: OperatorStatus | undefined): EmptyKind {
  // Apps and Cluster list live objects, not the operator's records: an empty one
  // is a statement about the cluster whether or not the operator runs.
  if (view === 'apps' || view === 'cluster') return 'none'
  // Without evidence the operator is down, an empty record is the operator's own
  // finding. With evidence it is down, emptiness says nothing about the cluster.
  if (operator?.found && operator.ready === 0) return 'operator-idle'
  return 'none'
}

