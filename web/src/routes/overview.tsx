import { useQuery } from '@tanstack/react-query'
import { Link, useSearch } from '@tanstack/react-router'
import { AlertTriangle, Info } from 'lucide-react'

import { PageHeader } from '@/components/app-shell'
import { ViewEmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { KpiTile } from '@/components/kpi-tile'
import { SeverityBadge } from '@/components/status'
import { Badge } from '@/components/ui/badge'
import { Card, CardBody, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Tooltip } from '@/components/ui/tooltip'
import {
  DEFAULT_WINDOW,
  incidentsQuery,
  overviewQuery,
  overviewWindowQuery,
  remediationsQuery,
  WINDOWS,
} from '@/lib/api'
import {
  bandSentence,
  distributionText,
  exclusionLabel,
  percent,
  secondsText,
  sumValues,
  unreadableConsequence,
  windowLabel,
} from '@/lib/report-format'
import type { Incident, OverviewPayload, Remediation, ReportDoc } from '@/lib/types'
import { absolute, age, cn } from '@/lib/utils'

/** "from RemediationActions and IncidentMemories, the last 7 days". */
function from(sources: string, window: string): string {
  return `from ${sources}, ${windowLabel(window)}`
}

export function OverviewView({ window: since = DEFAULT_WINDOW }: { window?: string }) {
  const isDefault = since === DEFAULT_WINDOW
  const live = useQuery({ ...overviewQuery, enabled: isDefault })
  const other = useQuery({ ...overviewWindowQuery(since), enabled: !isDefault })
  const { data, error } = isDefault ? live : other
  const incidents = useQuery(incidentsQuery).data
  const remediations = useQuery(remediationsQuery).data

  const picker = <WindowPicker current={since} />

  if (error) {
    return (
      <>
        <PageHeader title="Overview" trailing={picker} />
        <ErrorState title="Could not read the Overview" error={error} />
      </>
    )
  }

  const report = data?.report ?? null
  return (
    <div tabIndex={0} className="min-h-0 flex-1 overflow-y-auto outline-offset-[-2px]">
      <PageHeader
        title="Overview"
        description={
          <>
            What Dorgu detected, proposed and healed in {windowLabel(data?.window ?? since)}, computed
            from your cluster&apos;s own records by the same code as{' '}
            <code className="font-mono text-code">dorgu report</code>.
          </>
        }
        trailing={picker}
      />

      {!data && (
        <div className="px-6 pb-6" aria-busy="true">
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" data-testid="overview-loading">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-28 rounded-ctl" />
            ))}
          </div>
        </div>
      )}

      {data && !report && <ViewEmptyState readiness={data.readiness} rowCount={0} view="overview" />}

      {data && report && <ReportBody payload={data} report={report} incidents={incidents?.incidents} remediations={remediations?.remediations} />}
    </div>
  )
}

function WindowPicker({ current }: { current: string }) {
  return (
    <nav aria-label="Report window" className="flex items-center gap-1 rounded-ctl border border-line bg-surface p-0.5">
      {WINDOWS.map((w) => {
        const selected = w === current
        return (
          <Link
            key={w}
            to="/overview"
            search={w === DEFAULT_WINDOW ? {} : { since: w }}
            activeOptions={{ exact: true, includeSearch: true }}
            aria-current={selected ? 'page' : undefined}
            className={cn(
              'rounded-pill px-2.5 py-1 text-small font-medium',
              selected ? 'bg-accent-soft text-ink' : 'text-ink-muted hover:bg-surface-hover hover:text-ink',
            )}
          >
            {w}
          </Link>
        )
      })}
    </nav>
  )
}

