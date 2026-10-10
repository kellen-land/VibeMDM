import { Fragment, useState, type ReactNode } from 'react'
import {
  AlarmClockOff,
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  CircleOff,
  Clock,
  FileCode2,
  Hand,
  MinusCircle,
  ShieldCheck,
  SignalHigh,
  SignalLow,
  SignalMedium,
  Wrench,
  XCircle,
  type LucideIcon,
} from 'lucide-react'
import { qk, useComplianceRule, useInvalidate } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import type { ComplianceScanResult } from '../../api/types'
import { Badge, type BadgeTone } from '../ui/Badge'
import { Button } from '../ui/Button'
import { CodeBlock } from '../ui/CopyButton'
import { Spinner } from '../ui/Spinner'
import { useToast } from '../ui/Toast'
import { titleCase } from '../../lib/format'
import { cn } from '../../lib/cn'

// ---- benchmarks ----

const benchmarkNames: Record<string, string> = {
  cis_lvl1: 'CIS Benchmark Level 1',
  cis_lvl2: 'CIS Benchmark Level 2',
  cisv8: 'CIS Controls v8',
  '800-53r5_low': 'NIST 800-53 Rev 5 Low',
  '800-53r5_moderate': 'NIST 800-53 Rev 5 Moderate',
  '800-53r5_high': 'NIST 800-53 Rev 5 High',
  '800-171': 'NIST 800-171',
  cmmc_lvl1: 'CMMC Level 1',
  cmmc_lvl2: 'CMMC Level 2',
  'cnssi-1253_low': 'CNSSI-1253 Low',
  'cnssi-1253_moderate': 'CNSSI-1253 Moderate',
  'cnssi-1253_high': 'CNSSI-1253 High',
  disa_stig: 'DISA STIG',
  hicp_lp: 'HICP Large Practices',
  all_rules: 'All rules',
}

const benchmarkBlurbs: Record<string, string> = {
  cis_lvl1: 'Practical hardening most organizations can apply broadly with little impact on users.',
  cis_lvl2: 'Stricter CIS settings for higher-security environments. Some may limit functionality.',
  cisv8: 'Rules mapped to the CIS Critical Security Controls v8.',
  '800-53r5_low': 'US federal security controls for low-impact systems.',
  '800-53r5_moderate': 'US federal security controls for moderate-impact systems. A common default.',
  '800-53r5_high': 'US federal security controls for high-impact systems.',
  '800-171': 'Protecting Controlled Unclassified Information (CUI) on non-federal systems.',
  cmmc_lvl1: 'US DoD Cybersecurity Maturity Model Certification, Level 1.',
  cmmc_lvl2: 'US DoD Cybersecurity Maturity Model Certification, Level 2.',
  'cnssi-1253_low': 'US national security systems baseline, low impact.',
  'cnssi-1253_moderate': 'US national security systems baseline, moderate impact.',
  'cnssi-1253_high': 'US national security systems baseline, high impact.',
  disa_stig: 'US DoD Security Technical Implementation Guide. Rules carry a severity.',
  hicp_lp: 'Health Industry Cybersecurity Practices for large organizations.',
  all_rules: 'Every rule in the catalog. A starting point to tailor your own baseline from.',
}

/** Display order for the benchmark picker: the common choices first. */
export const benchmarkOrder = [
  'cis_lvl1',
  'cis_lvl2',
  '800-53r5_low',
  '800-53r5_moderate',
  '800-53r5_high',
  '800-171',
  'disa_stig',
  'cmmc_lvl1',
  'cmmc_lvl2',
  'cisv8',
  'cnssi-1253_low',
  'cnssi-1253_moderate',
  'cnssi-1253_high',
  'hicp_lp',
  'all_rules',
]

/** Friendly benchmark name; falls back to the catalog title without its "macOS 27.0: Security Configuration - " prefix. */
export function benchmarkName(key: string, title?: string): string {
  if (benchmarkNames[key]) return benchmarkNames[key]
  if (title) return title.replace(/^.*Security Configuration\s*-\s*/i, '') || title
  return key
}

export function benchmarkBlurb(key: string, description?: string): string {
  return benchmarkBlurbs[key] ?? description ?? ''
}

// ---- sections ----

