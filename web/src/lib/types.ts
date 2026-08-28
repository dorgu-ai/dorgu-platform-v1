/**
 * Wire types for the dashboard API.
 *
 * These mirror internal/view and internal/httpapi field for field. They are
 * hand-written rather than generated, which is a deliberate trade for a repo
 * this size: a generator is one more build step and one more thing to be out of
 * date, and the payloads are small enough that a mismatch shows up immediately
 * in the Go tests that pin the JSON shape.
 *
 * If these ever drift, the Go side is right.
 */

// ---------------------------------------------------------------------------
// shared
// ---------------------------------------------------------------------------

/**
 * The honesty header on every payload.
 *
 * An empty list has at least four causes and they call for four different
 * screens: the caches have not synced, the CRDs are not installed, the operator
 * is installed but found nothing, or the user genuinely has no apps. Rendering
 * an empty array without reading this is how a tool presents a blind spot as
 * good news.
 */
export interface Readiness {
  synced: boolean
  crdsInstalled: boolean
  missingCRDs?: string[]
  /**
   * Resources this view needs that could not be read at all.
   *
   * The fifth cause of an empty list and the only one the other four cannot
   * express: the resource exists, the operator is installed, and the read was
   * refused. Without it a rejected watch leaves `synced` false forever and the
   * screen sits on a skeleton, claiming to be reading something it gave up on.
   */
  unavailable?: UnavailableResource[]
}

/** One resource a view needs and could not read, with the reason. */
export interface UnavailableResource {
  resource: string
  reason: string
}

/** Reports that a list was capped, so a truncated view cannot read as complete. */
export interface TruncationNotice {
  shown: number
  total: number
  limit: number
}

export interface ResourceReference {
  kind: string
  name: string
  namespace?: string
  role?: string
}

export interface ResourceValues {
  cpu?: string
  memory?: string
}

/**
 * A live container resource block. An absent key is absent on the running pod,
 * which is load-bearing: a remediation may only change a key the workload
 * already has.
 */
export interface ObservedResources {
  requests?: ResourceValues
  limits?: ResourceValues
}

// ---------------------------------------------------------------------------
// apps
// ---------------------------------------------------------------------------

export type HealthStatus = 'Healthy' | 'Degraded' | 'Unhealthy' | 'Unknown'
export type HealthSource = 'persona-status' | 'observed' | 'none'
export type AppSource = 'persona' | 'deployment'

/** Who reconciles a workload's desired state. Only 'unmanaged' is patchable. */
export type ManagedBy = 'helm' | 'argocd' | 'flux' | 'kustomize' | 'unmanaged' | 'unknown'

export interface PodIssue {
  podName: string
  container: string
  reason: string
  message?: string
  phase?: string
  restartCount?: number
}

export interface AppHealth {
  status: HealthStatus
  source: HealthSource
  message?: string
  lastCheck?: string
  podIssues?: PodIssue[]
  /**
   * Set when the operator's recorded verdict was more flattering than what the
   * server could see, and states both readings. A stale Healthy over a live
   * crash loop is the one comfortable lie this view could tell, so when it
   * happens it is said out loud.
   */
  disagreement?: string
}

export interface WorkloadReplicas {
  desired: number
  ready: number
  available: number
  updated: number
}

export interface Workload {
  kind: string
  name: string
  namespace: string
  container?: string
  matchedBy?: string
  managedBy: ManagedBy
  managedByDetail?: string
  image?: string
  observedResources?: ObservedResources
  replicas: WorkloadReplicas
}

export interface AppOwnership {
  team?: string
  owner?: string
  oncall?: string
  runbook?: string
  repository?: string
}

export interface AppIncidentCounts {
  open: number
  critical: number
  personaReported: number
  lastIncidentTime?: string
}

export interface App {
  id: string
  source: AppSource
  monitored: boolean
  namespace: string
  name: string
  appName: string
  type?: string
  tier?: string
  phase?: string
  health: AppHealth
  workload?: Workload
  ownership?: AppOwnership
  incidents: AppIncidentCounts
  importCommand?: string
  warnings?: string[]
  createdAt: string
  lastUpdated?: string
}