function ReportBody({
  payload,
  report,
  incidents,
  remediations,
}: {
  payload: OverviewPayload
  report: ReportDoc
  incidents: Incident[] | undefined
  remediations: Remediation[] | undefined
}) {
  const window = report.window.since
  const heal = report.heal_accounting
  const quiet = heal.total === 0

  return (
    <div className="space-y-6 px-6 pb-8">
      {report.input_errors.length > 0 && <InputErrors errors={report.input_errors} />}

      {quiet && (
        <ViewEmptyState readiness={payload.readiness} rowCount={0} view="overview" />
      )}

      <section aria-labelledby="kpi-heading">
        <h2 id="kpi-heading" className="mb-2 text-h2 font-semibold text-ink">
          Key figures
        </h2>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <SafeHealsTile report={report} window={window} />
          <CleanHealsTile report={report} window={window} />
          <ApprovalTile report={report} window={window} />
          <PrecisionTile report={report} window={window} />
          <TimeToHealTile report={report} window={window} />
          <ReviewDepthTile report={report} window={window} />
          <RefusalsTile report={report} window={window} />
          <DeclinesTile report={report} window={window} />
        </div>
      </section>

      <div className="grid gap-4 xl:grid-cols-2">
        <OpenIncidents incidents={incidents} />
        <ProposalsWaiting remediations={remediations} />
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <HealAccounting report={report} />
        <RefusalMix report={report} />
      </div>

      <ReportFooter report={report} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// KPI tiles
// ---------------------------------------------------------------------------

type TileProps = { report: ReportDoc; window: string }

function SafeHealsTile({ report, window }: TileProps) {
  const ns = report.tier_a.ns_safe_heals
  // With nothing proposed in the window there is nothing to have healed: `dorgu report`
  // prints this section as "no data", and a 0 here would read as a measured failure.
  const empty = report.heal_accounting.total === 0
  const noData = empty || ns.no_data || ns.safe_heals === null
  return (
    <KpiTile
      label="Safe heals"
      value={noData ? 'no data' : ns.safe_heals}
      noData={noData}
      tone={!noData && (ns.safe_heals ?? 0) > 0 ? 'ok' : undefined}
      caption={
        empty
          ? 'no proposals were created in the window'
          : noData
            ? ns.reason
            : `${ns.clean_heals} clean heals in all; a safe heal also passed the approval-time gate`
      }
      provenance={from('RemediationActions, IncidentMemories and the gate record', window)}
      note="The north star: a RemediationAction that Completed, was HealCredited=True, whose incident outcome is resolved, and whose approval-time gate run passed."
    />
  )
}

function CleanHealsTile({ report, window }: TileProps) {
  const h = report.heal_accounting
  const empty = h.total === 0
  const noData = empty || h.no_data || h.clean === null
  return (
    <KpiTile
      label="Clean heals"
      value={noData ? 'no data' : h.clean}
      noData={noData}
      caption={empty ? 'no proposals were created in the window' : noData ? h.reason : `of ${h.total} proposals, ${h.completed} completed`}
      provenance={from('RemediationActions and IncidentMemories', window)}
    />
  )
}

function ApprovalTile({ report, window }: TileProps) {
  const a = report.tier_a.a6_approval_rate
  let value: string
  let caption: string
  if (a.no_data) {
    value = 'no data'
    caption = 'no proposal was approved or rejected'
  } else if (a.too_few_decisions) {
    value = `${a.approved} of ${a.approved + a.rejected}`
    caption = `approved; too few decisions (n=${a.approved + a.rejected}) to judge`
  } else {
    value = percent(a.rate)
    caption = `${a.approved} approved, ${a.rejected} rejected; ${bandSentence(a.band)}`
  }
  return (
    <KpiTile
      label="Approval rate"
      value={value}
      noData={a.no_data}
      caption={caption}
      provenance={from('RemediationActions', window)}
      note="A band, not a target: under 30% means noisy proposals, over 80% means rubber-stamping."
    />
  )
}

function PrecisionTile({ report, window }: TileProps) {
  const p = report.tier_q.q2_proposal_precision
  let value: string
  let caption: string
  if (p.no_data) {
    value = 'no data'
    caption = 'no proposal was decided'
  } else if (p.too_few_decisions) {
    value = `${p.wrong} of ${p.decided}`
    caption = `called wrong-diagnosis; too few decisions (n=${p.decided}) to judge`
  } else {
    value = percent(p.rate)
    caption = `${p.wrong} of ${p.decided} decided proposals called wrong-diagnosis`
  }
  return (
    <KpiTile
      label="Called wrong"
      value={value}
      noData={p.no_data}
      caption={caption}
      provenance={from('RemediationActions and rejection reasons', window)}
      note="Only a rejection with the reason wrong-diagnosis counts: the other reasons say the change was risky, mistimed or unnecessary, not that Dorgu misread the problem."
    />
  )
}

function TimeToHealTile({ report, window }: TileProps) {
  const d = report.tier_q.q1_time_to_heal.detect_to_proposal
  const noData = d.no_data || d.p50_seconds === null
  return (
    <KpiTile
      label="Detect to proposal"
      value={noData ? 'no data' : secondsText(d.p50_seconds ?? 0)}
      noData={noData}
      caption={noData ? (d.reason ?? 'n=0') : `p50 of ${d.n}; p90 ${secondsText(d.p90_seconds ?? 0)}`}
      provenance={from('IncidentMemories and RemediationActions', window)}
      note={`Machine latency, one sample per incident. All four stages: detect to proposal ${distributionText(d)}.`}
    />
  )
}

function ReviewDepthTile({ report, window }: TileProps) {
  const a = report.tier_a.a5_review_depth
  const noData = a.no_data || a.per_week === null
  return (
    <KpiTile
      label="Decisions per week"
      value={noData ? 'no data' : (a.per_week ?? 0).toFixed(1)}
      noData={noData}
      caption={noData ? 'no proposal was decided' : `${a.proposals_decided} over ${Math.round(a.window_days)} days`}
      provenance={from('RemediationActions', window)}
      note="Reading a proposal with the CLI leaves no record, so a decision is the measurable proxy for a review."
    />
  )
}

function RefusalsTile({ report, window }: TileProps) {
  const q = report.tier_q.q6_refusal_mix
  const refused = q.rows.reduce((n, r) => n + r.count, 0)
  const noData = q.no_data
  return (
    <KpiTile
      label="Refused by the gate"
      value={noData ? 'no data' : refused}
      noData={noData}
      tone={!noData && refused > 0 ? 'warn' : undefined}
      caption={
        noData
          ? (q.reason ?? 'no proposals in the window')
          : `of ${q.proposals} proposals. Deferrals are not refusals: ${q.deferred_proposals === null ? 'not recorded' : `${q.deferred_proposals} counted`}`
      }
      provenance={from('RemediationActions and IncidentMemories', window)}
      note={q.deferred_note}
    />
  )
}

function DeclinesTile({ report, window }: TileProps) {
  const d = report.cli_declines
  const total = sumValues(d.by_reason)
  return (
    <KpiTile
      label="Declined by the CLI"
      value={d.no_data ? 'not recorded' : total}
      noData={d.no_data}
      caption={d.no_data ? d.reason : 'Events the CLI wrote when it declined to act'}
      provenance={from('Kubernetes Events, which expire after about an hour', window)}
      note={d.note}
    />
  )
}

// ---------------------------------------------------------------------------
// Lists and tables
// ---------------------------------------------------------------------------

function InputErrors({ errors }: { errors: ReportDoc['input_errors'] }) {
  return (
    <div
      role="alert"
      className="flex items-start gap-3 rounded-ctl border border-warn/40 bg-warn-soft p-3"
      data-testid="input-errors"
    >
      <AlertTriangle className="mt-0.5 size-4 shrink-0 text-warn" aria-hidden="true" />
      <ul className="space-y-1 text-body text-ink">
        {errors.map((e) => (
          <li key={e.input}>
            <span className="font-medium">Could not read {e.input.replace('_', ' ')}</span> ({e.error}):{' '}
            {unreadableConsequence(e.input)}.
          </li>
        ))}
      </ul>
    </div>
  )
}

const OPEN_LIMIT = 5

function OpenIncidents({ incidents }: { incidents: Incident[] | undefined }) {
  const open = incidents?.filter((i) => i.phase !== 'Resolved') ?? []
  return (
    <Card>
      <CardHeader className="flex items-center justify-between gap-3">
        <CardTitle>Open incidents</CardTitle>
        <Link to="/incidents" className="text-small font-medium text-accent hover:underline">
          View all
        </Link>
      </CardHeader>
      <CardBody>
        {!incidents ? (
          <Skeleton className="h-16" />
        ) : open.length === 0 ? (
          <p className="text-body text-ink-muted">No open incidents. Resolved ones are in Memory.</p>
        ) : (
          <ul className="divide-y divide-line" aria-label="Open incidents">
            {open.slice(0, OPEN_LIMIT).map((i) => (
              <li key={i.id} className="flex items-center gap-3 py-2 first:pt-0 last:pb-0">
                <SeverityBadge severity={i.severity} />
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-table text-ink">{i.name}</p>
                  <p className="truncate text-small text-ink-muted">
                    {i.signal} in {i.namespace}
                  </p>
                </div>
                <Tooltip content={`Last seen ${absolute(i.lastSeen)}`}>
                  <span className="shrink-0 cursor-default text-small text-ink-muted">{age(i.lastSeen)}</span>
                </Tooltip>
              </li>
            ))}
          </ul>
        )}
        {open.length > OPEN_LIMIT && (
          <p className="mt-2 text-small text-ink-muted">and {open.length - OPEN_LIMIT} more.</p>
        )}
      </CardBody>
    </Card>
  )
}

function ProposalsWaiting({ remediations }: { remediations: Remediation[] | undefined }) {
  const pending = remediations?.filter((r) => r.phase === 'Pending') ?? []
  const expired = remediations?.filter((r) => r.phase === 'Expired') ?? []
  return (
    <Card>
      <CardHeader className="flex items-center justify-between gap-3">
        <CardTitle>Proposals waiting and expired</CardTitle>
        <Link to="/remediations" className="text-small font-medium text-accent hover:underline">
          View all
        </Link>
      </CardHeader>
      <CardBody className="space-y-3">
        {!remediations ? (
          <Skeleton className="h-16" />
        ) : (
          <>
            <ul className="divide-y divide-line" aria-label="Pending and expired proposals">
              {[...pending, ...expired].slice(0, OPEN_LIMIT).map((r) => (
                <li key={r.id} className="flex items-center gap-3 py-2 first:pt-0 last:pb-0">
                  <Badge tone={r.phase === 'Expired' ? 'warn' : 'neutral'}>
                    {r.phase === 'Expired' ? 'Expired' : 'Pending'}
                  </Badge>
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-table text-ink">{r.name}</p>
                    <p className="truncate text-small text-ink-muted">
                      {r.phase === 'Expired' && r.expired
                        ? `expired: ${r.expired.reason}`
                        : `${r.namespace}/${r.persona.name}, ${r.advisoryOnly ? 'advisory only' : 'appliable'}`}
                    </p>
                  </div>
                  <span className="shrink-0 text-small text-ink-muted">{age(r.createdAt)}</span>
                </li>
              ))}
            </ul>
            {pending.length + expired.length === 0 && (
              <p className="text-body text-ink-muted">Nothing is waiting for a decision, and nothing expired.</p>
            )}
          </>
        )}
      </CardBody>
    </Card>
  )
}

function HealAccounting({ report }: { report: ReportDoc }) {
  const h = report.heal_accounting
  return (
    <Card>
      <CardHeader>
        <CardTitle>Where {h.total} proposals ended</CardTitle>
      </CardHeader>
      <CardBody>
        {h.total === 0 ? (
          <p className="text-body text-ink-muted">no data: no RemediationActions were created in the window</p>
        ) : h.no_data ? (
          <p className="flex items-start gap-2 text-body text-ink-muted">
            <Info className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>
              {h.completed} completed. Clean heals: no data ({h.reason})
            </span>
          </p>
        ) : (
          <table className="w-full text-table">
            <caption className="sr-only">Every proposal is a clean heal or excluded for one reason</caption>
            <thead className="text-left text-small text-ink-muted">
              <tr>
                <th scope="col" className="pb-1 font-medium">Outcome</th>
                <th scope="col" className="pb-1 text-right font-medium">Proposals</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              <tr>
                <th scope="row" className="py-1 text-left font-medium text-ink">clean heal</th>
                <td className="py-1 text-right text-ink">{h.clean}</td>
              </tr>
              {Object.entries(h.excluded)
                .filter(([, n]) => n > 0)
                .map(([k, n]) => (
                  <tr key={k}>
                    <th scope="row" className="py-1 text-left font-normal text-ink-muted">{exclusionLabel(k)}</th>
                    <td className="py-1 text-right text-ink">{n}</td>
                  </tr>
                ))}
            </tbody>
          </table>
        )}
      </CardBody>
    </Card>
  )
}

function RefusalMix({ report }: { report: ReportDoc }) {
  const q = report.tier_q.q6_refusal_mix
  return (
    <Card>
      <CardHeader>
        <CardTitle>Refusals and deferrals</CardTitle>
      </CardHeader>
      <CardBody className="space-y-3">
        {q.no_data ? (
          <p className="text-body text-ink-muted">no data ({q.reason ?? 'no proposals in the window'})</p>
        ) : q.rows.length === 0 ? (
          <p className="text-body text-ink-muted">no refusals across {q.proposals} proposals</p>
        ) : (
          <table className="w-full text-table">
            <caption className="sr-only">What the gate refused, by phase, stage and reason</caption>
            <thead className="text-left text-small text-ink-muted">
              <tr>
                <th scope="col" className="pb-1 font-medium">Phase</th>
                <th scope="col" className="pb-1 font-medium">Stage and reason</th>
                <th scope="col" className="pb-1 text-right font-medium">Count</th>
                <th scope="col" className="pb-1 text-right font-medium">Share</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {q.rows.map((r) => (
                <tr key={`${r.phase}/${r.stage}/${r.reason}`}>
                  <td className="py-1 text-ink-muted">{r.phase}</td>
                  <td className="py-1 font-mono text-code text-ink">
                    {r.stage}/{r.reason}
                    {r.flagged && (
                      <Badge tone="warn" className="ml-2">
                        over 30% of proposals
                      </Badge>
                    )}
                  </td>
                  <td className="py-1 text-right text-ink">{r.count}</td>
                  <td className="py-1 text-right text-ink-muted">{percent(r.share_of_proposals)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <p className="text-small text-ink-muted" data-testid="deferred-note">
          <span className="font-medium text-ink">Deferrals are not refusals.</span> Deferred proposals:{' '}
          {q.deferred_proposals === null ? q.deferred_note : `${q.deferred_proposals} incident(s) with a proposal waiting for a decision. Otherwise ${q.deferred_note}`}
        </p>
      </CardBody>
    </Card>
  )
}

function ReportFooter({ report }: { report: ReportDoc }) {
  const sources = report.sources
  const part = (name: string, s: { ok: boolean; count: number; error?: string }) =>
    s.ok ? `${name} ${s.count}` : `${name} unreadable`
  return (
    <section aria-label="Where these figures came from" className="space-y-1 text-small text-ink-muted">
      <p>
        Computed {absolute(report.generated_at)} for {absolute(report.window.start)} to{' '}
        {absolute(report.window.end)}. Cluster {report.cluster.context || 'unknown'}, namespace scope{' '}
        {report.cluster.namespace}, operator {report.cluster.operator_version ?? 'unknown (not readable)'}.
      </p>
      <p>
        Read: {part('RemediationActions', sources.remediation_actions)},{' '}
        {part('IncidentMemories', sources.incidents)}, {part('personas', sources.personas)},{' '}
        {part('ClusterPersonas', sources.cluster_personas)}, {part('Events', sources.events)}. Proposals and
        incidents count by when they were created, Events by their last timestamp.
      </p>
      <details>
        <summary className="cursor-pointer text-ink">Not measurable from a cluster</summary>
        <ul className="mt-1 space-y-0.5">
          {report.not_measurable.map((n) => (
            <li key={n.id}>
              <span className="font-mono text-ink">{n.id}</span> {n.name}: {n.reason}
            </li>
          ))}
        </ul>
      </details>
    </section>
  )
}

/** The route's component: the window comes from `?since=`, and 7d is the default the stream keeps current. */
export function OverviewRoute() {
  const { since } = useSearch({ strict: false })
  return <OverviewView window={since && WINDOWS.includes(since as (typeof WINDOWS)[number]) ? since : DEFAULT_WINDOW} />
}
