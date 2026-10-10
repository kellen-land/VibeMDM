import { useCallback, useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ChevronRight, Eye, HeartPulse, Play, RefreshCw, Wand2, Wrench } from 'lucide-react'
import { qk, useHealthChecks, useHealthEvents, useHealthFindings, useHealthSummary, useInvalidate } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { HealthCheckInfo, HealthFinding, HealthGroup, HealthSeverity, HealthSummary } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardHeader } from '../../components/ui/Card'
import { Table } from '../../components/ui/Table'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Tabs } from '../../components/ui/Tabs'
import { Toggle } from '../../components/ui/Checkbox'
import { Select } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock, Spinner } from '../../components/ui/Spinner'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { useToast } from '../../components/ui/Toast'
import { FindingDrawer } from '../../components/health/FindingDrawer'
import {
  EventIcon,
  ObjectName,
  SeverityBadge,
  SeverityIcon,
  SinceTime,
  SnoozeMenu,
  StateBadge,
  StatusLine,
  attentionCount,
  eventMeta,
  objectNoun,
  objectTypeLabel,
  sevMeta,
  severities,
  severityMeta,
  sharedRunnable,
  useHealthActions,
} from '../../components/health/health'
import { formatDuration, parseDate, titleCase } from '../../lib/format'
import { cn } from '../../lib/cn'

type TabKey = 'issues' | 'checks' | 'activity' | 'resolved'
const tabKeys: TabKey[] = ['issues', 'checks', 'activity', 'resolved']

export const categoryOrder = ['platform', 'fleet', 'configuration', 'security']
const categoryBlurb: Record<string, string> = {
  platform: 'Certificates, push, and server settings',
  fleet: 'How the Macs themselves are doing',
  configuration: 'Policies, profiles, apps, and plans that cannot work as configured',
  security: 'Encryption, escrow, and risky settings',
}

function byCategory<T extends { category: string }>(items: T[]): [string, T[]][] {
  const m = new Map<string, T[]>()
  for (const it of items) m.set(it.category, [...(m.get(it.category) ?? []), it])
  return [...m.entries()].sort(([a], [b]) => (categoryOrder.indexOf(a) + 1 || 99) - (categoryOrder.indexOf(b) + 1 || 99))
}

