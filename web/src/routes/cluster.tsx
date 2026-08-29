import { useQuery } from '@tanstack/react-query'
import { createColumnHelper } from '@tanstack/react-table'
import { AlertCircle, Ban, Cpu } from 'lucide-react'
import { useMemo } from 'react'

import { DataTable, type ColumnMeta } from '@/components/data-table'
import { ViewEmptyState } from '@/components/empty-state'
import { Dot } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Card, CardBody, CardHeader, CardTitle } from '@/components/ui/card'
import { InlineMeter, Meter, MeterUnavailable, type MeterTone } from '@/components/ui/meter'
import { StatRow, StatTile } from '@/components/ui/stat-tile'
import { Tooltip } from '@/components/ui/tooltip'
import { clusterQuery } from '@/lib/api'
import type { Addon, ClusterNode, FigureSource, SaturationDetail } from '@/lib/types'
import { absolute, age } from '@/lib/utils'

/**
 * The Cluster view.
 *
 * This screen was gated for a release because cluster health reported 1689% CPU
 * on a cluster where 25% was requested and 1% was in use, and a wrong number in
 * a card is read as authoritative in a way terminal output is not. Two habits
 * come out of that and they are visible in every figure below:
 *
 *  1. requested and used are two numbers, never one, because they answer
 *     different questions and used to share a line
 *  2. every figure says whether the dashboard observed it or the operator
 *     recorded it, using the same words the Apps view uses for health
 *
 * There is no chart on this page. Nothing here is a series over time, and this
 * process holds only current state, so a trend line would be invented rather
 * than measured.
 */

/**
 * Where a figure came from, in one word plus an explanation.
 *
 * The vocabulary is the Apps view's on purpose: a reader who has learned what
 * "observed" means on one screen must not have to learn it again on another.
 */
const sourceNote: Record<FigureSource, string> = {
  observed:
    'Read by this dashboard from live Nodes and Pods, not from the operator. It cannot be stale and does not depend on which operator version is installed.',
  'persona-status':
    'Written by the Dorgu operator into ClusterPersona.status on a reconcile interval, so it can be behind the cluster.',
  none: 'Nothing to derive this from.',
}

function SourceBadge({ source }: { source: FigureSource }) {
  if (source === 'none') return null
  return (
    <Tooltip content={sourceNote[source]}>
      <Badge tone={source === 'observed' ? 'ok' : 'neutral'} className="cursor-default font-mono">
        {source}
      </Badge>
    </Tooltip>
  )
}

