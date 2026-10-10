import { useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import {
  AlarmClockOff,
  Bell,
  BellOff,
  CheckCircle2,
  CircleCheck,
  CircleDot,
  ChevronDown,
  Eye,
  ExternalLink,
  Info,
  OctagonAlert,
  RotateCcw,
  TrendingUp,
  TriangleAlert,
  Undo2,
  Wand2,
  Wrench,
  XCircle,
  type LucideIcon,
} from 'lucide-react'
import { qk, useInvalidate } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import type { HealthBulkAction, HealthBulkResult, HealthFinding, HealthRemediateResult, HealthRemediation, HealthSeverity, HealthSummary } from '../../api/types'
import { Badge, badgeToneClass, type BadgeTone } from '../ui/Badge'
import { Button } from '../ui/Button'
import { Dropdown } from '../ui/Dropdown'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { RelativeTime } from '../ui/RelativeTime'
import { useToast } from '../ui/Toast'
import { absoluteTime, parseDate, relativeTime, titleCase } from '../../lib/format'
import { cn } from '../../lib/cn'

// ---- severity ----

export type GroupSeverity = HealthSeverity | 'acknowledged'

interface SeverityMeta {
  label: string
  icon: LucideIcon
  tone: BadgeTone
  /** Icon ink used outside a badge. */
  ink: string
  rank: number
}

/** Colour is reserved for critical and warning; info and acknowledged stay neutral. */
export const severityMeta: Record<GroupSeverity, SeverityMeta> = {
  critical: { label: 'Critical', icon: OctagonAlert, tone: 'critical', ink: 'text-critical', rank: 0 },
  warning: { label: 'Warning', icon: TriangleAlert, tone: 'warning', ink: 'text-warning', rank: 1 },
  info: { label: 'Info', icon: Info, tone: 'neutral', ink: 'text-ink-faint', rank: 2 },
  acknowledged: { label: 'Acknowledged', icon: Eye, tone: 'neutral', ink: 'text-ink-faint', rank: 3 },
}

export const severities: HealthSeverity[] = ['critical', 'warning', 'info']

export function sevMeta(s: string): SeverityMeta {
  return severityMeta[s as GroupSeverity] ?? severityMeta.info
}

/** Icon + text, so severity never depends on colour alone. */
export function SeverityBadge({ severity, className }: { severity: string; className?: string }) {
  const m = sevMeta(severity)
  const Icon = m.icon
  return (
    <Badge tone={m.tone} icon={<Icon />} className={className}>
      {m.label}
    </Badge>
  )
}

/** Compact marker: coloured icon with the label available to screen readers and on hover. */
export function SeverityIcon({ severity, className }: { severity: string; className?: string }) {
  const m = sevMeta(severity)
  const Icon = m.icon
  return (
    <span className={cn('inline-flex shrink-0', className)} title={m.label}>
      <Icon className={cn('size-4', m.ink)} aria-hidden="true" />
      <span className="sr-only">{m.label}</span>
    </span>
  )
}

// ---- state ----

export function StateBadge({ f }: { f: Pick<HealthFinding, 'state' | 'snoozed_until' | 'acknowledged_by' | 'resolved_at'> }) {
  switch (f.state) {
    case 'acknowledged':
      return (
        <Badge icon={<Eye />} title={f.acknowledged_by ? `Acknowledged by ${f.acknowledged_by}` : undefined}>
          Acknowledged
        </Badge>
      )
    case 'snoozed':
      return (
        <Badge icon={<BellOff />} title={f.snoozed_until ? `Snoozed until ${absoluteTime(f.snoozed_until)}` : undefined} className="opacity-80">
          {f.snoozed_until ? `Snoozed · wakes ${relativeTime(f.snoozed_until)}` : 'Snoozed'}
        </Badge>
      )
    case 'resolved':
      return (
        <Badge tone="good" icon={<CheckCircle2 />}>
          Resolved
        </Badge>
      )
    case 'pending':
      return <Badge icon={<AlarmClockOff />}>Pending</Badge>
    default:
      return <Badge icon={<CircleDot />}>Open</Badge>
  }
}

// ---- objects ----

const nouns: Record<string, [string, string]> = {
  server: ['server item', 'server items'],
  computer: ['computer', 'computers'],
  policy: ['policy', 'policies'],
  profile: ['profile', 'profiles'],
  app: ['app', 'apps'],
  webhook: ['webhook', 'webhooks'],
  plan: ['update plan', 'update plans'],
}

export function objectNoun(type: string, n: number): string {
  const [one, many] = nouns[type] ?? ['item', 'items']
  return `${n.toLocaleString()} ${n === 1 ? one : many}`
}

/** Plural label for filters, e.g. "Computers". */
export function objectTypeLabel(type: string): string {
  return titleCase((nouns[type] ?? [type, type])[1])
}

/** Singular label, e.g. "Update plan". */
export function objectTypeName(type: string): string {
  const one = (nouns[type] ?? [type, type])[0]
  return one.charAt(0).toUpperCase() + one.slice(1)
}

export function objectLink(type: string, id: string): string | null {
  switch (type) {
    case 'computer':
      return `/computers/${id}`
    case 'policy':
      return `/policies/${id}`
    case 'profile':
      return `/profiles/${id}`
    case 'app':
      return `/apps/${id}`
    case 'plan':
      return `/updates/plans/${id}`
    case 'webhook':
      return '/settings/webhooks'
    default:
      return null
  }
}

export function ObjectName({ f, className }: { f: Pick<HealthFinding, 'object_type' | 'object_id' | 'object_name'>; className?: string }) {
  const to = objectLink(f.object_type, f.object_id)
  const name = f.object_name || f.object_id
  if (!to) return <span className={cn('font-medium text-ink', className)}>{name}</span>
  return (
    <Link to={to} onClick={(e) => e.stopPropagation()} className={cn('font-medium text-ink hover:underline', className)}>
      {name}
    </Link>
  )
}

// ---- facts ----

const isoRe = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/

function factLabel(k: string): string {
  return titleCase(k)
    .replace(/\bMb\b/g, 'MB')
    .replace(/\bGb\b/g, 'GB')
    .replace(/\bMdm\b/g, 'MDM')
    .replace(/\bApns\b/g, 'APNs')
    .replace(/\bUrl\b/g, 'URL')
    .replace(/\bId\b/g, 'ID')
    .replace(/\bOs\b/g, 'OS')
    .replace(/\bDdm\b/g, 'DDM')
}

function FactValue({ v }: { v: unknown }) {
  if (v === null || v === undefined || v === '') return <span className="text-ink-faint">—</span>
  if (typeof v === 'boolean') return <>{v ? 'Yes' : 'No'}</>
  if (typeof v === 'number') return <span className="tabular-nums">{v.toLocaleString()}</span>
  if (typeof v === 'string') {
    if (isoRe.test(v)) {
      if (!parseDate(v)) return <span className="text-ink-faint">never</span>
      return (
        <span>
          {absoluteTime(v)} <span className="text-ink-muted">({relativeTime(v)})</span>
        </span>
      )
    }
    return <>{v}</>
  }
  if (Array.isArray(v)) {
    if (v.length === 0) return <span className="text-ink-faint">none</span>
    if (v.every((x) => typeof x !== 'object' || x === null)) return <>{v.map((x) => String(x)).join(', ')}</>
  }
  return <pre className="whitespace-pre-wrap font-mono text-xs">{JSON.stringify(v, null, 2)}</pre>
}

/** Tidy key/value rendering of a finding's facts. */
export function FactsList({ facts, className }: { facts: Record<string, unknown> | null | undefined; className?: string }) {
  const entries = Object.entries(facts ?? {})
  if (entries.length === 0) return null
  return (
    <dl className={cn('grid grid-cols-[minmax(0,10rem)_1fr] gap-x-4 gap-y-1.5 text-[13px]', className)}>
      {entries.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-ink-muted truncate" title={k}>
            {factLabel(k)}
          </dt>
          <dd className="min-w-0 break-words text-ink">
            <FactValue v={v} />
          </dd>
        </div>
      ))}
    </dl>
  )
}