export function HealthPage() {
  const { tab } = useParams()
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const qc = useQueryClient()
  const invalidate = useInvalidate()
  const summary = useHealthSummary()
  const [running, setRunning] = useState(false)
  const active: TabKey = tabKeys.includes(tab as TabKey) ? (tab as TabKey) : 'issues'
  const findingId = sp.get('finding')

  const openFinding = useCallback(
    (id: string) =>
      setSp((p) => {
        const n = new URLSearchParams(p)
        n.set('finding', id)
        return n
      }),
    [setSp],
  )
  const closeFinding = useCallback(
    () =>
      setSp(
        (p) => {
          const n = new URLSearchParams(p)
          n.delete('finding')
          return n
        },
        { replace: true },
      ),
    [setSp],
  )

  const runAll = async () => {
    setRunning(true)
    try {
      const r = await api.post<HealthSummary>('/api/v1/health/run')
      qc.setQueryData(qk.healthSummary, r)
      await invalidate(qk.health)
      const n = attentionCount(r)
      toast.success('All checks ran', n === 0 ? 'Nothing needs attention.' : `${n} ${n === 1 ? 'issue needs' : 'issues need'} attention.`)
    } catch (e) {
      toast.error('Run failed', errorMessage(e))
    } finally {
      setRunning(false)
    }
  }

  const s = summary.data
  return (
    <>
      <PageHeader
        title="Health"
        description="Spots misconfigurations and fleet problems early, explains them, and fixes what it safely can."
        actions={
          isAdmin && (
            <Button icon={<Play />} loading={running} onClick={runAll}>
              Run all checks
            </Button>
          )
        }
      />
      {summary.isPending && <LoadingBlock />}
      {summary.error && <Alert tone="critical">{errorMessage(summary.error)}</Alert>}
      {s && (
        <>
          <Card className="mb-4 flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
            <StatusLine summary={s} />
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-muted">
              <span>
                Last checked{' '}
                {running ? (
                  <span className="inline-flex items-center gap-1">
                    <Spinner className="size-3" /> running…
                  </span>
                ) : (
                  <RelativeTime value={s.last_run} emptyLabel="not yet" />
                )}
              </span>
              <span>
                {s.checks_enabled} of {s.checks_total} checks enabled
              </span>
              {s.counts.acknowledged > 0 && <span>{s.counts.acknowledged} acknowledged</span>}
              {s.counts.snoozed > 0 && <span>{s.counts.snoozed} snoozed</span>}
            </div>
          </Card>
          {s.check_errors.length > 0 && (
            <div className="mb-4 rounded-xl border border-line bg-surface-sunken/40 px-4 py-2.5 text-xs text-ink-muted">
              <p className="font-medium text-ink">
                {s.check_errors.length === 1 ? '1 check' : `${s.check_errors.length} checks`} could not run, so {s.check_errors.length === 1 ? 'its' : 'their'} results may be stale:
              </p>
              <ul className="mt-1 space-y-0.5">
                {s.check_errors.map((e) => (
                  <li key={e.check_id}>
                    <span className="text-ink">{e.title}</span> <span className="text-critical">— {e.error}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <Tabs<TabKey>
            items={[
              { key: 'issues', label: 'Issues', count: attentionCount(s) || undefined },
              { key: 'checks', label: 'Checks' },
              { key: 'activity', label: 'Activity' },
              { key: 'resolved', label: 'Resolved' },
            ]}
            value={active}
            onChange={(k) => navigate({ pathname: k === 'issues' ? '/health' : `/health/${k}`, search: findingId ? `?finding=${findingId}` : '' }, { replace: true })}
            className="mb-5"
          />
          {active === 'issues' && <IssuesTab summary={s} openFinding={openFinding} initialCheck={sp.get('check')} />}
          {active === 'checks' && <ChecksTab />}
          {active === 'activity' && <ActivityTab openFinding={openFinding} />}
          {active === 'resolved' && <ResolvedTab openFinding={openFinding} />}
        </>
      )}
      <FindingDrawer id={findingId} onClose={closeFinding} />
    </>
  )
}

// ---- Issues ----

interface ViewGroup extends HealthGroup {
  /** Every finding in the group is snoozed (only shown with "Show snoozed"). */
  snoozedOnly?: boolean
  objectType: string
}

function worst(fs: HealthFinding[]): HealthSeverity {
  return fs.reduce<HealthSeverity>((w, f) => (severityMeta[f.severity].rank < severityMeta[w].rank ? f.severity : w), 'info')
}

function IssuesTab({ summary, openFinding, initialCheck }: { summary: HealthSummary; openFinding: (id: string) => void; initialCheck: string | null }) {
  const [sevs, setSevs] = useState<Set<HealthSeverity>>(() => new Set<HealthSeverity>(['critical', 'warning']))
  const [objType, setObjType] = useState('')
  const [showSnoozed, setShowSnoozed] = useState(false)
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set(initialCheck ? [initialCheck] : []))
  const snoozed = useHealthFindings({ state: 'snoozed', limit: 1000 }, showSnoozed)

  const groups: ViewGroup[] = useMemo(() => {
    const out: ViewGroup[] = summary.groups.map((g) => ({ ...g, objectType: g.samples[0]?.object_type ?? '' }))
    if (showSnoozed) {
      const known = new Set(out.map((g) => g.check_id))
      const extra = new Map<string, HealthFinding[]>()
      for (const f of snoozed.data?.items ?? []) if (!known.has(f.check_id)) extra.set(f.check_id, [...(extra.get(f.check_id) ?? []), f])
      for (const [checkId, fs] of extra) {
        out.push({
          check_id: checkId,
          title: fs[0].check_title || checkId,
          category: fs[0].category ?? '',
          severity: worst(fs),
          count: fs.length,
          open: 0,
          samples: fs.slice(0, 3),
          first_seen: fs.reduce((a, f) => (f.first_seen < a ? f.first_seen : a), fs[0].first_seen),
          snoozedOnly: true,
          objectType: fs[0].object_type,
        })
      }
    }
    return out
  }, [summary.groups, showSnoozed, snoozed.data])

  const objTypes = useMemo(() => [...new Set(groups.map((g) => g.objectType).filter(Boolean))].sort(), [groups])

  const groupSeverities = (g: ViewGroup): HealthSeverity[] => (g.severity === 'acknowledged' ? [...new Set(g.samples.map((f) => f.severity))] : [g.severity])
  const visible = groups.filter((g) => (!objType || g.objectType === objType) && groupSeverities(g).some((x) => sevs.has(x)))
  const hiddenInfo = !sevs.has('info') ? summary.counts.info : 0

  const toggleSev = (x: HealthSeverity) =>
    setSevs((cur) => {
      const n = new Set(cur)
      if (n.has(x)) n.delete(x)
      else n.add(x)
      return n
    })
  const toggleGroup = (id: string) =>
    setExpanded((cur) => {
      const n = new Set(cur)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })

  if (groups.length === 0 && !(showSnoozed && snoozed.isPending)) {
    return (
      <div className="flex flex-col gap-4">
        <NoIssues summary={summary} />
        {summary.counts.snoozed > 0 && (
          <div className="flex justify-center">
            <Toggle size="sm" checked={showSnoozed} onChange={setShowSnoozed} label={<span className="text-[13px] text-ink-muted">Show {summary.counts.snoozed} snoozed</span>} />
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex items-center gap-1.5" role="group" aria-label="Filter by severity">
          {severities.map((x) => {
            const m = severityMeta[x]
            const on = sevs.has(x)
            const n = summary.counts[x]
            return (
              <button
                key={x}
                type="button"
                aria-pressed={on}
                onClick={() => toggleSev(x)}
                className={cn(
                  'inline-flex h-8 items-center gap-1.5 rounded-md border px-2.5 text-[13px] font-medium transition-colors focus-ring',
                  on ? 'border-line-strong bg-surface-raised text-ink' : 'border-line bg-transparent text-ink-faint hover:text-ink-muted',
                )}
              >
                <m.icon className={cn('size-3.5', on ? m.ink : 'text-ink-faint')} aria-hidden="true" />
                {m.label}
                <span className="tabular-nums text-xs text-ink-muted">{n}</span>
              </button>
            )
          })}
        </div>
        {objTypes.length > 1 && (
          <Select value={objType} onChange={(e) => setObjType(e.target.value)} className="w-44" aria-label="Filter by object type">
            <option value="">All objects</option>
            {objTypes.map((t) => (
              <option key={t} value={t}>
                {objectTypeLabel(t)}
              </option>
            ))}
          </Select>
        )}
        <Toggle size="sm" className="ml-auto" checked={showSnoozed} onChange={setShowSnoozed} label={<span className="text-[13px] text-ink-muted">Show snoozed{summary.counts.snoozed ? ` (${summary.counts.snoozed})` : ''}</span>} />
      </div>

      {visible.length === 0 ? (
        <Card className="px-4 py-8 text-center">
          <p className="text-sm font-medium text-ink">Nothing at the selected severities</p>
          {hiddenInfo > 0 && (
            <p className="mt-1 text-xs text-ink-muted">
              {hiddenInfo} info-level {hiddenInfo === 1 ? 'item is' : 'items are'} hidden.{' '}
              <button type="button" className="text-accent-700 hover:underline dark:text-accent-300" onClick={() => toggleSev('info')}>
                Show info
              </button>
            </p>
          )}
        </Card>
      ) : (
        <div className="flex flex-col gap-2">
          {visible.map((g) => (
            <GroupRow
              key={g.check_id}
              group={g}
              expanded={expanded.has(g.check_id)}
              onToggle={() => toggleGroup(g.check_id)}
              states={g.snoozedOnly ? 'snoozed' : showSnoozed ? 'open,acknowledged,snoozed' : 'open,acknowledged'}
              sevs={sevs}
              objType={objType}
              openFinding={openFinding}
            />
          ))}
          {hiddenInfo > 0 && (
            <p className="px-1 text-xs text-ink-faint">
              {hiddenInfo} info-level {hiddenInfo === 1 ? 'item' : 'items'} hidden.{' '}
              <button type="button" className="text-ink-muted hover:text-ink hover:underline" onClick={() => toggleSev('info')}>
                Show
              </button>
            </p>
          )}
        </div>
      )}
    </div>
  )
}

function GroupRow({
  group: g,
  expanded,
  onToggle,
  states,
  sevs,
  objType,
  openFinding,
}: {
  group: ViewGroup
  expanded: boolean
  onToggle: () => void
  states: string
  sevs: Set<HealthSeverity>
  objType: string
  openFinding: (id: string) => void
}) {
  const quiet = g.severity === 'info' || g.severity === 'acknowledged' || g.snoozedOnly
  const acked = g.count - g.open
  return (
    <Card className={cn('overflow-hidden', quiet && 'bg-surface/60')}>
      <button type="button" onClick={onToggle} aria-expanded={expanded} className="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-surface-sunken/50 focus-ring">
        <ChevronRight className={cn('mt-0.5 size-4 shrink-0 text-ink-faint transition-transform', expanded && 'rotate-90')} aria-hidden="true" />
        <span className={cn('min-w-0 flex-1', quiet && 'opacity-75')}>
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            {g.snoozedOnly ? <Badge>Snoozed</Badge> : <SeverityBadge severity={g.severity} />}
            <span className="text-[13px] font-semibold text-ink">{g.title}</span>
            {g.category && <Badge className="text-ink-faint">{titleCase(g.category)}</Badge>}
          </span>
          {g.samples[0] && <span className="mt-1 block truncate text-xs text-ink-muted">{g.samples[0].summary}</span>}
        </span>
        <span className="hidden shrink-0 flex-col items-end gap-0.5 text-xs text-ink-muted sm:flex">
          <span className="font-medium text-ink tabular-nums">{objectNoun(g.objectType, g.count)}</span>
          <span>
            <SinceTime value={g.first_seen} />
            {!g.snoozedOnly && acked > 0 && g.open > 0 && <> · {acked} acknowledged</>}
          </span>
        </span>
      </button>
      {expanded && <GroupPanel checkId={g.check_id} states={states} sevs={sevs} objType={objType} openFinding={openFinding} />}
    </Card>
  )
}

function GroupPanel({ checkId, states, sevs, objType, openFinding }: { checkId: string; states: string; sevs: Set<HealthSeverity>; objType: string; openFinding: (id: string) => void }) {
  const { isAdmin } = useAuth()
  const q = useHealthFindings({ check: checkId, state: states, limit: 1000 })
  const checks = useHealthChecks()
  const check = checks.data?.find((c) => c.id === checkId)
  const { bulk } = useHealthActions()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [busy, setBusy] = useState<string | null>(null)
  const [confirmFix, setConfirmFix] = useState<{ id: string; label: string } | null>(null)

  const all = q.data?.items ?? []
  const rows = all.filter((f) => sevs.has(f.severity) && (!objType || f.object_type === objType))
  const hidden = all.length - rows.length
  const sel = rows.filter((f) => selected.has(f.id))
  const fixes = sharedRunnable(sel)
  const allSelected = rows.length > 0 && sel.length === rows.length

  const toggle = (id: string) =>
    setSelected((cur) => {
      const n = new Set(cur)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })

  const runBulk = async (key: string, fn: () => Promise<boolean>) => {
    setBusy(key)
    const ok = await fn()
    setBusy(null)
    if (ok) setSelected(new Set())
  }
  const ids = sel.map((f) => f.id)

  return (
    <div className="border-t border-line">
      {check?.description && <p className="px-4 pt-3 text-xs text-ink-muted max-w-3xl">{check.description}</p>}
      {isAdmin && sel.length > 0 && (
        <div className="mx-4 mt-3 flex flex-wrap items-center gap-2 rounded-lg border border-line bg-surface-sunken/60 px-3 py-2">
          <span className="text-xs font-medium text-ink tabular-nums">{sel.length} selected</span>
          <Button size="xs" icon={<Eye />} loading={busy === 'ack'} onClick={() => runBulk('ack', () => bulk(ids, 'acknowledge'))}>
            Acknowledge
          </Button>
          <SnoozeMenu size="xs" disabled={!!busy} onSnooze={(h) => runBulk('snooze', () => bulk(ids, 'snooze', { snooze_hours: h }))} />
          {fixes.map((r) => (
            <Button
              key={r.id}
              size="xs"
              variant={r.safe ? 'primary' : 'secondary'}
              icon={<Wrench />}
              loading={busy === `fix:${r.id}`}
              onClick={() => (r.safe ? runBulk(`fix:${r.id}`, () => bulk(ids, 'remediate', { remediation_id: r.id, label: r.label })) : setConfirmFix({ id: r.id, label: r.label }))}
            >
              Fix: {r.label}
            </Button>
          ))}
          <button type="button" className="ml-auto text-xs text-ink-muted hover:text-ink" onClick={() => setSelected(new Set())}>
            Clear
          </button>
        </div>
      )}
      <Table<HealthFinding>
        className="mt-2"
        rows={rows}
        loading={q.isPending}
        error={q.error ? errorMessage(q.error) : null}
        rowKey={(f) => f.id}
        dense
        onRowClick={(f) => openFinding(f.id)}
        selected={(f) => selected.has(f.id)}
        rowClassName={(f) => (f.severity === 'info' || f.state === 'snoozed' ? 'opacity-70' : undefined)}
        emptyTitle={hidden > 0 ? 'All findings here are hidden by the filters' : 'Nothing open for this check'}
        columns={[
          ...(isAdmin
            ? [
                {
                  key: 'sel',
                  width: '36px',
                  header: (
                    <input
                      type="checkbox"
                      aria-label="Select all findings"
                      className="size-4 accent-accent-600 focus-ring"
                      checked={allSelected}
                      onChange={() => setSelected(allSelected ? new Set() : new Set(rows.map((f) => f.id)))}
                    />
                  ),
                  render: (f: HealthFinding) => (
                    <input
                      type="checkbox"
                      aria-label={`Select ${f.object_name || f.object_id}`}
                      className="size-4 accent-accent-600 focus-ring"
                      checked={selected.has(f.id)}
                      onClick={(e) => e.stopPropagation()}
                      onChange={() => toggle(f.id)}
                    />
                  ),
                },
              ]
            : []),
          {
            key: 'object',
            header: 'Object',
            render: (f) => (
              <span className="inline-flex items-center gap-2 whitespace-nowrap">
                <SeverityIcon severity={f.severity} />
                <ObjectName f={f} />
              </span>
            ),
          },
          { key: 'summary', header: 'Summary', render: (f) => <span className="text-ink-muted">{f.summary}</span> },
          { key: 'first', header: 'First seen', render: (f) => <RelativeTime value={f.first_seen} className="text-ink-muted" /> },
          {
            key: 'state',
            header: 'State',
            render: (f) => (
              <span className="inline-flex flex-wrap items-center gap-1.5">
                <StateBadge f={f} />
                {f.heal_attempts > 0 && (
                  <Badge tone={f.last_heal_error ? 'critical' : 'neutral'} icon={<Wand2 />} title={f.last_heal_error || `Last attempt ${parseDate(f.last_heal_at)?.toLocaleString() ?? ''}`}>
                    {f.heal_attempts} heal {f.heal_attempts === 1 ? 'attempt' : 'attempts'}
                  </Badge>
                )}
              </span>
            ),
          },
        ]}
      />
      {hidden > 0 && rows.length > 0 && <p className="px-4 py-2 text-xs text-ink-faint border-t border-line">{hidden} more hidden by the current filters.</p>}
      <ConfirmDialog
        open={!!confirmFix}
        onClose={() => setConfirmFix(null)}
        onConfirm={async () => {
          if (confirmFix) await runBulk(`fix:${confirmFix.id}`, () => bulk(ids, 'remediate', { remediation_id: confirmFix.id, label: confirmFix.label }))
          setConfirmFix(null)
        }}
        title={`${confirmFix?.label}?`}
        description={`This runs on ${sel.length} ${sel.length === 1 ? 'finding' : 'findings'}. It is not marked safe, so it never runs automatically.`}
        confirmLabel="Run fix"
      />
    </div>
  )
}

function NoIssues({ summary }: { summary: HealthSummary }) {
  const checks = useHealthChecks()
  const enabled = (checks.data ?? []).filter((c) => c.enabled)
  return (
    <Card className="px-6 py-10">
      <div className="flex flex-col items-center text-center">
        <div className="mb-3 flex size-11 items-center justify-center rounded-full bg-accent-500/12 text-accent-400 ring-1 ring-accent-500/25">
          <HeartPulse className="size-5" aria-hidden="true" />
        </div>
        <p className="text-sm font-semibold text-ink">No issues detected</p>
        <p className="mt-1 max-w-md text-xs text-ink-muted">
          {summary.checks_enabled} {summary.checks_enabled === 1 ? 'check is' : 'checks are'} watching your server and fleet every 5 minutes. Anything that needs you will show up here.
        </p>
      </div>
      {enabled.length > 0 && (
        <div className="mx-auto mt-6 grid max-w-3xl grid-cols-2 gap-3 sm:grid-cols-4">
          {byCategory(enabled).map(([cat, cs]) => (
            <div key={cat} className="rounded-lg border border-line bg-surface-sunken/40 px-3 py-2.5">
              <p className="micro-label">{titleCase(cat)}</p>
              <p className="mt-1 text-lg font-semibold text-ink tabular-nums">{cs.length}</p>
              <p className="text-[11px] text-ink-faint">{cs.length === 1 ? 'check' : 'checks'} watching</p>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}

// ---- Checks ----

function graceLabel(s: number): string {
  if (!s) return ''
  if (s < 3600) return `${Math.round(s / 60)} min grace`
  if (s < 86400) return `${+(s / 3600).toFixed(1)} h grace`
  return `${+(s / 86400).toFixed(1)} d grace`
}

/** Tri-state: we can only infer an override when the effective value differs from the default. */
function autoHealValue(c: HealthCheckInfo): 'default' | 'on' | 'off' {
  if (c.auto_heal === c.auto_heal_default) return 'default'
  return c.auto_heal ? 'on' : 'off'
}

export function HowHealthWorks({ className }: { className?: string }) {
  return (
    <details className={cn('group rounded-lg border border-line bg-surface-sunken/40', className)}>
      <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-sm font-medium text-ink select-none [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-4 text-ink-faint transition-transform group-open:rotate-90" aria-hidden="true" />
        How health checks work
      </summary>
      <ul className="list-disc space-y-1 border-t border-line py-3 pl-8 pr-3 text-[13px] text-ink-muted">
        <li>Checks run every 5 minutes. Some have a grace period, so brief blips never show.</li>
        <li>An issue resolves on its own once the condition is gone for two runs in a row. <span className="text-ink">Re-check now</span> resolves it right away.</li>
        <li>If an issue comes back within an hour, it reopens quietly instead of alerting again.</li>
        <li>
          Webhooks fire only for new or escalated warning and critical issues (<code className="font-mono text-xs">HealthIssueOpened</code>, <code className="font-mono text-xs">HealthIssueResolved</code>).
        </li>
        <li>Auto-heal runs only fixes marked safe: at most 3 tries per issue, at least an hour apart.</li>
        <li>Acknowledge keeps an issue listed but out of the headline. Snooze hides it until the snooze ends.</li>
      </ul>
    </details>
  )
}

function ChecksTab() {
  const { isAdmin } = useAuth()
  const q = useHealthChecks()
  const qc = useQueryClient()
  const toast = useToast()
  const invalidate = useInvalidate()
  const { runCheck } = useHealthActions()
  const [busy, setBusy] = useState<string | null>(null)

  const save = async (c: HealthCheckInfo, body: { enabled?: boolean; auto_heal?: boolean | null }, msg: string) => {
    setBusy(c.id)
    try {
      const r = await api.put<HealthCheckInfo[]>(`/api/v1/health/checks/${c.id}`, body)
      qc.setQueryData(qk.healthChecks, r)
      await invalidate(qk.health)
      toast.success(msg, c.title)
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  const run = async (c: HealthCheckInfo) => {
    setBusy(`run:${c.id}`)
    await runCheck(c.id, c.title)
    setBusy(null)
  }

  if (q.isPending) return <LoadingBlock />
  if (q.error || !q.data) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'Checks unavailable'}</Alert>

  return (
    <div className="flex flex-col gap-5">
      <HowHealthWorks />
      {byCategory(q.data).map(([cat, cs]) => (
        <Card key={cat}>
          <CardHeader title={titleCase(cat)} description={categoryBlurb[cat]} />
          <Table<HealthCheckInfo>
            rows={cs}
            rowKey={(c) => c.id}
            rowClassName={(c) => (c.enabled ? undefined : 'opacity-60')}
            columns={[
              {
                key: 'enabled',
                header: 'On',
                width: '60px',
                render: (c) => <Toggle size="sm" checked={c.enabled} disabled={!isAdmin || busy === c.id} onChange={(v) => save(c, { enabled: v }, v ? 'Check enabled' : 'Check disabled')} />,
              },
              {
                key: 'check',
                header: 'Check',
                render: (c) => (
                  <span className="block max-w-2xl">
                    <span className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-ink">{c.title}</span>
                      {c.grace_seconds > 0 && <span className="text-[11px] text-ink-faint whitespace-nowrap">{graceLabel(c.grace_seconds)}</span>}
                    </span>
                    <span className="mt-0.5 block text-xs text-ink-muted">{c.description}</span>
                    {c.last_run_error && <span className="mt-1 block text-xs text-critical">Last run failed: {c.last_run_error}</span>}
                  </span>
                ),
              },
              {
                key: 'heal',
                header: 'Auto-heal',
                width: '170px',
                render: (c) =>
                  c.supports_heal ? (
                    <Select
                      value={autoHealValue(c)}
                      disabled={!isAdmin || busy === c.id || !c.enabled}
                      aria-label={`Auto-heal for ${c.title}`}
                      className="w-40"
                      onChange={(e) => {
                        const v = e.target.value
                        save(c, { auto_heal: v === 'default' ? null : v === 'on' }, v === 'default' ? 'Auto-heal reset to default' : v === 'on' ? 'Auto-heal on' : 'Auto-heal off')
                      }}
                    >
                      <option value="default">Default ({c.auto_heal_default ? 'on' : 'off'})</option>
                      <option value="on">On</option>
                      <option value="off">Off</option>
                    </Select>
                  ) : (
                    <span title="No automatic fix" className="inline-block">
                      <Select value="none" disabled aria-label={`Auto-heal for ${c.title}: no automatic fix`} className="w-40">
                        <option value="none">No automatic fix</option>
                      </Select>
                    </span>
                  ),
              },
              {
                key: 'run',
                header: '',
                align: 'right',
                render: (c) =>
                  isAdmin ? (
                    <Button size="xs" variant="ghost" icon={<RefreshCw />} loading={busy === `run:${c.id}`} disabled={!c.enabled} onClick={() => run(c)} aria-label={`Run ${c.title} now`}>
                      Run
                    </Button>
                  ) : null,
              },
            ]}
          />
        </Card>
      ))}
    </div>
  )
}

// ---- Activity ----

function ActivityTab({ openFinding }: { openFinding: (id: string) => void }) {
  const q = useHealthEvents({ limit: 200 })
  const checks = useHealthChecks()
  const titles = useMemo(() => new Map((checks.data ?? []).map((c) => [c.id, c.title])), [checks.data])
  if (q.isPending) return <LoadingBlock />
  if (q.error) return <Alert tone="critical">{errorMessage(q.error)}</Alert>
  const events = q.data ?? []
  return (
    <Card>
      <CardHeader title="Recent activity" description="What the health engine and admins did, newest first." />
      {events.length === 0 ? (
        <p className="px-4 py-10 text-center text-sm text-ink-muted">No health activity yet.</p>
      ) : (
        <ol className="divide-y divide-line">
          {events.map((e) => {
            const m = eventMeta(e.event)
            const hl = m.tone === 'accent' ? 'text-accent-700 dark:text-accent-300' : m.tone === 'critical' ? 'text-critical' : 'text-ink'
            return (
              <li key={e.id}>
                <button type="button" onClick={() => openFinding(e.finding_id)} className="flex w-full items-start gap-3 px-4 py-2.5 text-left hover:bg-surface-sunken/50 focus-ring">
                  <EventIcon event={e.event} />
                  <span className="min-w-0 flex-1 pt-0.5">
                    <span className="flex flex-wrap items-baseline gap-x-2">
                      <span className={cn('text-[13px] font-medium', hl)}>{m.label}</span>
                      <span className="text-[13px] text-ink-muted">{titles.get(e.check_id) ?? e.check_id}</span>
                    </span>
                    {e.message && <span className="mt-0.5 block truncate text-xs text-ink-muted">{e.message}</span>}
                  </span>
                  <span className="shrink-0 text-right text-xs text-ink-muted">
                    <RelativeTime value={e.at} />
                    {e.actor && <span className="block text-ink-faint">{e.actor}</span>}
                  </span>
                </button>
              </li>
            )
          })}
        </ol>
      )}
    </Card>
  )
}

// ---- Resolved ----

function ResolvedTab({ openFinding }: { openFinding: (id: string) => void }) {
  const q = useHealthFindings({ state: 'resolved', limit: 200 })
  return (
    <Card>
      <CardHeader title="Resolved" description="Issues that cleared on their own or were fixed, newest first." />
      <Table<HealthFinding>
        rows={q.data?.items}
        loading={q.isPending}
        error={q.error ? errorMessage(q.error) : null}
        rowKey={(f) => f.id}
        onRowClick={(f) => openFinding(f.id)}
        emptyTitle="Nothing resolved yet"
        columns={[
          {
            key: 'object',
            header: 'Object',
            render: (f) => (
              <span className="inline-flex items-center gap-2 whitespace-nowrap">
                <SeverityIcon severity={f.severity} className="opacity-70" />
                <ObjectName f={f} />
              </span>
            ),
          },
          { key: 'check', header: 'Check', render: (f) => <span className="text-ink-muted whitespace-nowrap">{f.check_title || f.check_id}</span> },
          { key: 'summary', header: 'Summary', render: (f) => <span className="text-ink-muted">{f.summary}</span> },
          {
            key: 'when',
            header: 'First seen → resolved',
            render: (f) => (
              <span className="whitespace-nowrap text-ink-muted">
                <RelativeTime value={f.first_seen} /> → <RelativeTime value={f.resolved_at} />
              </span>
            ),
          },
          {
            key: 'dur',
            header: 'Open for',
            align: 'right',
            render: (f) => {
              const a = parseDate(f.first_seen)
              const b = parseDate(f.resolved_at)
              return <span className="tabular-nums text-ink-muted">{a && b ? formatDuration((b.getTime() - a.getTime()) / 1000) : '—'}</span>
            },
          },
        ]}
      />
    </Card>
  )
}

/** Compact health card for the dashboard: status line and the top three groups. */
export function HealthCard() {
  const q = useHealthSummary()
  const s = q.data
  return (
    <Card>
      <CardHeader
        title={
          <span className="inline-flex items-center gap-2">
            <HeartPulse className="size-4 text-ink-faint" aria-hidden="true" />
            Health
          </span>
        }
        actions={
          <Link to="/health" className="text-xs text-accent-700 hover:underline dark:text-accent-300">
            View all
          </Link>
        }
      />
      <div className="px-4 py-3">
        {q.isPending ? (
          <Spinner className="size-4 text-ink-muted" />
        ) : q.error || !s ? (
          <p className="text-xs text-ink-muted">{q.error ? errorMessage(q.error) : 'Unavailable'}</p>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
              <StatusLine summary={s} size="sm" />
              <span className="text-xs text-ink-muted">
                Last checked <RelativeTime value={s.last_run} emptyLabel="not yet" /> · {s.checks_enabled} checks watching
              </span>
            </div>
            {s.groups
              .filter((g) => g.severity !== 'info' && g.severity !== 'acknowledged')
              .slice(0, 3)
              .map((g) => (
                <Link key={g.check_id} to={`/health?check=${encodeURIComponent(g.check_id)}`} className="flex items-center gap-2.5 rounded-md px-1 py-1 -mx-1 text-[13px] hover:bg-surface-sunken/60 focus-ring">
                  <SeverityIcon severity={g.severity} />
                  <span className="truncate font-medium text-ink">{g.title}</span>
                  <span className="shrink-0 text-xs text-ink-muted">{objectNoun(g.samples[0]?.object_type ?? '', g.count)}</span>
                  <span className="ml-auto hidden shrink-0 text-xs text-ink-faint sm:inline">{sevMeta(g.severity).label}</span>
                </Link>
              ))}
          </div>
        )}
      </div>
    </Card>
  )
}