export function ClusterView() {
  const { data, error } = useQuery(clusterQuery)
  const nodes = useMemo(() => data?.nodes ?? [], [data])

  if (error) {
    return (
      <div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
        <AlertCircle className="size-6 text-danger" />
        <h3 className="text-sm font-semibold text-ink">Could not read the Cluster view</h3>
        <p className="max-w-lg text-xs text-ink-muted">{error.message}</p>
      </div>
    )
  }

  const empty = (
    <ViewEmptyState readiness={data?.readiness} rowCount={nodes.length} view="cluster" />
  )
  // The node table is the only part gated on having nodes. Saturation and the
  // add-on list would render an empty frame that says nothing, so on a cluster
  // whose nodes could not be read the whole screen becomes the explanation.
  if (data && (!data.readiness.synced || nodes.length === 0)) {
    return <div className="min-h-0 flex-1 overflow-auto">{empty}</div>
  }

  return (
    <div className="min-h-0 flex-1 overflow-auto">
      <Identity />
      <div className="grid gap-3 px-4 pb-4 lg:grid-cols-2">
        <SaturationCard />
        <AddonsCard />
      </div>
      <NodeTable nodes={nodes} emptyState={empty} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// identity
// ---------------------------------------------------------------------------

/**
 * Which cluster this is, as a row of stat tiles.
 *
 * A handful of headline numbers is a KPI row, not a chart. None of them carries a
 * delta, because there is no previous value to compare against and a fabricated
 * one would be worse than none.
 */
function Identity() {
  const { data } = useQuery(clusterQuery)
  if (!data) return null

  const { identity, summary, saturation } = data
  const mixedVersions = (identity.kubeletVersions?.length ?? 0) > 1

  return (
    <>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 border-b border-line px-4 py-2.5">
        <span className="text-sm font-semibold text-ink">
          {identity.name || 'This cluster has no ClusterPersona'}
        </span>
        {identity.environment && (
          <Badge tone={identity.environment === 'production' ? 'warn' : 'neutral'}>
            {identity.environment}
          </Badge>
        )}
        {identity.platform && (
          <span className="font-mono text-[11px] text-ink-faint">{identity.platform}</span>
        )}
        {!identity.personaPresent && (
          <Tooltip content="The operator has not described this cluster, so the name, the environment and the add-on list are empty. Nodes, capacity and saturation are read from your cluster directly and are unaffected.">
            <span className="cursor-default text-[11px] text-ink-muted">
              nodes and saturation are read directly, so this screen still works
            </span>
          </Tooltip>
        )}
        {identity.lastDiscovery && (
          <Tooltip
            content={`The operator last described this cluster ${absolute(identity.lastDiscovery)}. Every persona-sourced figure on this screen is at most that fresh.`}
          >
            <span className="ml-auto cursor-default text-[11px] text-ink-faint">
              operator discovery {age(identity.lastDiscovery)} ago
            </span>
          </Tooltip>
        )}
      </div>

      <StatRow>
        <StatTile
          label="Nodes"
          value={summary.nodes}
          detail={`${summary.nodesReady} ready`}
          tone={summary.nodesReady < summary.nodes ? 'danger' : undefined}
          note="Counted from the live Node list this dashboard watches."
        />
        <StatTile
          label="Kubernetes"
          value={identity.kubernetesVersion || 'unknown'}
          detail={
            mixedVersions ? (
              <span className="text-warn">{identity.kubeletVersions?.join(', ')}</span>
            ) : (
              <SourceBadge source={identity.kubernetesVersionSource} />
            )
          }
          tone={mixedVersions ? 'warn' : undefined}
          note={
            mixedVersions
              ? 'The nodes do not agree on a kubelet version, which is what a cluster mid-upgrade looks like. Both versions are listed rather than collapsed to one.'
              : sourceNote[identity.kubernetesVersionSource]
          }
        />
        <StatTile
          label="Architecture"
          value={identity.architectures?.join(', ') || 'unknown'}
          note="Read off the nodes. It matters because a container image built for one architecture cannot run on the other: every Dorgu operator image before v0.11.1 was amd64-only, which left arm64 nodes in ImagePullBackOff."
        />
        <StatTile
          label="Scheduled pods"
          value={summary.scheduledPods}
          detail="hold an allocation"
          note="Pods a node has accepted. These are the only ones counted in the requested figures, because they are the only ones holding anything."
        />
        <StatTile
          label="Unscheduled pods"
          value={summary.unscheduledPods}
          tone={summary.unscheduledPods > 0 ? 'warn' : undefined}
          detail={summary.unscheduledPods > 0 ? 'excluded, see below' : 'none waiting'}
          note="Pods no node has accepted. They hold no allocation anywhere, so they are excluded from saturation. Counting them is what made cluster health report 1689% CPU."
        />
        <StatTile
          label="Namespaces"
          value={identity.namespaces?.total ?? 'n/a'}
          detail={
            identity.namespaces ? (
              <>
                {identity.namespaces.withPersonas} with a persona{' '}
                <SourceBadge source="persona-status" />
              </>
            ) : (
              'no operator record'
            )
          }
          note={
            identity.namespaces
              ? sourceNote['persona-status']
              : 'The dashboard does not watch Namespaces, so this count comes from the operator or not at all.'
          }
        />
      </StatRow>

      {saturation.unscheduledPods > 0 && <UnscheduledNotice count={saturation.unscheduledPods} />}
    </>
  )
}

/**
 * The exclusion, stated where the numbers are.
 *
 * These pods are the real problem the old percentage was burying, so they get a
 * sentence rather than a footnote.
 */
function UnscheduledNotice({ count }: { count: number }) {
  return (
    <div className="mx-4 mb-3 rounded-lg border border-warn/40 bg-warn-soft px-3 py-2 text-[11px]">
      <p className="font-medium text-warn">
        <Ban className="mr-1 inline size-3" aria-hidden="true" />
        {count === 1
          ? '1 pod is not scheduled onto any node, so it is excluded from the figures below.'
          : `${count} pods are not scheduled onto any node, so they are excluded from the figures below.`}
      </p>
      <p className="mt-0.5 text-ink-muted">
        A pod no node has accepted holds no allocation. Counting one would inflate saturation
        without limit, because it can request more than the cluster owns.
      </p>
    </div>
  )
}

// ---------------------------------------------------------------------------
// saturation
// ---------------------------------------------------------------------------

/**
 * Requested and used, as four meters against one allocatable pool.
 *
 * They are separate rows rather than two colours on one bar, so identity comes
 * from the label and colour is left free to carry severity. That also means the
 * two are told apart without relying on colour at all, and an absent used figure
 * can drop its bar entirely instead of drawing a zero.
 */
function SaturationCard() {
  const { data } = useQuery(clusterQuery)
  if (!data) return null

  const { saturation } = data

  return (
    <Card>
      <CardHeader className="flex flex-wrap items-center gap-2">
        <CardTitle>
          <Cpu className="mr-1.5 inline size-3.5 text-ink-faint" aria-hidden="true" />
          Saturation
        </CardTitle>
        <SourceBadge source="observed" />
        <span className="ml-auto text-[11px] text-ink-faint">
          {saturation.nodes} node{saturation.nodes === 1 ? '' : 's'}, {saturation.scheduledPods}{' '}
          scheduled pod{saturation.scheduledPods === 1 ? '' : 's'}
        </span>
      </CardHeader>
      <CardBody className="space-y-4">
        {saturation.cpu && (
          <ResourceMeters
            name="CPU"
            detail={saturation.cpu}
            usedUnavailable={saturation.usedUnavailable}
          />
        )}
        {saturation.memory && (
          <ResourceMeters
            name="Memory"
            detail={saturation.memory}
            usedUnavailable={saturation.usedUnavailable}
          />
        )}
        {!saturation.cpu && !saturation.memory && (
          <p className="text-xs text-ink-muted">
            The nodes report no allocatable CPU or memory, so there is no pool for anything to be a
            share of.
          </p>
        )}

        <p className="text-[11px] leading-relaxed text-ink-muted">
          Requested is what the scheduler has committed. Used is what the containers are consuming.
          They are two numbers because they answer different questions: requests near allocatable
          means nothing more will schedule, and used near allocatable means what is already running
          is struggling.
        </p>
        {saturation.usedReadAt && (
          <p className="text-[11px] text-ink-faint">
            Used figures measured {age(saturation.usedReadAt)} ago. metrics-server is polled,
            because its API serves no watch.
          </p>
        )}
      </CardBody>
    </Card>
  )
}

/** One resource, as a requested meter and a used meter over the same pool. */
function ResourceMeters({
  name,
  detail,
  usedUnavailable,
}: {
  name: string
  detail: SaturationDetail
  usedUnavailable?: string
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline gap-2">
        <span className="text-xs font-medium text-ink">{name}</span>
        <span className="font-mono text-[11px] text-ink-faint">
          {detail.allocatable} allocatable
        </span>
        {detail.pressure && (
          <Tooltip content="At or above 90% of allocatable requested. New pods may not schedule onto this cluster, and the reader should not have to do the division.">
            <span className="flex cursor-default items-center gap-1 text-[11px] text-warn">
              <Dot tone="warn" />
              new pods may not schedule
            </span>
          </Tooltip>
        )}
      </div>

      <Meter
        label="requested"
        emphasis
        percent={detail.requestedPercent}
        tone={pressureTone(detail.requestedPercent)}
        value={`${detail.requested} / ${detail.allocatable}  ${percent(detail.requestedPercent)}`}
        note="What the scheduler has committed to the pods a node has accepted. This is the number that decides whether anything else can be placed."
      />

      {detail.usedPercent === undefined ? (
        <MeterUnavailable label="used" reason={usedUnavailable ?? 'not reported'} />
      ) : (
        <Meter
          label="used"
          percent={detail.usedPercent}
          tone={pressureTone(detail.usedPercent)}
          value={`${detail.used} / ${detail.allocatable}  ${percent(detail.usedPercent)}`}
          note="What the containers are actually consuming, read from metrics-server. It is polled rather than watched, because the metrics API serves no watch verb."
        />
      )}
    </div>
  )
}

/**
 * Colour here means state, not identity, so it uses the reserved status tokens.
 *
 * The 90% threshold is the same one the CLI warns at, so the two agree about the
 * same cluster.
 */
function pressureTone(value: number): MeterTone {
  if (value >= 90) return 'danger'
  if (value >= 75) return 'warn'
  return 'ok'
}

/** One decimal below 10%, whole numbers above, so a small share is still legible. */
function percent(value: number): string {
  return value < 10 ? `${value.toFixed(1)}%` : `${Math.round(value)}%`
}

// ---------------------------------------------------------------------------
// add-ons
// ---------------------------------------------------------------------------

function AddonsCard() {
  const { data } = useQuery(clusterQuery)
  if (!data) return null

  const addons = data.addons ?? []

  return (
    <Card>
      <CardHeader className="flex flex-wrap items-center gap-2">
        <CardTitle>Add-ons</CardTitle>
        <SourceBadge source="persona-status" />
        {addons.length > 0 && (
          <span className="ml-auto text-[11px] text-ink-faint">
            {data.summary.addonsInstalled} installed
          </span>
        )}
      </CardHeader>
      <CardBody>
        {addons.length === 0 ? (
          <p className="text-xs text-ink-muted">
            {data.identity.personaPresent
              ? 'The operator has not recorded any add-ons for this cluster yet.'
              : 'Add-on discovery is the operator’s. Without a ClusterPersona there is no list, which is a statement about the operator rather than about your cluster.'}
          </p>
        ) : (
          <ul className="space-y-1.5">
            {addons.map((addon) => (
              <AddonRow key={addon.name} addon={addon} />
            ))}
          </ul>
        )}
      </CardBody>
    </Card>
  )
}

/**
 * One add-on.
 *
 * Status wears an icon and a label as well as a colour, never colour alone.
 */
function AddonRow({ addon }: { addon: Addon }) {
  const tone = !addon.installed
    ? 'neutral'
    : addon.disagreement
      ? 'warn'
      : addon.healthy === false
        ? 'danger'
        : addon.healthy === true
          ? 'ok'
          : 'neutral'

  const state = !addon.installed
    ? 'not installed'
    : addon.healthy === false
      ? 'unhealthy'
      : addon.healthy === true
        ? 'healthy'
        : 'installed'

  return (
    <li className="min-w-0">
      <div className="flex min-w-0 items-center gap-2 text-xs">
        <Dot tone={tone} />
        <span className="truncate font-mono text-ink">{addon.name}</span>
        {addon.version && <span className="text-[11px] text-ink-faint">{addon.version}</span>}
        <Badge tone={tone} className="ml-auto">
          {state}
        </Badge>
      </div>
      {/*
        The dashboard's own observation contradicting the operator's record. It is
        on the row rather than in a tooltip for the same reason the Apps view puts
        "operator says otherwise" on the row: a reader deciding whether to trust
        this screen needs it without hovering.
      */}
      {addon.disagreement && (
        <p className="mt-0.5 ml-3.5 text-[11px] leading-relaxed text-warn">{addon.disagreement}</p>
      )}
    </li>
  )
}

// ---------------------------------------------------------------------------
// nodes
// ---------------------------------------------------------------------------

const column = createColumnHelper<ClusterNode>()

const columns = [
  column.accessor('name', {
    header: 'Node',
    meta: { width: 'minmax(14rem, 1.6fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <NodeCell node={row.original} />,
  }),
  column.accessor('ready', {
    header: 'State',
    // Wide enough for the NotReady reason to render as visible text. A node that
    // is down is the one row on this screen somebody is reading for a reason,
    // and the reason must not be hover-only.
    meta: { width: 'minmax(11rem, 0.9fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <StateCell node={row.original} />,
  }),
  column.accessor('requestedCpuPercent', {
    header: 'CPU requested',
    meta: { width: 'minmax(11rem, 1fr)' } satisfies ColumnMeta,
    cell: ({ row }) => (
      <NodeMeterCell
        percent={row.original.requestedCpuPercent}
        allocatable={row.original.allocatable.cpu}
      />
    ),
  }),
  column.accessor('requestedMemoryPercent', {
    header: 'Memory requested',
    meta: { width: 'minmax(11rem, 1fr)' } satisfies ColumnMeta,
    cell: ({ row }) => (
      <NodeMeterCell
        percent={row.original.requestedMemoryPercent}
        allocatable={row.original.allocatable.memory}
      />
    ),
  }),
  column.accessor((node) => node.pods.scheduled, {
    id: 'pods',
    header: 'Pods',
    meta: { width: '7rem', numeric: true } satisfies ColumnMeta,
    cell: ({ row }) => <PodsCell node={row.original} />,
  }),
  column.accessor('kubeletVersion', {
    header: 'Kubelet',
    meta: { width: 'minmax(8rem, 0.8fr)' } satisfies ColumnMeta,
    cell: ({ row }) => <KubeletCell node={row.original} />,
  }),
]

/**
 * The node table.
 *
 * A table rather than a chart: nodes are nominal, so colouring them by value
 * would spend the identity channel re-encoding what the numbers already say. It
 * is also the table view for every per-node figure, so nothing on this screen is
 * reachable only by hovering.
 */
function NodeTable({ nodes, emptyState }: { nodes: ClusterNode[]; emptyState: React.ReactNode }) {
  return (
    <div className="flex min-h-[16rem] flex-col border-t border-line">
      <DataTable
        ariaLabel="Nodes"
        columns={columns}
        data={nodes}
        emptyState={emptyState}
        estimatedRowHeight={56}
      />
    </div>
  )
}

function NodeCell({ node }: { node: ClusterNode }) {
  return (
    <div className="min-w-0">
      <div className="truncate font-mono text-ink">{node.name}</div>
      <div className="flex min-w-0 items-center gap-1.5">
        {(node.roles ?? []).map((role) => (
          <Badge key={role} tone="neutral" className="shrink-0">
            {role}
          </Badge>
        ))}
        {node.architecture && (
          <span className="shrink-0 text-[11px] text-ink-faint">{node.architecture}</span>
        )}
        {/*
          The taints themselves, not just how many. A tainted node with free
          capacity is why a pod is unschedulable on a cluster that looks empty,
          and which taint it is decides what to do about it.

          Shown without their domain prefix, because `node.kubernetes.io/` is on
          most of them and identifies none of them. The full keys are in the
          tooltip, which is an addition here rather than the only way to read it.
        */}
        {(node.taints?.length ?? 0) > 0 && (
          <Tooltip content={`Taints: ${node.taints?.join(', ')}`}>
            <span className="min-w-0 cursor-default truncate font-mono text-[10px] text-warn">
              {node.taints?.slice(0, 2).map(shortTaint).join(', ')}
              {(node.taints?.length ?? 0) > 2 && ` +${(node.taints?.length ?? 0) - 2}`}
            </span>
          </Tooltip>
        )}
      </div>
    </div>
  )
}

/**
 * Drops the domain prefix from a taint key, keeping the part that identifies it.
 *
 * `node.kubernetes.io/unschedulable:NoSchedule` becomes
 * `unschedulable:NoSchedule`. Every well-known taint carries that prefix, so it
 * costs a third of the row and distinguishes nothing.
 */
function shortTaint(taint: string): string {
  const slash = taint.indexOf('/')
  return slash === -1 ? taint : taint.slice(slash + 1)
}

/**
 * Ready and schedulable are separate facts.
 *
 * A cordoned node is healthy and deliberately closed, and folding it into Ready
 * would report a maintenance window as a fault.
 */
function StateCell({ node }: { node: ClusterNode }) {
  return (
    <div className="flex min-w-0 flex-col items-start gap-0.5">
      <Badge tone={node.ready ? 'ok' : 'danger'}>
        <Dot tone={node.ready ? 'ok' : 'danger'} pulse={!node.ready} />
        {node.ready ? 'Ready' : 'NotReady'}
      </Badge>
      {/*
        The reason is rendered, not tucked into a tooltip. "NotReady" on its own
        is the half of the fact that cannot be acted on, and this screen exists
        to stop a number or a badge standing in for the thing it is about.
      */}
      {!node.ready && node.notReadyReason && (
        <span className="text-[10px] leading-snug text-danger">{node.notReadyReason}</span>
      )}
      {!node.schedulable && (
        <Tooltip content="Cordoned. The node is healthy and deliberately closed to new pods, which is a maintenance state rather than a fault.">
          <span className="cursor-default text-[10px] text-warn">cordoned</span>
        </Tooltip>
      )}
    </div>
  )
}

/**
 * Per-node saturation.
 *
 * A cluster at 40% with one node at 98% is the shape that stops a rollout, and
 * the cluster-wide figure cannot show it.
 */
function NodeMeterCell({ percent: value, allocatable }: { percent: number; allocatable?: string }) {
  return (
    <InlineMeter
      percent={value}
      tone={pressureTone(value)}
      value={`${percent(value)} of ${allocatable ?? 'n/a'}`}
      note="What the pods on this node have claimed from it, as a share of its allocatable pool. A cluster at 40% with one node at 98% is the shape that stops a rollout, and the cluster-wide figure cannot show it."
    />
  )
}

/** Pod count is a real scheduling limit and is invisible in CPU and memory. */
function PodsCell({ node }: { node: ClusterNode }) {
  return (
    <Tooltip content="Pods this node is carrying against how many it will take. A node can refuse a pod on this limit while its CPU and memory look idle.">
      <span className="cursor-default font-mono text-ink-muted">
        {node.pods.scheduled}
        <span className="text-ink-faint"> / {node.pods.capacity || 'n/a'}</span>
      </span>
    </Tooltip>
  )
}

function KubeletCell({ node }: { node: ClusterNode }) {
  return (
    <div className="min-w-0">
      <div className="truncate font-mono text-[11px] text-ink-muted">
        {node.kubeletVersion || 'unknown'}
      </div>
      {node.containerRuntime && (
        <div className="truncate text-[10px] text-ink-faint">{node.containerRuntime}</div>
      )}
    </div>
  )
}