export interface AppsSummary {
  total: number
  monitored: number
  unmonitored: number
  healthy: number
  degraded: number
  unhealthy: number
  unmonitoredNamespaces: string[]
}

export interface AppsPayload {
  apps: App[]
  summary: AppsSummary
  readiness: Readiness
}

// ---------------------------------------------------------------------------
// incidents
// ---------------------------------------------------------------------------

export type Severity = 'critical' | 'warning' | 'info'
export type IncidentPhase = 'Detected' | 'Investigating' | 'Resolved' | 'Recurring' | ''
export type Attribution = 'persona' | 'unattributed'

export interface ContributingSignal {
  signal: string
  detail: string
}

export interface IncidentPersona {
  kind: string
  name: string
  namespace?: string
  /** False on an unattributed incident, where personaRef names a workload. */
  exists: boolean
}

export interface IncidentRootCause {
  summary: string
  /** The decimal string the diagnosis wrote. Not rounded server side. */
  confidence: string
  provider: string
  contributing?: ContributingSignal[]
}

export interface IncidentResolution {
  action: string
  /** 'acknowledged' means a human approved an advisory plan and nothing was applied. */
  outcome?: 'resolved' | 'partial' | 'failed' | 'rollback' | 'acknowledged'
  appliedAt?: string
  remediationName?: string
  remediationNamespace?: string
}

export interface Incident {
  id: string
  name: string
  namespace: string
  severity: Severity
  category: string
  phase: IncidentPhase
  attribution: Attribution
  signal: string
  source: string
  persona: IncidentPersona
  rootCause?: IncidentRootCause
  affectedResources?: ResourceReference[]
  resolution?: IncidentResolution
  firstSeen: string
  lastSeen: string
  occurrenceCount: number
  lastOccurrence?: string
  createdAt: string
}

export interface IncidentsSummary {
  total: number
  open: number
  critical: number
  warning: number
  info: number
  unattributed: number
  diagnosed: number
}

export interface IncidentsPayload {
  incidents: Incident[]
  summary: IncidentsSummary
  readiness: Readiness
  truncation?: TruncationNotice
}

// ---------------------------------------------------------------------------
// remediations
// ---------------------------------------------------------------------------

export type RemediationPhase =
  | 'Pending'
  | 'Approved'
  | 'Applying'
  | 'Verifying'
  | 'Completed'
  | 'Acknowledged'
  | 'RolledBack'
  | 'Failed'
  | 'Rejected'
  | 'Expired'

export type StepRisk = 'low' | 'medium' | 'high' | 'unknown'
export type StepMode = 'auto' | 'advisory'

/**
 * Which guardrail ruled. Not a closed set: an operator newer than this build may
 * add one, and an entry this build cannot classify is still Dorgu's verdict, so
 * it is rendered rather than dropped.
 */
export type SafetyRule = 'blast-radius' | 'plan-validation' | 'absent-field'

/**
 * What happened to the value the plan asked for.
 *
 * - clamped: refused, and Dorgu substituted a value it permits.
 * - rejected: refused, and nothing replaces it. The field is gone from the patch.
 * - derived: the plan named a change and carried no usable patch, so Dorgu
 *   computed the value itself from the live workload.
 */
export type SafetyVerdict = 'clamped' | 'rejected' | 'derived'

/**
 * One guardrail verdict on one field.
 *
 * Every string in here is Dorgu's own arithmetic against the workload it
 * observed. No part of it comes from a model, which is the entire reason the
 * field exists: the verdict used to arrive spliced onto the model's rationale,
 * one line below the model's claim that the same 16x change was "well within a
 * 2x ceiling". Render it somewhere visually distinct from any prose.
 */
