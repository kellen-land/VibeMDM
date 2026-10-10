import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ChevronDown, ChevronRight, Laptop, Play, TrendingDown, Wrench } from 'lucide-react'
import { useComplianceCatalog, useComplianceComputer } from '../../api/queries'
import { errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { ComplianceComputerRule } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import {
  ComplianceAttribution,
  RuleCapabilityBadges,
  RuleSeverityBadge,
  RuleStatusBadge,
  ScoreMeter,
  benchmarkName,
  ruleStatusOrder,
  sectionLabel,
  statusMeta,
  useComplianceScan,
} from '../../components/compliance/compliance'
import { ScansTable } from './BaselineReportPage'
import { cn } from '../../lib/cn'

/** Groups that start collapsed: nothing to act on. */
const quietStatuses = new Set(['pass', 'manual', 'excluded', 'not_applicable'])

const drifted = (r: ComplianceComputerRule) => r.status === 'fail' && (r.previous_status === 'pass' || r.previous_status === 'remediated')

export function BaselineComputerPage() {
  const { id = '', computerId = '' } = useParams()
  const { isAdmin } = useAuth()
  const scan = useComplianceScan()
  const catalog = useComplianceCatalog()
  const q = useComplianceComputer(id, computerId)
  const [busy, setBusy] = useState(false)
  const [confirmRemediate, setConfirmRemediate] = useState(false)
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set(quietStatuses))

  if (q.isPending) return <LoadingBlock />
  if (q.error || !q.data) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'Not found'}</Alert>
  const d = q.data
  const b = d.baseline
  const c = d.computer
  const rules = d.rules ?? []
  const count = (s: string) => rules.filter((r) => r.status === s).length
  const good = count('pass') + count('remediated')
  const scored = good + count('fail') + count('error')
  const score = scored ? (good / scored) * 100 : 0
  const lastChecked = rules.reduce<string | undefined>((a, r) => (r.checked_at && (!a || r.checked_at > a) ? r.checked_at : a), undefined)
  const canScan = isAdmin && b.enabled && c.agent_enrolled
  const benchTitle = benchmarkName(b.benchmark, catalog.data?.baselines.find((x) => x.key === b.benchmark)?.title)
  const groups = ruleStatusOrder.map((s) => [s, rules.filter((r) => r.status === s)] as const).filter(([, rs]) => rs.length > 0)
  const unknown = rules.filter((r) => !ruleStatusOrder.includes(r.status))
  if (unknown.length) groups.push(['other', unknown] as const)

  const doScan = async (remediate: boolean) => {
    setBusy(true)
    await scan(id, { computerIds: [c.id], remediate, computerName: c.name })
    setBusy(false)
  }

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Compliance', to: '/compliance' }, { label: b.name, to: `/compliance/${id}?tab=computers` }, { label: c.name || c.id }]}
        title={
          <span className="inline-flex items-center gap-3">
            <Laptop className="size-6 text-ink-faint" aria-hidden="true" />
            {c.name || c.id}
          </span>
        }
        description={
          <>
            {b.name} · {benchTitle}
          </>
        }
        meta={
          <>
            <ScoreMeter score={score} hasData={scored > 0} />
            <span className="text-xs text-ink-muted">macOS {c.os_version || '?'}</span>
            <span className="text-xs text-ink-muted">
              Last checked <RelativeTime value={lastChecked} emptyLabel="never" />
            </span>
            <Link to={`/computers/${c.id}/compliance`} className="text-xs text-accent-700 hover:underline dark:text-accent-300">
              Open computer
            </Link>
          </>
        }
        actions={
          isAdmin && (
            <>
              <Button icon={<Wrench />} onClick={() => setConfirmRemediate(true)} disabled={!canScan}>
                Scan &amp; remediate
              </Button>
              <Button variant="primary" icon={<Play />} loading={busy} onClick={() => doScan(false)} disabled={!canScan}>
                Scan now
              </Button>
            </>
          )
        }
      />
      <div className="flex flex-col gap-6">
        {!c.agent_enrolled && <Alert tone="warning" title="No VibeMDM agent on this Mac">Compliance scans run through the agent. Install it to scan this Mac.</Alert>}
        {!b.enabled && <Alert tone="info">This baseline is disabled, so it isn&apos;t scanned.</Alert>}

        <Card className="flex flex-wrap gap-x-5 gap-y-2 px-4 py-3">
          {ruleStatusOrder.map((s) => {
            const n = count(s)
            const m = statusMeta(s)
            return (
              <span key={s} className={cn('inline-flex items-center gap-1.5 text-[13px]', n === 0 && 'opacity-50')}>
                <m.icon className={cn('size-4', n > 0 ? m.ink : 'text-ink-faint')} aria-hidden="true" />
                <span className="text-ink-muted">{m.label}</span>
                <span className="tabular-nums text-ink">{n}</span>
              </span>
            )
          })}
        </Card>

        {rules.length === 0 ? (
          <Card className="px-4 py-10 text-center text-[13px] text-ink-muted">This baseline has no rules.</Card>
        ) : (
          groups.map(([status, rs]) => {
            const m = statusMeta(status)
            const isCollapsed = collapsed.has(status)
            return (
              <Card key={status}>
                <button
                  type="button"
                  className="flex w-full items-center gap-2 rounded-xl px-4 py-3 text-left focus-ring"
                  onClick={() =>
                    setCollapsed((cur) => {
                      const n = new Set(cur)
                      if (n.has(status)) n.delete(status)
                      else n.add(status)
                      return n
                    })
                  }
                  aria-expanded={!isCollapsed}
                >
                  {isCollapsed ? <ChevronRight className="size-4 text-ink-faint" /> : <ChevronDown className="size-4 text-ink-faint" />}
                  <m.icon className={cn('size-4', m.ink)} aria-hidden="true" />
                  <span className="text-[13px] font-semibold text-ink">{m.label}</span>
                  <span className="text-xs tabular-nums text-ink-muted">{rs.length}</span>
                  {status === 'not_applicable' && <span className="ml-2 text-xs text-ink-faint">Not part of mSCP for macOS {c.os_version || 'this version'}</span>}
                </button>
                {!isCollapsed && (
                  <ul className="border-t border-line">
                    {rs.map((r) => (
                      <ResultRow key={r.id} r={r} />
                    ))}
                  </ul>
                )}
              </Card>
            )
          })
        )}

        <Card>
          <CardHeader title="Recent scans" />
          <ScansTable scans={d.scans ?? []} showComputer={false} />
        </Card>
      </div>
      <ComplianceAttribution release={catalog.data?.release} />
      <ConfirmDialog
        open={confirmRemediate}
        onClose={() => setConfirmRemediate(false)}
        onConfirm={async () => {
          await doScan(true)
          setConfirmRemediate(false)
        }}
        title={`Scan and remediate ${c.name || 'this Mac'}?`}
        description="The Mac runs the checks, then mSCP's fix script for every failing rule that has one, as root, and checks again. Settings change without asking the user."
        confirmLabel="Scan & remediate"
      />
    </>
  )
}

