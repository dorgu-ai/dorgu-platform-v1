/** Later release, said once and in the same words wherever it appears. */
export const APPROVE_LATER_NOTE =
  'Approving from the dashboard arrives in a later release. Until then the CLI is how a proposal is approved, and it prints this same plan first.'


/** The owners Dorgu will not write, and where each one's change has to be made. */
const OWNED_WHERE: Record<string, { owner: string; where: string }> = {
  helm: { owner: 'Helm', where: 'the chart or its values' },
  argocd: { owner: 'Argo CD', where: 'the Git source it syncs from' },
  flux: { owner: 'Flux', where: 'the Git source it reconciles' },
  kustomize: { owner: 'Kustomize', where: 'the kustomization it builds from' },
}

/**
 * The review footer's sentence for a workload something else owns: approving
 * records the decision only, and the change has to be made where the owner keeps
 * it. Short and calm, and it says what to do next. Undefined for a workload Dorgu
 * may write.
 */
export function ownedFooterNote(managedBy: string): string | undefined {
  const named = OWNED_WHERE[managedBy]
  if (named) {
    return `Approving records the decision only. ${named.owner} owns this workload, so make the change there, in ${named.where}.`
  }
  if (managedBy === 'unknown') {
    return 'Approving records the decision only. Dorgu could not tell what owns this workload, so it will not write it. Make the change where its desired state lives.'
  }
  return undefined
}