export interface StepSafety {
  /**
   * Typed as string, not as SafetyRule.
   *
   * The set is open: an operator newer than this build may add a rule, and an
   * entry this build cannot classify is still Dorgu's verdict, so it is rendered
   * verbatim rather than dropped. SafetyRule names the values this build knows
   * how to explain, and is used for lookups rather than to constrain the wire.
   */
  rule: string
  /** Open for the same reason as `rule`. See SafetyVerdict. */
  verdict: string
  field: string
  /** The value Dorgu measured against: the live workload's, or the persona's. */
  baseline?: string
  /**
   * The value the plan asked for.
   *
   * This is the one place a refused value may appear. It must never be rendered
   * as something that will happen: a field a guardrail refused is gone from the
   * patch, so it is in no diff, and this block is the only record that it was
   * asked for at all.
   */
  requested?: string
  /** What will actually be applied. Absent means nothing will be. */
  permitted?: string
  ratio?: string
  maxRatio?: string
  /** Dorgu's one-line rendering of the verdict, ready to print. */
  message: string
}

/** One field's before and after, used for both diffs this view renders. */
export interface ResourceChange {
  path: string
  before: string
  after: string
  /** The field does not exist today, so applying the plan introduces it. */
  added: boolean
  changed: boolean
}

export interface StepStatus {
  phase?: string
  appliedAt?: string
  verificationResult?: string
}

export interface Step {
  order: number
  id: string
  type: string
  /** Open set; StepRisk names the values this build renders a tone for. */
  risk: string
  mode: StepMode
  autoExecutable: boolean
  /** The step's own sentence, with the guardrail messages taken back out. */
  description: string
  /** The plan's reasoning. May be a model's, so it is rendered as prose. */
  rationale?: string
  command?: string
  /** Why a command the object carried is not offered. */
  commandWithheld?: string
  safety?: StepSafety[]
  patchChanges?: ResourceChange[]
  status?: StepStatus
}

export interface RemediationWorkload {
  kind: string
  name: string
  namespace: string
  container?: string
  managedBy: ManagedBy
  managedByDetail?: string
  owned: boolean
  ownerName?: string
  whyNotPatched?: string
  changeLocation?: string
  observed: boolean
  observedImage?: string
  observedResources?: ObservedResources
  observedAt?: string
}

export interface OwnerInstruction {
  order: number
  description: string
  command?: string
}

export interface RemediationRollback {
  enabled: boolean
  healthCheckAfter?: string
  maxRetries: number
}

export interface RemediationReference {
  name: string
  namespace?: string
  exists: boolean
}

export interface RemediationPersonaRef extends RemediationReference {
  kind?: string
}

export interface Condition {
  type: string
  status: string
  reason?: string
  message?: string
  lastTransitionTime?: string
}

export interface Remediation {
  id: string
  name: string
  namespace: string
  /** Open set; RemediationPhase names the values this build renders a tone for. */
  phase: string
  planSource?: string
  aiPlanned: boolean
  /** The decimal string the plan wrote. Not rounded server side. */
  confidence: string
  trustLevel: number
  planSummary?: string
  explanation?: string
  incident: RemediationReference
  persona: RemediationPersonaRef
  workload?: RemediationWorkload
  steps: Step[]
  /** What the plan does to the running container. This is the hero. */
  workloadChanges?: ResourceChange[]
  appliable: boolean
  /** Why an appliable plan will still not be applied by Dorgu. */
  appliableBlockedBy?: string
  strongestVerdict?: string
  guardrailCount: number
  ownerInstructions?: OwnerInstruction[]
  /** The CLI command that shows this plan in a terminal. */
  diffCommand: string
  rollback?: RemediationRollback
  approvalRequired: boolean
  approvalDeadline?: string
  approvedBy?: string
  approvedAt?: string
  appliedAt?: string
  verificationResult?: string
  conditions?: Condition[]
  createdAt: string
}

export interface RemediationsSummary {
  total: number
  pending: number
  appliable: number
  advisory: number
  guarded: number
  owned: number
  aiPlanned: number
  completed: number
  failed: number
}

export interface RemediationsPayload {
  remediations: Remediation[]
  summary: RemediationsSummary
  readiness: Readiness
  truncation?: TruncationNotice
}

// ---------------------------------------------------------------------------
// cluster
// ---------------------------------------------------------------------------

/** Where a Cluster-view figure came from. Same vocabulary as Apps health. */
export type FigureSource = HealthSource

export interface NamespaceCounts {
  total: number
  active: number
  withPersonas: number
}