const sectionNames: Record<string, string> = {
  os: 'macOS',
  system_settings: 'System Settings',
  audit: 'Auditing',
  pwpolicy: 'Password policy',
  icloud: 'iCloud',
  auth: 'Authentication',
  supplemental: 'Supplemental',
}
export const sectionOrder = ['auth', 'pwpolicy', 'os', 'system_settings', 'audit', 'icloud', 'supplemental']

export function sectionLabel(s: string): string {
  return sectionNames[s] ?? (s ? titleCase(s) : 'Other')
}

export function bySection<T extends { section: string }>(items: T[]): [string, T[]][] {
  const m = new Map<string, T[]>()
  for (const it of items) m.set(it.section, [...(m.get(it.section) ?? []), it])
  const rank = (s: string) => {
    const i = sectionOrder.indexOf(s)
    return i < 0 ? 99 : i
  }
  return [...m.entries()].sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
}

// ---- intervals ----

export function intervalLabel(hours: number): string {
  if (!hours) return '—'
  if (hours % 168 === 0) return hours === 168 ? 'Weekly' : `Every ${hours / 168} weeks`
  if (hours % 24 === 0) return hours === 24 ? 'Daily' : `Every ${hours / 24} days`
  return `Every ${hours} h`
}

// ---- score ----

export function scoreText(score: number | null | undefined, hasData = true): string {
  if (!hasData || score === null || score === undefined || Number.isNaN(score)) return '—'
  const r = Math.round(score)
  if (r === 100 && score < 100) return '99%'
  return `${r}%`
}

/** Percent-passing meter. Teal fill: the number carries meaning, so colour adds no alarm. */
export function ScoreMeter({ score, hasData = true, className, size = 'md', showValue = true }: { score: number; hasData?: boolean; className?: string; size?: 'sm' | 'md' | 'lg'; showValue?: boolean }) {
  const pct = hasData ? Math.max(0, Math.min(100, score)) : 0
  return (
    <span className={cn('inline-flex items-center gap-2', className)}>
      <span
        className={cn('relative block overflow-hidden rounded-full bg-line/60', size === 'sm' ? 'h-1.5 w-16' : size === 'lg' ? 'h-2 w-40' : 'h-2 w-24')}
        role="meter"
        aria-valuenow={hasData ? Math.round(pct) : undefined}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label="Compliance score"
      >
        <span className="absolute inset-y-0 left-0 rounded-full bg-accent-500" style={{ width: `${pct}%` }} />
      </span>
      {showValue && <span className={cn('tabular-nums', size === 'sm' ? 'text-xs' : 'text-[13px] font-medium', hasData ? 'text-ink' : 'text-ink-faint')}>{scoreText(score, hasData)}</span>}
    </span>
  )
}

// ---- rule status ----

export interface StatusMeta {
  label: string
  icon: LucideIcon
  tone: BadgeTone
  ink: string
  fill: string
  rank: number
}

export const ruleStatusMeta: Record<string, StatusMeta> = {
  fail: { label: 'Fail', icon: XCircle, tone: 'critical', ink: 'text-critical', fill: 'bg-critical', rank: 0 },
  error: { label: 'Error', icon: AlertTriangle, tone: 'warning', ink: 'text-warning', fill: 'bg-warning', rank: 1 },
  remediated: { label: 'Remediated', icon: Wrench, tone: 'accent', ink: 'text-accent-500', fill: 'bg-accent-500', rank: 2 },
  not_scanned: { label: 'Not scanned', icon: CircleDashed, tone: 'neutral', ink: 'text-ink-faint', fill: 'bg-line-strong', rank: 3 },
  pass: { label: 'Pass', icon: CheckCircle2, tone: 'good', ink: 'text-good', fill: 'bg-good', rank: 4 },
  manual: { label: 'Manual', icon: Hand, tone: 'neutral', ink: 'text-ink-faint', fill: 'bg-ink-faint', rank: 5 },
  excluded: { label: 'Exempt', icon: MinusCircle, tone: 'neutral', ink: 'text-ink-faint', fill: 'bg-ink-faint', rank: 6 },
  not_applicable: { label: 'N/A', icon: CircleOff, tone: 'neutral', ink: 'text-ink-faint', fill: 'hatch-neutral', rank: 7 },
}

export const ruleStatusOrder = ['fail', 'error', 'remediated', 'not_scanned', 'pass', 'manual', 'excluded', 'not_applicable']

