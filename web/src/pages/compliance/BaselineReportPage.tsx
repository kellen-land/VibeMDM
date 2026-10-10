import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import {
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  CircleOff,
  Clock,
  Download,
  Gauge,
  Laptop,
  ListX,
  MinusCircle,
  Pencil,
  Play,
  ScanLine,
  ShieldCheck,
  Target,
  Trash2,
  TrendingDown,
  Wrench,
  X,
  XCircle,
  type LucideIcon,
} from 'lucide-react'
import { qk, useComplianceReport, useComplianceScans, useInvalidate } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { ComplianceComputerDetail, ComplianceComputerSummary, ComplianceReport, ComplianceReportRule, ComplianceScan } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button, IconButton } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Tabs } from '../../components/ui/Tabs'
import { Table } from '../../components/ui/Table'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock, Spinner } from '../../components/ui/Spinner'
import { SearchInput } from '../../components/ui/SearchInput'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { useToast } from '../../components/ui/Toast'
import { ScopeSummary } from '../../components/pickers/ScopeEditor'
import { BarHeadline, SegmentBar } from '../../components/charts/ComplianceBar'
import {
  ComplianceAttribution,
  Discussion,
  References,
  ResultMiniBar,
  RuleCapabilityBadges,
  RuleScripts,
  RuleSeverityBadge,
  RuleStatusBadge,
  ScanStatusBadge,
  ScoreMeter,
  benchmarkName,
  intervalLabel,
  ruleStatusMeta,
  scoreText,
  sectionLabel,
  triggerLabel,
  useComplianceScan,
} from '../../components/compliance/compliance'
import { compactNumber, formatMs } from '../../lib/format'
import { cn } from '../../lib/cn'

type TabKey = 'rules' | 'computers' | 'scans'
type RuleFilter = 'failing' | 'drifted' | 'exempt' | 'all'
type FleetKey = 'compliant' | 'failing' | 'errors' | 'unscanned'

const isCompliant = (c: ComplianceComputerSummary) => c.total > 0 && c.failed === 0 && c.errors === 0