export interface ClusterIdentity {
  personaPresent: boolean
  name?: string
  environment?: string
  description?: string
  kubernetesVersion?: string
  kubernetesVersionSource: FigureSource
  /** Every distinct kubelet version, present only when the nodes disagree. */
  kubeletVersions?: string[]
  architectures?: string[]
  platform?: string
  namespaces?: NamespaceCounts
  lastDiscovery?: string
}

/**
 * One resource against the allocatable pool.
 *
 * Quantities arrive formatted, so this screen and `dorgu health` render the same
 * cluster identically. Percentages arrive as numbers because a meter is sized
 * from them: a string would have to be parsed back, which is two answers to one
 * question.
 *
 * `usedPercent` being absent is the signal that nothing measured it. It is
 * optional rather than zero so that rendering an empty bar is impossible rather
 * than merely discouraged.
 */
export interface SaturationDetail {
  allocatable: string
  requested: string
  requestedPercent: number
  used?: string
  usedPercent?: number
  /** Requests are at or above 90% of allocatable, so new pods may not schedule. */
  pressure: boolean
}

export interface Saturation {
  cpu?: SaturationDetail
  memory?: SaturationDetail
  nodes: number
  scheduledPods: number
  /**
   * Pods excluded from the requested figure because no node has accepted them.
   *
   * Counting one is what made cluster health report 1689% CPU: a pod no node has
   * accepted holds no allocation, and it can ask for more than the cluster owns,
   * so the error has no upper bound.
   */
  unscheduledPods: number
  /** Why there is no used figure, ready to render inside "n/a (...)". */
  usedUnavailable?: string
  usedReadAt?: string
}

export interface NodeResources {
  cpu?: string
  memory?: string
  pods?: string
}

export interface NodePods {
  scheduled: number
  capacity: number
  percent: number
}

export interface ClusterNode {
  id: string
  name: string
  ready: boolean
  notReadyReason?: string
  /** False when cordoned, which is healthy and deliberately closed. */
  schedulable: boolean
  roles?: string[]
  taints?: string[]
  kubeletVersion?: string
  containerRuntime?: string
  os?: string
  architecture?: string
  allocatable: NodeResources
  capacity: NodeResources
  requestedCpuPercent: number
  requestedMemoryPercent: number
  pods: NodePods
  createdAt: string
}

export interface Addon {
  name: string
  type?: string
  namespace?: string
  version?: string
  installed: boolean
  healthy?: boolean
  /** Set when this process's own observation contradicts the operator's record. */
  disagreement?: string
}

export interface ClusterSummary {
  nodes: number
  nodesReady: number
  nodesUnschedulable: number
  scheduledPods: number
  unscheduledPods: number
  addonsInstalled: number
  addonsUnhealthy: number
}

export interface ClusterPayload {
  identity: ClusterIdentity
  saturation: Saturation
  nodes: ClusterNode[]
  addons?: Addon[]
  summary: ClusterSummary
  readiness: Readiness
}

// ---------------------------------------------------------------------------
// meta
// ---------------------------------------------------------------------------

export interface ClusterInfo {
  context?: string
  server: string
  namespace?: string
  inCluster: boolean
}

export interface ViewStatus {
  id: string
  label: string
  available: boolean
  reason?: string
  /**
   * What a view that does render still does not do.
   *
   * Available and finished are not the same thing, and the gap is the part a
   * reader has to be told. A screen that shows a remediation plan and cannot
   * approve it is trustworthy as long as it says so.
   */
  limitation?: string
}

export interface Meta {
  version: string
  cluster: ClusterInfo
  crds: Record<string, boolean>
  missingCRDs?: string[]
  operatorInstalled: boolean
  synced: boolean
  streamClients: number
  views: ViewStatus[]
}

/** The SSE event names, which are also the REST resource names. */
export const TOPICS = ['meta', 'apps', 'incidents', 'remediations', 'cluster'] as const
export type Topic = (typeof TOPICS)[number]

/** Maps a topic to the payload the server pushes under it. */
export interface TopicPayloads {
  meta: Meta
  apps: AppsPayload
  incidents: IncidentsPayload
  remediations: RemediationsPayload
  cluster: ClusterPayload
}