// ---- headline ----

export function attentionCount(s: HealthSummary | undefined): number {
  return s ? s.counts.critical + s.counts.warning : 0
}

/** "All clear" or "2 critical · 5 warnings", icon + text. */
export function StatusLine({ summary, size = 'md' }: { summary: HealthSummary; size?: 'sm' | 'md' }) {
  const { critical, warning } = summary.counts
  const big = size === 'md'
  if (critical + warning === 0) {
    return (
      <span className={cn('inline-flex items-center gap-2 font-semibold text-ink', big ? 'text-base' : 'text-[13px]')}>
        <CircleCheck className={cn('text-accent-400', big ? 'size-5' : 'size-4')} aria-hidden="true" />
        All clear
      </span>
    )
  }
  const parts: ReactNode[] = []
  if (critical) parts.push(<span key="c">{critical} critical</span>)
  if (warning) parts.push(<span key="w">{warning === 1 ? '1 warning' : `${warning} warnings`}</span>)
  const Icon = critical ? OctagonAlert : TriangleAlert
  return (
    <span className={cn('inline-flex items-center gap-2 font-semibold text-ink', big ? 'text-base' : 'text-[13px]')}>
      <Icon className={cn(critical ? 'text-critical' : 'text-warning', big ? 'size-5' : 'size-4')} aria-hidden="true" />
      {parts.map((p, i) => (
        <span key={i} className="inline-flex items-center gap-2">
          {i > 0 && <span className="text-ink-faint" aria-hidden="true">·</span>}
          {p}
        </span>
      ))}
    </span>
  )
}