export function BaselineReportPage() {
  const { id = '' } = useParams()
  const [sp, setSp] = useSearchParams()
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const scan = useComplianceScan()
  const q = useComplianceReport(id)
  const [busy, setBusy] = useState<string | null>(null)
  const [confirmRemediate, setConfirmRemediate] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [fleetFilter, setFleetFilter] = useState<FleetKey | ''>('')

  const tabParam = sp.get('tab')
  const tab: TabKey = tabParam === 'computers' || tabParam === 'scans' ? tabParam : 'rules'
  const setTab = (k: TabKey) =>
    setSp(
      (p) => {
        const n = new URLSearchParams(p)
        if (k === 'rules') n.delete('tab')
        else n.set('tab', k)
        return n
      },
      { replace: true },
    )
  const ruleId = sp.get('rule')
  const openRule = (rid: string | null) =>
    setSp(
      (p) => {
        const n = new URLSearchParams(p)
        if (rid) n.set('rule', rid)
        else n.delete('rule')
        return n
      },
      { replace: !rid },
    )

  const run = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key)
    await fn()
    setBusy(null)
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/compliance/baselines/${id}`)
      await invalidate(qk.compliance, qk.profiles)
      toast.success('Baseline deleted', q.data?.baseline.deploy_profile ? 'Its configuration profile is being removed from Macs.' : undefined)
      navigate('/compliance')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (q.isPending) return <LoadingBlock />
  if (q.error || !q.data) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'Baseline not found'}</Alert>
  const r = q.data
  const b = r.baseline
  const sum = r.summary
  const computers = r.computers ?? []
  const rules = r.rules ?? []
  const benchTitle = benchmarkName(b.benchmark, r.benchmark_title)

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Compliance', to: '/compliance' }, { label: b.name }]}
        title={b.name}
        description={
          <>
            {benchTitle} · mSCP {r.release}
            {b.description && <span className="mt-1 block text-ink-muted">{b.description}</span>}
          </>
        }
        meta={
          <>
            {!b.enabled && <Badge icon={<CircleOff />}>Disabled</Badge>}
            {b.auto_remediate && (
              <Badge tone="accent" icon={<Wrench />}>
                Auto-remediate
              </Badge>
            )}
            {b.deploy_profile && (
              <Badge icon={<ShieldCheck />} title={`Deployed as “Compliance: ${b.name}” in Profiles`}>
                Profile deployed
              </Badge>
            )}
            <span className="inline-flex items-center gap-1 text-xs text-ink-muted">
              <Clock className="size-3.5 text-ink-faint" aria-hidden="true" />
              Scanned {intervalLabel(b.scan_interval_hours).toLowerCase()}
            </span>
            <span className="text-xs text-ink-muted">
              Scope: <ScopeSummary scope={b.scope} />
            </span>
          </>
        }
        actions={
          <>
            <a
              href={`/api/v1/compliance/baselines/${id}/export.csv`}
              download
              className="inline-flex h-[34px] items-center gap-2 rounded-md px-3.5 text-[13px] font-medium text-ink-muted hover:bg-line/40 hover:text-ink focus-ring"
            >
              <Download className="size-4" aria-hidden="true" />
              Export CSV
            </a>
            {isAdmin && (
              <>
                <IconButton label="Delete baseline" variant="ghost" size="md" onClick={() => setConfirmDelete(true)}>
                  <Trash2 />
                </IconButton>
                <Link to={`/compliance/${id}/edit`}>
                  <Button icon={<Pencil />}>Edit</Button>
                </Link>
                <Button icon={<Wrench />} onClick={() => setConfirmRemediate(true)} disabled={!b.enabled}>
                  Scan &amp; remediate
                </Button>
                <Button variant="primary" icon={<Play />} loading={busy === 'scan'} onClick={() => run('scan', () => scan(id, { remediate: false }))} disabled={!b.enabled} title={b.enabled ? undefined : 'Enable the baseline to scan'}>
                  Scan now
                </Button>
              </>
            )}
          </>
        }
      />

      <div className="flex flex-col gap-6">
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
          <Tile label="Fleet score" icon={Gauge} value={scoreText(sum.score, sum.scanned > 0)} sub={sum.scanned ? 'average passing' : 'no scans yet'} />
          <Tile label="Compliant Macs" icon={CheckCircle2} value={sum.scanned ? `${compactNumber(sum.compliant)}/${compactNumber(sum.scanned)}` : '—'} sub="of scanned" />
          <Tile label="Targeted" icon={Target} value={compactNumber(sum.targeted)} sub={sum.targeted - sum.scanned > 0 ? `${sum.targeted - sum.scanned} not scanned yet` : 'in scope'} />
          <Tile label="Failing rules" icon={ListX} value={compactNumber(sum.failing_rules)} sub="fail on at least one Mac" onClick={() => setTab('rules')} />
          <Tile label="Drifted" icon={TrendingDown} value={compactNumber(sum.drifted)} sub="passed before, failing now" tone={sum.drifted > 0 ? 'warn' : undefined} />
          <Tile label="Remediated" icon={Wrench} value={compactNumber(sum.remediated)} sub="fixed by auto-remediate" />
        </div>

        <FleetCard
          report={r}
          active={fleetFilter}
          onSelect={(k) => {
            setFleetFilter(k)
            if (k) setTab('computers')
          }}
        />

        <div>
          <Tabs<TabKey>
            items={[
              { key: 'rules', label: 'Rules', count: rules.length },
              { key: 'computers', label: 'Computers', count: computers.length },
              { key: 'scans', label: 'Scans' },
            ]}
            value={tab}
            onChange={setTab}
            className="mb-5"
          />
          {tab === 'rules' && <RulesTab rules={rules} scanned={sum.scanned} onOpen={openRule} />}
          {tab === 'computers' && <ComputersTab baselineId={id} computers={computers} filter={fleetFilter} setFilter={setFleetFilter} canScan={isAdmin && b.enabled} />}
          {tab === 'scans' && <ScansTab baselineId={id} />}
        </div>
      </div>

      <ComplianceAttribution release={r.release} />
      <RuleDrawer report={r} ruleId={ruleId} onClose={() => openRule(null)} />
      <ConfirmDialog
        open={confirmRemediate}
        onClose={() => setConfirmRemediate(false)}
        onConfirm={async () => {
          await scan(id, { remediate: true })
          setConfirmRemediate(false)
        }}
        title="Scan and remediate every Mac in scope?"
        description="Each Mac runs the checks, then mSCP's fix script for every failing rule that has one, as root, and checks again. Settings change without asking the user. Rules that can only be enforced by a profile stay failing."
        confirmLabel="Scan & remediate"
      />
      <ConfirmDialog
        open={confirmDelete}
        onClose={() => setConfirmDelete(false)}
        onConfirm={doDelete}
        destructive
        title={`Delete “${b.name}”?`}
        description={
          <>
            All scan results and history for this baseline are removed.
            {b.deploy_profile || b.profile_id ? <> The configuration profile &ldquo;Compliance: {b.name}&rdquo; is withdrawn from every Mac.</> : null}
          </>
        }
        confirmLabel="Delete baseline"
      />
    </>
  )
}

function Tile({ label, icon: Icon, value, sub, tone, onClick }: { label: string; icon: LucideIcon; value: ReactNode; sub?: ReactNode; tone?: 'warn'; onClick?: () => void }) {
  const body = (
    <Card className={cn('group relative h-full overflow-hidden px-4 pt-3.5 pb-3.5 transition-colors', onClick && 'hover:border-line-strong')}>
      <div className="flex items-start justify-between gap-2">
        <p className="micro-label">{label}</p>
        <Icon className={cn('size-4 shrink-0', tone === 'warn' ? 'text-warning' : 'text-ink-faint')} aria-hidden="true" />
      </div>
      <p className="mt-2 text-[28px] font-semibold tracking-tight text-ink leading-9 tabular-nums">{value}</p>
      {sub && <p className="mt-0.5 truncate text-[11px] text-ink-muted">{sub}</p>}
      <span className={cn('absolute inset-x-0 bottom-0 h-[2px]', tone === 'warn' ? 'bg-warning/70' : onClick ? 'bg-transparent group-hover:bg-accent-500/70' : 'bg-transparent')} aria-hidden="true" />
    </Card>
  )
  return onClick ? (
    <button type="button" onClick={onClick} className="block h-full rounded-xl text-left focus-ring">
      {body}
    </button>
  ) : (
    body
  )
}

function FleetCard({ report, active, onSelect }: { report: ComplianceReport; active: FleetKey | ''; onSelect: (k: FleetKey | '') => void }) {
  const cs = report.computers ?? []
  const compliant = cs.filter(isCompliant).length
  const failing = cs.filter((c) => c.failed > 0).length
  const errors = cs.filter((c) => c.failed === 0 && c.errors > 0).length
  const unscanned = Math.max(0, report.summary.targeted - report.summary.scanned)
  const segments = [
    { key: 'failing' as FleetKey, label: 'Failing', n: failing, fill: ruleStatusMeta.fail.fill, ink: ruleStatusMeta.fail.ink, icon: XCircle },
    { key: 'errors' as FleetKey, label: 'Check errors', n: errors, fill: ruleStatusMeta.error.fill, ink: ruleStatusMeta.error.ink, icon: AlertTriangle },
    { key: 'compliant' as FleetKey, label: 'Compliant', n: compliant, fill: ruleStatusMeta.pass.fill, ink: ruleStatusMeta.pass.ink, icon: CheckCircle2 },
    { key: 'unscanned' as FleetKey, label: 'Not scanned yet', n: unscanned, fill: 'hatch-neutral', ink: 'text-ink-faint', icon: CircleDashed },
  ]
  return (
    <Card>
      <CardHeader title="Fleet" description="Macs in scope by result of their latest scan. Select a segment to list those Macs." />
      <CardBody>
        <SegmentBar<FleetKey>
          segments={segments}
          active={active}
          onSelect={(k) => onSelect(k === 'unscanned' ? '' : k)}
          emptyTitle="No Macs in scope yet"
          emptyDescription="Add computers to the baseline's scope, then scan."
          headline={
            <BarHeadline value={report.summary.scanned ? `${Math.round((compliant / report.summary.scanned) * 100)}%` : '—'}>
              of scanned Macs pass every rule{' '}
              <span className="tabular-nums">
                ({compliant.toLocaleString()} of {report.summary.scanned.toLocaleString()})
              </span>
            </BarHeadline>
          }
        />
      </CardBody>
    </Card>
  )
}

// ---- rules ----

function RulesTab({ rules, scanned, onOpen }: { rules: ComplianceReportRule[]; scanned: number; onOpen: (id: string) => void }) {
  const [filter, setFilter] = useState<RuleFilter>(scanned > 0 ? 'failing' : 'all')
  const [search, setSearch] = useState('')
  const counts: Record<RuleFilter, number> = {
    failing: rules.filter((r) => r.failed > 0 && !r.excluded).length,
    drifted: rules.filter((r) => r.drifted > 0).length,
    exempt: rules.filter((r) => !!r.excluded).length,
    all: rules.length,
  }
  const s = search.trim().toLowerCase()
  const shown = rules.filter((r) => {
    if (filter === 'failing' && !(r.failed > 0 && !r.excluded)) return false
    if (filter === 'drifted' && r.drifted === 0) return false
    if (filter === 'exempt' && !r.excluded) return false
    return !s || r.title.toLowerCase().includes(s) || r.id.toLowerCase().includes(s)
  })
  return (
    <Card>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-2.5">
        <Tabs<RuleFilter>
          variant="pills"
          value={filter}
          onChange={setFilter}
          items={[
            { key: 'failing', label: 'Failing', count: counts.failing },
            { key: 'drifted', label: 'Drifted', count: counts.drifted },
            { key: 'exempt', label: 'Exempted', count: counts.exempt },
            { key: 'all', label: 'All', count: counts.all },
          ]}
        />
        <SearchInput value={search} onChange={setSearch} placeholder="Search rules…" className="w-full sm:w-64" />
      </div>
      <Table<ComplianceReportRule>
        rows={shown}
        rowKey={(r) => r.id}
        onRowClick={(r) => onOpen(r.id)}
        emptyTitle={filter === 'failing' && !s ? (scanned ? 'No failing rules' : 'No scan results yet') : 'No rules match'}
        emptyDescription={filter === 'failing' && !s ? (scanned ? 'Every enforced rule passes on every scanned Mac.' : 'Results appear here once the agent on each Mac reports its first scan.') : undefined}
        rowClassName={(r) => (r.excluded ? 'opacity-70' : undefined)}
        columns={[
          {
            key: 'rule',
            header: 'Rule',
            render: (r) => (
              <span className="block min-w-0">
                <span className="block font-medium text-ink">{r.title}</span>
                <span className="mt-0.5 flex flex-wrap items-center gap-1.5">
                  <span className="font-mono text-[11px] text-ink-faint">{r.id}</span>
                  {r.excluded && (
                    <Badge icon={<MinusCircle />} title={r.excluded}>
                      Exempt
                    </Badge>
                  )}
                  {!r.in_benchmark && <Badge>Added</Badge>}
                </span>
              </span>
            ),
          },
          { key: 'section', header: 'Section', render: (r) => <span className="whitespace-nowrap text-ink-muted">{sectionLabel(r.section)}</span> },
          { key: 'severity', header: 'Severity', render: (r) => (r.severity ? <RuleSeverityBadge severity={r.severity} /> : <span className="text-ink-faint">—</span>) },
          { key: 'results', header: 'Results', render: (r) => <ResultMiniBar passed={r.passed} remediated={r.remediated} errors={r.errors} failed={r.failed} /> },
          {
            key: 'failed',
            header: 'Failed',
            align: 'right',
            render: (r) =>
              r.failed > 0 ? (
                <span className="inline-flex items-center gap-1 tabular-nums text-ink">
                  <XCircle className="size-3.5 text-critical" aria-hidden="true" />
                  {r.failed}
                </span>
              ) : (
                <span className="tabular-nums text-ink-faint">0</span>
              ),
          },
          {
            key: 'drifted',
            header: 'Drifted',
            align: 'right',
            render: (r) =>
              r.drifted > 0 ? (
                <span className="inline-flex items-center gap-1 tabular-nums text-ink">
                  <TrendingDown className="size-3.5 text-warning" aria-hidden="true" />
                  {r.drifted}
                </span>
              ) : (
                <span className="tabular-nums text-ink-faint">0</span>
              ),
          },
        ]}
      />
    </Card>
  )
}

/** Slide-over for one rule: counts, explanation, references, scripts, and (on demand) which Macs fail it. */
function RuleDrawer({ report, ruleId, onClose }: { report: ComplianceReport; ruleId: string | null; onClose: () => void }) {
  const rule = (report.rules ?? []).find((r) => r.id === ruleId)
  useEffect(() => {
    if (!ruleId) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [ruleId, onClose])
  if (!ruleId) return null
  return createPortal(
    <div className="fixed inset-0 z-50 animate-fade-in" role="dialog" aria-modal="true" aria-label="Rule details">
      <div className="absolute inset-0 bg-black/45 backdrop-blur-[2px]" onClick={onClose} />
      <aside className="absolute inset-y-0 right-0 flex w-full max-w-xl flex-col border-l border-line-strong/60 bg-surface shadow-2xl">
        <div className="flex items-start justify-between gap-3 px-5 pt-4 pb-3">
          <div className="min-w-0">
            {rule ? (
              <>
                <div className="flex flex-wrap items-center gap-1.5">
                  <RuleSeverityBadge severity={rule.severity} />
                  <RuleCapabilityBadges hasFix={rule.has_fix} hasProfile={rule.has_profile} manualReason={rule.manual_reason} />
                  <Badge>{sectionLabel(rule.section)}</Badge>
                </div>
                <h2 className="mt-2 text-base font-semibold text-ink break-words">{rule.title}</h2>
                <p className="mt-0.5 font-mono text-xs text-ink-muted">{rule.id}</p>
              </>
            ) : (
              <h2 className="text-base font-semibold text-ink">Rule not in this baseline</h2>
            )}
          </div>
          <IconButton label="Close" variant="ghost" onClick={onClose} className="-mr-2">
            <X />
          </IconButton>
        </div>
        {rule && (
          <div className="flex-1 overflow-y-auto">
            {rule.excluded && (
              <div className="px-5 pb-4">
                <Alert tone="info" title="Exempt in this baseline">
                  {rule.excluded}
                </Alert>
              </div>
            )}
            <DrawerSection title="Results">
              <div className="flex flex-wrap gap-x-5 gap-y-2 text-[13px]">
                {(
                  [
                    ['fail', rule.failed],
                    ['error', rule.errors],
                    ['remediated', rule.remediated],
                    ['pass', rule.passed],
                  ] as const
                ).map(([k, n]) => {
                  const m = ruleStatusMeta[k]
                  return (
                    <span key={k} className="inline-flex items-center gap-1.5">
                      <m.icon className={cn('size-4', n > 0 ? m.ink : 'text-ink-faint')} aria-hidden="true" />
                      <span className="text-ink-muted">{m.label}</span>
                      <span className="tabular-nums text-ink">{n}</span>
                    </span>
                  )
                })}
                {rule.drifted > 0 && (
                  <span className="inline-flex items-center gap-1.5">
                    <TrendingDown className="size-4 text-warning" aria-hidden="true" />
                    <span className="text-ink-muted">Drifted</span>
                    <span className="tabular-nums text-ink">{rule.drifted}</span>
                  </span>
                )}
              </div>
              {rule.failed + rule.errors > 0 && <FailingMacs report={report} ruleId={rule.id} />}
            </DrawerSection>
            <DrawerSection title="Discussion">
              <Discussion text={rule.discussion} />
              {rule.manual_reason && (
                <p className="mt-2 text-xs text-ink-muted">
                  <span className="font-medium text-ink">Not scanned automatically:</span> {rule.manual_reason}
                </p>
              )}
            </DrawerSection>
            {rule.references && Object.keys(rule.references).length > 0 && (
              <DrawerSection title="References">
                <References refs={rule.references} />
              </DrawerSection>
            )}
            <DrawerSection title="Check">
              <RuleScripts ruleId={rule.id} benchmark={report.baseline.benchmark} />
            </DrawerSection>
          </div>
        )}
      </aside>
    </div>,
    document.body,
  )
}

function DrawerSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="border-t border-line px-5 py-4">
      <h3 className="micro-label mb-2.5">{title}</h3>
      {children}
    </section>
  )
}

const FAILING_LOOKUP_CAP = 40

/** Which Macs fail this rule. Loads per-computer results on request, only for Macs that have failures at all. */
function FailingMacs({ report, ruleId }: { report: ComplianceReport; ruleId: string }) {
  const [load, setLoad] = useState(false)
  const candidates = useMemo(() => (report.computers ?? []).filter((c) => c.failed > 0 || c.errors > 0), [report.computers])
  const subset = candidates.slice(0, FAILING_LOOKUP_CAP)
  const results = useQueries({
    queries: subset.map((c) => ({
      queryKey: qk.complianceComputer(report.baseline.id, c.computer_id),
      queryFn: () => api.get<ComplianceComputerDetail>(`/api/v1/compliance/baselines/${report.baseline.id}/computers/${c.computer_id}`),
      enabled: load,
      staleTime: 30_000,
    })),
  })
  if (!load)
    return (
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button size="xs" icon={<Laptop />} onClick={() => setLoad(true)}>
          Show failing Macs
        </Button>
        <span className="text-xs text-ink-faint">Looks up {Math.min(candidates.length, FAILING_LOOKUP_CAP)} Macs with failures.</span>
      </div>
    )
  const pending = results.some((x) => x.isPending)
  const hits = subset
    .map((c, i) => ({ c, row: results[i]?.data?.rules?.find((r) => r.id === ruleId) }))
    .filter((x) => x.row && (x.row.status === 'fail' || x.row.status === 'error'))
  return (
    <div className="mt-3">
      {pending && (
        <p className="mb-2 inline-flex items-center gap-2 text-xs text-ink-muted">
          <Spinner className="size-3.5" /> Looking up results…
        </p>
      )}
      {!pending && hits.length === 0 && <p className="text-xs text-ink-muted">None of the looked-up Macs currently fail this rule.</p>}
      {hits.length > 0 && (
        <ul className="divide-y divide-line rounded-lg border border-line">
          {hits.map(({ c, row }) => (
            <li key={c.computer_id}>
              <Link to={`/compliance/${report.baseline.id}/computers/${c.computer_id}`} className="flex items-center gap-3 px-3 py-2 text-[13px] hover:bg-surface-sunken/60 focus-ring">
                <RuleStatusBadge status={row!.status} />
                <span className="min-w-0 flex-1 truncate font-medium text-ink">{c.computer_name || c.serial_number}</span>
                {row!.actual && (
                  <span className="max-w-40 truncate font-mono text-[11px] text-ink-muted" title={`Actual: ${row!.actual}`}>
                    {row!.actual}
                  </span>
                )}
              </Link>
            </li>
          ))}
        </ul>
      )}
      {candidates.length > FAILING_LOOKUP_CAP && (
        <p className="mt-2 text-xs text-ink-faint">
          Showing results from the first {FAILING_LOOKUP_CAP} of {candidates.length} Macs with failures. Export CSV for the full list.
        </p>
      )}
    </div>
  )
}

// ---- computers ----

function ComputersTab({ baselineId, computers, filter, setFilter, canScan }: { baselineId: string; computers: ComplianceComputerSummary[]; filter: FleetKey | ''; setFilter: (k: FleetKey | '') => void; canScan: boolean }) {
  const navigate = useNavigate()
  const scan = useComplianceScan()
  const [search, setSearch] = useState('')
  const [scanning, setScanning] = useState<string | null>(null)
  const s = search.trim().toLowerCase()
  const shown = computers
    .filter((c) => {
      if (filter === 'compliant' && !isCompliant(c)) return false
      if (filter === 'failing' && c.failed === 0) return false
      if (filter === 'errors' && !(c.failed === 0 && c.errors > 0)) return false
      return !s || c.computer_name.toLowerCase().includes(s) || c.serial_number.toLowerCase().includes(s)
    })
    .sort((a, b) => a.score - b.score || a.computer_name.localeCompare(b.computer_name))
  const f = filter === 'unscanned' ? '' : filter
  return (
    <Card>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-2.5">
        <Tabs<string>
          variant="pills"
          value={f}
          onChange={(k) => setFilter(k as FleetKey | '')}
          items={[
            { key: '', label: 'All', count: computers.length },
            { key: 'failing', label: 'Failing', count: computers.filter((c) => c.failed > 0).length },
            { key: 'errors', label: 'Check errors', count: computers.filter((c) => c.failed === 0 && c.errors > 0).length },
            { key: 'compliant', label: 'Compliant', count: computers.filter(isCompliant).length },
          ]}
        />
        <SearchInput value={search} onChange={setSearch} placeholder="Search by name or serial…" className="w-full sm:w-64" />
      </div>
      <Table<ComplianceComputerSummary>
        rows={shown}
        rowKey={(c) => c.computer_id}
        onRowClick={(c) => navigate(`/compliance/${baselineId}/computers/${c.computer_id}`)}
        emptyTitle={computers.length ? 'No computers match' : 'No scan results yet'}
        emptyDescription={computers.length ? undefined : 'Macs appear here after their first scan. Use Scan now to start one.'}
        columns={[
          {
            key: 'name',
            header: 'Computer',
            render: (c) => (
              <span className="block">
                <span className="block font-medium text-ink">{c.computer_name || c.serial_number}</span>
                <span className="block font-mono text-[11px] text-ink-faint">{c.serial_number}</span>
              </span>
            ),
          },
          { key: 'score', header: 'Score', render: (c) => <ScoreMeter score={c.score} hasData={c.total > 0} size="sm" /> },
          { key: 'passed', header: 'Passed', align: 'right', render: (c) => <span className="tabular-nums text-ink-muted">{c.passed + c.remediated}</span> },
          {
            key: 'failed',
            header: 'Failed',
            align: 'right',
            render: (c) =>
              c.failed > 0 ? (
                <span className="inline-flex items-center gap-1 tabular-nums text-ink">
                  <XCircle className="size-3.5 text-critical" aria-hidden="true" />
                  {c.failed}
                </span>
              ) : (
                <span className="tabular-nums text-ink-faint">0</span>
              ),
          },
          {
            key: 'errors',
            header: 'Errors',
            align: 'right',
            render: (c) =>
              c.errors > 0 ? (
                <span className="inline-flex items-center gap-1 tabular-nums text-ink">
                  <AlertTriangle className="size-3.5 text-warning" aria-hidden="true" />
                  {c.errors}
                </span>
              ) : (
                <span className="tabular-nums text-ink-faint">0</span>
              ),
          },
          { key: 'os', header: 'macOS', render: (c) => <span className="tabular-nums text-ink-muted">{c.os_version || '—'}</span> },
          {
            key: 'last',
            header: 'Last scan',
            render: (c) => (
              <span className="flex flex-wrap items-center gap-2">
                <RelativeTime value={c.last_scan_at} className="text-ink-muted" emptyLabel="—" />
                {c.last_scan_status && c.last_scan_status !== 'completed' && <ScanStatusBadge status={c.last_scan_status} />}
              </span>
            ),
          },
          ...(canScan
            ? [
                {
                  key: 'actions',
                  header: '',
                  align: 'right' as const,
                  render: (c: ComplianceComputerSummary) => (
                    <Button
                      size="xs"
                      variant="ghost"
                      icon={<ScanLine />}
                      loading={scanning === c.computer_id}
                      onClick={async (e) => {
                        e.stopPropagation()
                        setScanning(c.computer_id)
                        await scan(baselineId, { computerIds: [c.computer_id], remediate: false, computerName: c.computer_name })
                        setScanning(null)
                      }}
                    >
                      Scan
                    </Button>
                  ),
                },
              ]
            : []),
        ]}
      />
    </Card>
  )
}

// ---- scans ----

export function ScanCounts({ s }: { s: Pick<ComplianceScan, 'passed' | 'failed' | 'errors' | 'remediated' | 'total' | 'status'> }) {
  if (s.status !== 'completed' && s.total === 0) return <span className="text-ink-faint">—</span>
  return (
    <span className="inline-flex flex-wrap items-center gap-x-2.5 gap-y-0.5 text-xs tabular-nums text-ink-muted">
      <span className="inline-flex items-center gap-1" title="Passed">
        <CheckCircle2 className="size-3.5 text-good" aria-hidden="true" />
        {s.passed}
        <span className="sr-only">passed</span>
      </span>
      <span className="inline-flex items-center gap-1" title="Failed">
        <XCircle className={cn('size-3.5', s.failed ? 'text-critical' : 'text-ink-faint')} aria-hidden="true" />
        {s.failed}
        <span className="sr-only">failed</span>
      </span>
      {s.errors > 0 && (
        <span className="inline-flex items-center gap-1" title="Errors">
          <AlertTriangle className="size-3.5 text-warning" aria-hidden="true" />
          {s.errors}
          <span className="sr-only">errors</span>
        </span>
      )}
      {s.remediated > 0 && (
        <span className="inline-flex items-center gap-1" title="Remediated">
          <Wrench className="size-3.5 text-accent-500" aria-hidden="true" />
          {s.remediated}
          <span className="sr-only">remediated</span>
        </span>
      )}
    </span>
  )
}

export function ScansTable({ scans, loading, error, showComputer = true }: { scans: ComplianceScan[] | undefined; loading?: boolean; error?: string | null; showComputer?: boolean }) {
  return (
    <Table<ComplianceScan>
      rows={scans}
      loading={loading}
      error={error}
      rowKey={(s) => s.id}
      dense
      emptyTitle="No scans yet"
      emptyDescription="Scans run on schedule, or when an admin selects Scan now."
      columns={[
        ...(showComputer
          ? [
              {
                key: 'computer',
                header: 'Computer',
                render: (s: ComplianceScan) => (
                  <Link to={`/compliance/${s.baseline_id}/computers/${s.computer_id}`} className="font-medium text-ink hover:underline">
                    {s.computer_name || s.computer_id}
                  </Link>
                ),
              },
            ]
          : []),
        {
          key: 'trigger',
          header: 'Trigger',
          render: (s) => (
            <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-ink-muted">
              {triggerLabel(s.trigger)}
              {s.remediate && s.trigger !== 'manual_remediate' && <Wrench className="size-3.5 text-ink-faint" aria-label="with remediation" />}
            </span>
          ),
        },
        { key: 'status', header: 'Status', render: (s) => <ScanStatusBadge status={s.status} /> },
        { key: 'counts', header: 'Results', render: (s) => <ScanCounts s={s} /> },
        { key: 'duration', header: 'Duration', align: 'right', render: (s) => <span className="tabular-nums text-ink-muted">{s.duration_ms ? formatMs(s.duration_ms) : '—'}</span> },
        { key: 'requested', header: 'Requested', render: (s) => <RelativeTime value={s.requested_at} className="text-ink-muted" /> },
        {
          key: 'error',
          header: 'Error',
          render: (s) =>
            s.error ? (
              <span className="block max-w-64 truncate text-xs text-critical" title={s.error}>
                {s.error}
              </span>
            ) : (
              <span className="text-ink-faint">—</span>
            ),
        },
      ]}
    />
  )
}

function ScansTab({ baselineId }: { baselineId: string }) {
  const q = useComplianceScans(baselineId, { limit: 100 })
  return (
    <Card>
      <ScansTable scans={q.data} loading={q.isPending} error={q.error ? errorMessage(q.error) : null} />
    </Card>
  )
}