export function statusMeta(s: string): StatusMeta {
  return ruleStatusMeta[s] ?? { label: titleCase(s || 'unknown'), icon: CircleDashed, tone: 'neutral', ink: 'text-ink-faint', fill: 'bg-line-strong', rank: 9 }
}

export function RuleStatusBadge({ status, className }: { status: string; className?: string }) {
  const m = statusMeta(status)
  const Icon = m.icon
  return (
    <Badge tone={m.tone} icon={<Icon />} className={className}>
      {m.label}
    </Badge>
  )
}

// ---- severity ----

const severityMetas: Record<string, { label: string; icon: LucideIcon; tone: BadgeTone }> = {
  high: { label: 'High', icon: SignalHigh, tone: 'critical' },
  medium: { label: 'Medium', icon: SignalMedium, tone: 'warning' },
  low: { label: 'Low', icon: SignalLow, tone: 'neutral' },
}

export const severityRank = (s: string) => (s === 'high' ? 0 : s === 'medium' ? 1 : s === 'low' ? 2 : 3)

/** mSCP severity (mostly DISA STIG). Renders nothing when the benchmark sets none. */
export function RuleSeverityBadge({ severity }: { severity: string }) {
  const m = severityMetas[severity]
  if (!m) return null
  const Icon = m.icon
  return (
    <Badge tone={m.tone} icon={<Icon />} title={`${m.label} severity`}>
      {m.label}
    </Badge>
  )
}

/** Fix / Profile / Manual capability chips. Neutral: they describe the rule, they are not status. */
export function RuleCapabilityBadges({ hasFix, hasProfile, manualReason }: { hasFix?: boolean; hasProfile?: boolean; manualReason?: string }) {
  return (
    <>
      {hasFix && (
        <Badge icon={<Wrench />} title="mSCP ships a fix script that auto-remediate can run">
          Fix
        </Badge>
      )}
      {hasProfile && (
        <Badge icon={<ShieldCheck />} title="Enforceable with a configuration profile">
          Profile
        </Badge>
      )}
      {manualReason && (
        <Badge icon={<Hand />} title={manualReason}>
          Manual
        </Badge>
      )}
    </>
  )
}

// ---- scans ----

const scanStatusMetas: Record<string, { label: string; icon: LucideIcon; tone: BadgeTone }> = {
  pending: { label: 'Pending', icon: Clock, tone: 'info' },
  completed: { label: 'Completed', icon: CheckCircle2, tone: 'good' },
  failed: { label: 'Failed', icon: XCircle, tone: 'critical' },
  expired: { label: 'Expired', icon: AlarmClockOff, tone: 'neutral' },
}

export function ScanStatusBadge({ status }: { status: string }) {
  if (!status) return <span className="text-ink-faint">—</span>
  const m = scanStatusMetas[status] ?? { label: titleCase(status), icon: CircleDashed, tone: 'neutral' as BadgeTone }
  const Icon = m.icon
  return (
    <Badge tone={m.tone} icon={<Icon />}>
      {m.label}
    </Badge>
  )
}

export function triggerLabel(t: string): string {
  switch (t) {
    case 'scheduled':
      return 'Scheduled'
    case 'manual':
      return 'Manual'
    case 'manual_remediate':
      return 'Manual, remediate'
    case 'health':
      return 'Health self-heal'
    default:
      return titleCase(t || 'unknown')
  }
}

/** Queue scans with consistent toasts; refreshes every compliance query (and the computer tab) afterwards. */
export function useComplianceScan() {
  const toast = useToast()
  const invalidate = useInvalidate()
  return async (baselineId: string, opts: { computerIds?: string[]; remediate?: boolean; computerName?: string } = {}) => {
    try {
      const r = await api.post<ComplianceScanResult>(`/api/v1/compliance/baselines/${baselineId}/scan`, {
        computer_ids: opts.computerIds?.length ? opts.computerIds : undefined,
        remediate: opts.remediate,
      })
      const skipped = Object.entries(r.skipped ?? {})
      const verb = opts.remediate ? 'Scan and remediate' : 'Scan'
      const target = opts.computerName ?? `${r.queued} ${r.queued === 1 ? 'Mac' : 'Macs'}`
      if (r.queued === 0 && skipped.length) toast.warning(`${verb} not queued`, skipped.map(([n, why]) => `${n}: ${why}`).slice(0, 3).join(' · '))
      else if (r.queued === 0) toast.info('Nothing to scan', 'No Macs are in this baseline’s scope.')
      else if (skipped.length) toast.warning(`${verb} queued for ${target}`, `${skipped.length} skipped. ${skipped[0][0]}: ${skipped[0][1]}`)
      else toast.success(`${verb} queued for ${target}`, 'Results arrive as each agent checks in.')
      await invalidate(qk.compliance, ...(opts.computerIds ?? []).map((id) => qk.computerCompliance(id)))
      return r
    } catch (e) {
      toast.error('Scan failed', errorMessage(e))
      return null
    }
  }
}

