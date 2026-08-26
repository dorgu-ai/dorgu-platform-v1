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
export const TOPICS = ['meta', 'apps', 'incidents'] as const
export type Topic = (typeof TOPICS)[number]

/** Maps a topic to the payload the server pushes under it. */
export interface TopicPayloads {
  meta: Meta
  apps: AppsPayload
  incidents: IncidentsPayload
}