// ---- events ----

interface EventMeta {
  label: string
  icon: LucideIcon
  tone: BadgeTone
}

const eventMetas: Record<string, EventMeta> = {
  opened: { label: 'Opened', icon: CircleDot, tone: 'neutral' },
  reopened: { label: 'Reopened', icon: RotateCcw, tone: 'neutral' },
  resolved: { label: 'Resolved', icon: CheckCircle2, tone: 'neutral' },
  escalated: { label: 'Escalated', icon: TrendingUp, tone: 'neutral' },
  auto_heal: { label: 'Auto-healed', icon: Wand2, tone: 'accent' },
  heal_failed: { label: 'Auto-heal failed', icon: XCircle, tone: 'critical' },
  acknowledged: { label: 'Acknowledged', icon: Eye, tone: 'neutral' },
  snoozed: { label: 'Snoozed', icon: BellOff, tone: 'neutral' },
  unsnoozed: { label: 'Snooze ended', icon: Bell, tone: 'neutral' },
  open: { label: 'Reopened by admin', icon: Undo2, tone: 'neutral' },
  remediated: { label: 'Fix applied', icon: Wrench, tone: 'accent' },
  remediation_failed: { label: 'Fix failed', icon: XCircle, tone: 'critical' },
}

export function eventMeta(e: string): EventMeta {
  return eventMetas[e] ?? { label: titleCase(e), icon: CircleDot, tone: 'neutral' }
}

export function EventIcon({ event, className }: { event: string; className?: string }) {
  const m = eventMeta(event)
  const Icon = m.icon
  return (
    <span className={cn('relative z-[1] flex size-7 shrink-0 items-center justify-center rounded-full border [&>svg]:size-3.5', badgeToneClass[m.tone], className)} aria-hidden="true">
      <Icon />
    </span>
  )
}

// ---- actions ----

export const snoozeOptions: { label: string; hours: number }[] = [
  { label: '1 hour', hours: 1 },
  { label: '1 day', hours: 24 },
  { label: '7 days', hours: 168 },
  { label: '30 days', hours: 720 },
]