// ---- result bar ----

/** Tiny stacked bar of one rule's results across Macs. Label is in aria-label and title; counts sit beside it in tables. */
export function ResultMiniBar({ passed, remediated, errors, failed, className }: { passed: number; remediated: number; errors: number; failed: number; className?: string }) {
  const segs = [
    { k: 'pass', n: passed },
    { k: 'remediated', n: remediated },
    { k: 'error', n: errors },
    { k: 'fail', n: failed },
  ].filter((x) => x.n > 0)
  const total = segs.reduce((a, x) => a + x.n, 0)
  const label = total ? segs.map((x) => `${statusMeta(x.k).label} ${x.n}`).join(', ') : 'No results yet'
  return (
    <span className={cn('flex h-1.5 w-24 gap-px overflow-hidden rounded-full bg-line/60', className)} role="img" aria-label={label} title={label}>
      {segs.map((x) => (
        <span key={x.k} className={cn('h-full', statusMeta(x.k).fill)} style={{ flexGrow: x.n, flexBasis: 0 }} />
      ))}
    </span>
  )
}

// ---- discussion ----

const inlineRe = /((?<![A-Za-z0-9])_[^_\n]+?_(?![A-Za-z0-9])|`[^`\n]+`)/g

function inline(text: string): ReactNode[] {
  return text.split(inlineRe).map((part, i) => {
    if (i % 2 === 1) {
      if (part.startsWith('`')) return <code key={i} className="rounded bg-surface-sunken px-1 font-mono text-[12px]">{part.slice(1, -1)}</code>
      return <em key={i} className="font-semibold not-italic text-ink">{part.slice(1, -1)}</em>
    }
    return <Fragment key={i}>{part}</Fragment>
  })
}

/** mSCP discussion text: blank-line paragraphs, "* " bullets, _emphasis_ and `code`. */
export function Discussion({ text, className }: { text: string; className?: string }) {
  const blocks = (text || '').trim().split(/\n\s*\n/)
  if (!text?.trim()) return <p className={cn('text-[13px] text-ink-faint', className)}>No discussion provided.</p>
  return (
    <div className={cn('flex flex-col gap-2 text-[13px] leading-relaxed text-ink-muted', className)}>
      {blocks.map((b, i) => {
        const lines = b.split('\n')
        if (lines.every((l) => /^\s*[*-]\s+/.test(l)))
          return (
            <ul key={i} className="list-disc space-y-0.5 pl-5">
              {lines.map((l, j) => (
                <li key={j}>{inline(l.replace(/^\s*[*-]\s+/, ''))}</li>
              ))}
            </ul>
          )
        return (
          <p key={i} className="whitespace-pre-line">
            {inline(b)}
          </p>
        )
      })}
    </div>
  )
}

// ---- references ----

const refLabels: Record<string, string> = {
  nist: 'NIST',
  cis: 'CIS',
  disa: 'DISA',
  hhs: 'HHS',
  bzk: 'BZK',
  bsi: 'BSI',
  '800-53r5': '800-53 r5',
  '800-171r3': '800-171 r3',
  cce: 'CCE',
  cci: 'CCI',
  srg: 'SRG',
  disa_stig: 'STIG',
  benchmark: 'Benchmark',
  controls_v8: 'Controls v8',
  cmmc: 'CMMC',
  hicp: 'HICP',
  bio: 'BIO',
  indigo: 'Indigo',
  sfr: 'SFR',
}

const refLabel = (k: string) => refLabels[k] ?? titleCase(k)
const isObj = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)

interface RefRow {
  label: string
  values: string[]
}