function ResultRow({ r }: { r: ComplianceComputerRule }) {
  const expandable = !!(r.actual || r.expected || (r.detail && r.status !== 'excluded' && r.status !== 'manual') || r.previous_status)
  const [open, setOpen] = useState(false)
  const isDrift = drifted(r)
  return (
    <li className="border-b border-line last:border-b-0">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 px-4 py-2.5">
        <button
          type="button"
          disabled={!expandable}
          onClick={() => setOpen((o) => !o)}
          className="flex min-w-0 flex-1 basis-80 items-start gap-2 rounded text-left focus-ring disabled:cursor-default"
          aria-expanded={expandable ? open : undefined}
        >
          {expandable ? (
            open ? (
              <ChevronDown className="mt-0.5 size-4 shrink-0 text-ink-faint" />
            ) : (
              <ChevronRight className="mt-0.5 size-4 shrink-0 text-ink-faint" />
            )
          ) : (
            <span className="w-4 shrink-0" aria-hidden="true" />
          )}
          <span className="min-w-0">
            <span className="block text-[13px] font-medium text-ink">{r.title}</span>
            <span className="mt-0.5 flex flex-wrap items-center gap-1.5">
              <span className="font-mono text-[11px] text-ink-faint">{r.id}</span>
              <span className="text-[11px] text-ink-muted">{sectionLabel(r.section)}</span>
              <RuleSeverityBadge severity={r.severity} />
              {(r.status === 'fail' || r.status === 'error') && <RuleCapabilityBadges hasFix={r.has_fix} />}
            </span>
            {(r.status === 'excluded' || r.status === 'manual') && r.detail && <span className="mt-1 block text-xs text-ink-muted">{r.detail}</span>}
          </span>
        </button>
        <div className="flex items-center gap-2">
          {isDrift && (
            <Badge tone="warning" icon={<TrendingDown />} title={`Previously ${statusMeta(r.previous_status ?? '').label.toLowerCase()}`}>
              Drifted
            </Badge>
          )}
          <RuleStatusBadge status={r.status} />
          {r.checked_at && <RelativeTime value={r.checked_at} className="hidden w-24 text-right text-xs text-ink-faint sm:inline" />}
        </div>
      </div>
      {open && expandable && (
        <div className="flex flex-col gap-2.5 px-4 pb-3.5 pl-10">
          {(r.actual || r.expected) && (
            <dl className="grid gap-2 sm:grid-cols-2">
              <div className="min-w-0 rounded-lg border border-line bg-surface-sunken/60 px-3 py-2">
                <dt className="micro-label">Actual</dt>
                <dd className="mt-1 break-all font-mono text-xs text-ink">{r.actual || <span className="text-ink-faint">(empty)</span>}</dd>
              </div>
              <div className="min-w-0 rounded-lg border border-line bg-surface-sunken/60 px-3 py-2">
                <dt className="micro-label">Expected</dt>
                <dd className="mt-1 break-all font-mono text-xs text-ink">{r.expected || <span className="text-ink-faint">—</span>}</dd>
              </div>
            </dl>
          )}
          {r.detail && r.status !== 'excluded' && r.status !== 'manual' && <p className="whitespace-pre-wrap break-words text-xs text-ink-muted">{r.detail}</p>}
          {r.previous_status && r.previous_status !== r.status && (
            <p className="text-xs text-ink-muted">
              Previously: <span className="text-ink">{statusMeta(r.previous_status).label.toLowerCase()}</span>
              {isDrift && ' (this rule drifted)'}
            </p>
          )}
        </div>
      )}
    </li>
  )
}