export function SnoozeMenu({ onSnooze, size = 'sm', disabled, align = 'left' }: { onSnooze: (hours: number) => void; size?: 'xs' | 'sm'; disabled?: boolean; align?: 'left' | 'right' }) {
  return (
    <Dropdown
      align={align}
      menuClassName="min-w-36"
      trigger={
        <Button size={size} icon={<BellOff />} iconRight={<ChevronDown />} disabled={disabled} aria-haspopup="menu">
          Snooze
        </Button>
      }
      items={snoozeOptions.map((o) => ({ label: `Snooze ${o.label}`, onSelect: () => onSnooze(o.hours) }))}
    />
  )
}

/** Health mutations with consistent toasts; every success invalidates the whole ['health'] prefix. */
export function useHealthActions() {
  const toast = useToast()
  const invalidate = useInvalidate()
  const refresh = () => invalidate(qk.health)

  const setState = async (f: Pick<HealthFinding, 'id'>, state: 'acknowledged' | 'snoozed' | 'open', snoozeHours?: number) => {
    try {
      await api.post(`/api/v1/health/findings/${f.id}/state`, { state, snooze_hours: snoozeHours })
      const opt = snoozeOptions.find((o) => o.hours === snoozeHours)
      toast.success(state === 'acknowledged' ? 'Acknowledged' : state === 'snoozed' ? `Snoozed for ${opt?.label ?? `${snoozeHours} h`}` : 'Reopened')
      await refresh()
      return true
    } catch (e) {
      toast.error('Update failed', errorMessage(e))
      return false
    }
  }

  const remediate = async (f: Pick<HealthFinding, 'id'>, r: HealthRemediation) => {
    try {
      const res = await api.post<HealthRemediateResult>(`/api/v1/health/findings/${f.id}/remediate`, { remediation_id: r.id })
      toast.success(res.resolved ? 'Already resolved' : r.label, res.message)
      await refresh()
      return true
    } catch (e) {
      toast.error(`${r.label} failed`, errorMessage(e))
      await refresh()
      return false
    }
  }

  const bulk = async (ids: string[], action: HealthBulkAction, extra: { snooze_hours?: number; remediation_id?: string; label?: string } = {}) => {
    try {
      const res = await api.post<HealthBulkResult>('/api/v1/health/findings/bulk', { ids, action, snooze_hours: extra.snooze_hours, remediation_id: extra.remediation_id })
      const failed = Object.values(res.failed ?? {})
      const verb = action === 'acknowledge' ? 'Acknowledged' : action === 'snooze' ? 'Snoozed' : action === 'reopen' ? 'Reopened' : extra.label ?? 'Fixed'
      const n = `${res.succeeded} ${res.succeeded === 1 ? 'finding' : 'findings'}`
      if (failed.length === 0) toast.success(`${verb}: ${n}`)
      else toast.warning(`${verb}: ${n}, ${failed.length} failed`, failed[0])
      await refresh()
      return true
    } catch (e) {
      toast.error('Bulk action failed', errorMessage(e))
      return false
    }
  }

  const runCheck = async (checkId: string, title?: string) => {
    try {
      await api.post(`/api/v1/health/checks/${checkId}/run`)
      toast.success('Re-checked', title ? `${title} ran just now.` : undefined)
      await refresh()
      return true
    } catch (e) {
      toast.error('Check failed to run', errorMessage(e))
      return false
    }
  }

  return { setState, remediate, bulk, runCheck, refresh }
}

// ---- remediations ----

function GuidanceLink({ r }: { r: HealthRemediation }) {
  if (!r.url) return null
  const external = /^https?:\/\//.test(r.url)
  const cls = 'inline-flex items-center gap-1.5 h-8 px-2.5 rounded-md text-[13px] font-medium bg-surface-raised text-ink border border-line-strong/70 hover:bg-surface-sunken hover:border-line-strong focus-ring'
  return external ? (
    <a href={r.url} target="_blank" rel="noreferrer" className={cls}>
      {r.label}
      <ExternalLink className="size-3.5 text-ink-muted" aria-hidden="true" />
    </a>
  ) : (
    <Link to={r.url} className={cls}>
      {r.label}
      <ExternalLink className="size-3.5 text-ink-muted" aria-hidden="true" />
    </Link>
  )
}