/** Flatten any references shape into labelled rows. Per-OS maps (macos_14…) show the newest OS only. */
function flattenRefs(v: unknown, path: string[], out: RefRow[]) {
  if (v === null || v === undefined || v === '') return
  if (Array.isArray(v)) {
    const values = v.filter((x) => x !== null && x !== undefined && typeof x !== 'object').map(String)
    if (values.length) out.push({ label: path.map(refLabel).join(' · '), values })
    v.filter(isObj).forEach((x) => flattenRefs(x, path, out))
    return
  }
  if (isObj(v)) {
    const keys = Object.keys(v)
    const osKeys = keys.filter((k) => /^macos_\d+/.test(k))
    if (osKeys.length && osKeys.length === keys.length) {
      const latest = osKeys.sort((a, b) => Number(b.slice(6)) - Number(a.slice(6)))[0]
      const before = out.length
      flattenRefs(v[latest], path, out)
      for (const row of out.slice(before)) row.label += ` (macOS ${latest.slice(6)})`
      return
    }
    for (const k of keys) flattenRefs(v[k], [...path, k], out)
    return
  }
  out.push({ label: path.map(refLabel).join(' · '), values: [String(v)] })
}

export function References({ refs, className }: { refs: Record<string, unknown> | null | undefined; className?: string }) {
  const rows: RefRow[] = []
  flattenRefs(refs, [], rows)
  if (rows.length === 0) return <p className="text-[13px] text-ink-faint">No references.</p>
  return (
    <dl className={cn('grid grid-cols-[minmax(0,9.5rem)_1fr] gap-x-4 gap-y-1.5 text-[13px]', className)}>
      {rows.map((r, i) => (
        <div key={i} className="contents">
          <dt className="truncate pt-0.5 text-xs text-ink-muted" title={r.label}>
            {r.label}
          </dt>
          <dd className="flex min-w-0 flex-wrap gap-1">
            {r.values.slice(0, 24).map((v) => (
              <span key={v} className="rounded border border-line bg-surface-sunken/60 px-1.5 py-px font-mono text-[11px] text-ink">
                {v}
              </span>
            ))}
            {r.values.length > 24 && <span className="text-xs text-ink-faint">+{r.values.length - 24} more</span>}
          </dd>
        </div>
      ))}
    </dl>
  )
}

// ---- scripts ----

/** Check / expected result / fix scripts, fetched only when the admin asks to see them. */
export function RuleScripts({ ruleId, benchmark, className }: { ruleId: string; benchmark: string; className?: string }) {
  const [open, setOpen] = useState(false)
  const q = useComplianceRule(ruleId, benchmark, open)
  if (!open)
    return (
      <Button size="xs" variant="secondary" icon={<FileCode2 />} onClick={() => setOpen(true)} className={className}>
        View check script
      </Button>
    )
  return (
    <div className={cn('flex flex-col gap-2', className)}>
      {q.isPending && (
        <span className="inline-flex items-center gap-2 text-xs text-ink-muted">
          <Spinner className="size-3.5" /> Loading scripts…
        </span>
      )}
      {q.error && <p className="text-xs text-critical">{errorMessage(q.error)}</p>}
      {q.data && (
        <>
          {q.data.check ? <CodeBlock text={q.data.check} filename="check (runs as root)" maxHeight="14rem" /> : <p className="text-xs text-ink-faint">This rule has no automated check.</p>}
          {q.data.expected && (
            <p className="text-xs text-ink-muted">
              Expected result: <span className="font-mono text-ink">{q.data.expected}</span>
            </p>
          )}
          {q.data.fix && <CodeBlock text={q.data.fix} filename="fix (auto-remediate)" maxHeight="14rem" />}
        </>
      )}
      <button type="button" className="w-fit text-xs text-ink-muted hover:text-ink" onClick={() => setOpen(false)}>
        Hide scripts
      </button>
    </div>
  )
}

// ---- attribution ----

export function ComplianceAttribution({ release, className }: { release?: string; className?: string }) {
  return (
    <p className={cn('mt-8 text-[11px] text-ink-faint', className)}>
      Rules and baselines from the{' '}
      <a href="https://github.com/usnistgov/macos_security" target="_blank" rel="noreferrer" className="hover:text-ink hover:underline">
        macOS Security Compliance Project (mSCP)
      </a>
      {release ? ` ${release}` : ''}, licensed{' '}
      <a href="https://creativecommons.org/licenses/by/4.0/" target="_blank" rel="noreferrer" className="hover:text-ink hover:underline">
        CC BY 4.0
      </a>
      . Changes were made.
    </p>
  )
}