/**
 * Remediations for one finding. Runnable safe fixes run immediately; unsafe ones confirm first.
 * Guidance renders as an in-app link, or plain text when there is no URL. Viewers see the list without buttons.
 */
export function RemediationList({ finding, canAct, autoHeal }: { finding: HealthFinding; canAct: boolean; autoHeal?: boolean }) {
  const { remediate } = useHealthActions()
  const [busy, setBusy] = useState<string | null>(null)
  const [confirm, setConfirm] = useState<HealthRemediation | null>(null)
  const items = finding.remediations ?? []
  if (items.length === 0) return <p className="text-[13px] text-ink-muted">No fix is offered for this finding. It resolves automatically once the condition clears.</p>
  const resolved = finding.state === 'resolved'

  const run = async (r: HealthRemediation) => {
    setBusy(r.id)
    await remediate(finding, r)
    setBusy(null)
  }

  return (
    <ul className="flex flex-col gap-2.5">
      {items.map((r) => (
        <li key={r.id} className="rounded-lg border border-line bg-surface-sunken/40 px-3 py-2.5">
          {r.runnable ? (
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <p className="text-[13px] font-medium text-ink">{r.label}</p>
                {r.description && <p className="mt-0.5 text-xs text-ink-muted">{r.description}</p>}
                {r.safe && (
                  <p className="mt-1 inline-flex items-center gap-1 text-[11px] text-accent-700 dark:text-accent-300">
                    <Wand2 className="size-3" aria-hidden="true" />
                    {autoHeal ? 'Runs automatically (auto-heal is on)' : 'Auto-heal eligible'}
                  </p>
                )}
              </div>
              {canAct && !resolved && (
                <Button size="sm" variant={r.safe ? 'primary' : 'secondary'} icon={<Wrench />} loading={busy === r.id} onClick={() => (r.safe ? run(r) : setConfirm(r))}>
                  {r.safe ? 'Fix now' : 'Run…'}
                </Button>
              )}
            </div>
          ) : r.url ? (
            <div className="flex flex-wrap items-start justify-between gap-3">
              <p className="min-w-0 flex-1 text-xs text-ink-muted">{r.description || 'Fix this manually in the console.'}</p>
              <GuidanceLink r={r} />
            </div>
          ) : (
            <div>
              <p className="text-[13px] font-semibold text-ink">{r.label}</p>
              {r.description && <p className="mt-0.5 text-xs text-ink-muted">{r.description}</p>}
            </div>
          )}
        </li>
      ))}
      <ConfirmDialog
        open={!!confirm}
        onClose={() => setConfirm(null)}
        onConfirm={async () => {
          if (confirm) await run(confirm)
          setConfirm(null)
        }}
        title={`${confirm?.label}?`}
        description={
          <>
            {confirm?.description && <p>{confirm.description}</p>}
            <p className={cn(confirm?.description && 'mt-2')}>
              This applies to <span className="font-medium text-ink">{finding.object_name || finding.object_id}</span>. It is not marked safe, so it never runs automatically.
            </p>
          </>
        }
        confirmLabel="Run fix"
      />
    </ul>
  )
}

/** Remediations every selected finding offers that the server can run. */
export function sharedRunnable(findings: HealthFinding[]): HealthRemediation[] {
  if (findings.length === 0) return []
  const [first, ...rest] = findings
  return (first.remediations ?? []).filter((r) => r.runnable && rest.every((f) => (f.remediations ?? []).some((x) => x.runnable && x.id === r.id)))
}

export function SinceTime({ value }: { value: string }) {
  return (
    <span className="whitespace-nowrap">
      since <RelativeTime value={value} />
    </span>
  )
}
