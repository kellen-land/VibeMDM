import { Link } from 'react-router-dom'
import { CircleOff, Clock, Plus, ShieldCheck, ShieldHalf, Wrench } from 'lucide-react'
import { useComplianceBaselines, useComplianceCatalog } from '../../api/queries'
import { errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { ComplianceBaselineRow } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ScopeSummary } from '../../components/pickers/ScopeEditor'
import { ComplianceAttribution, ScoreMeter, benchmarkName, intervalLabel, scoreText } from '../../components/compliance/compliance'
import { cn } from '../../lib/cn'

export function CompliancePage() {
  const { isAdmin } = useAuth()
  const q = useComplianceBaselines()
  const cat = useComplianceCatalog()
  const titles = new Map((cat.data?.baselines ?? []).map((b) => [b.key, b.title]))
  const rows = [...(q.data ?? [])].sort((a, b) => Number(b.enabled) - Number(a.enabled) || a.name.localeCompare(b.name))

  return (
    <>
      <PageHeader
        title="Compliance"
        description="Security baselines from the macOS Security Compliance Project, checked on every Mac by the VibeMDM agent."
        actions={
          isAdmin &&
          rows.length > 0 && (
            <Link to="/compliance/new" data-shortcut="new">
              <Button variant="primary" icon={<Plus />}>
                New baseline
              </Button>
            </Link>
          )
        }
      />
      {q.isPending && <LoadingBlock />}
      {q.error && <Alert tone="critical">{errorMessage(q.error)}</Alert>}
      {q.data && rows.length === 0 && <EmptyIntro isAdmin={isAdmin} release={cat.data?.release} />}
      {rows.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-2">
          {rows.map((b) => (
            <BaselineCard key={b.id} b={b} benchmarkTitle={titles.get(b.benchmark)} />
          ))}
        </div>
      )}
      <ComplianceAttribution release={cat.data?.release} />
    </>
  )
}

function BaselineCard({ b, benchmarkTitle }: { b: ComplianceBaselineRow; benchmarkTitle?: string }) {
  const scanned = b.computers > 0
  return (
    <Link to={`/compliance/${b.id}`} className="block rounded-xl focus-ring">
      <Card className={cn('flex h-full flex-col gap-4 px-4 py-4 transition-colors hover:border-line-strong', !b.enabled && 'opacity-75')}>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="truncate text-[15px] font-semibold text-ink">{b.name}</p>
            <p className="mt-0.5 truncate text-xs text-ink-muted">{benchmarkName(b.benchmark, benchmarkTitle)}</p>
          </div>
          {!b.enabled && <Badge icon={<CircleOff />}>Disabled</Badge>}
        </div>

        <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-2">
          <div>
            <p className="text-[30px] font-semibold leading-9 tracking-tight text-ink tabular-nums">{scoreText(b.score, scanned)}</p>
            <p className="text-xs text-ink-muted">{scanned ? 'average score' : 'Not scanned yet'}</p>
          </div>
          <div className="flex flex-col items-end gap-1.5">
            <ScoreMeter score={b.score} hasData={scanned} size="lg" showValue={false} />
            <p className="text-xs text-ink-muted">
              {scanned ? (
                <>
                  <span className="tabular-nums text-ink">{b.compliant.toLocaleString()}</span> of <span className="tabular-nums">{b.computers.toLocaleString()}</span> {b.computers === 1 ? 'Mac' : 'Macs'} fully compliant
                </>
              ) : (
                <ScopeSummary scope={b.scope} />
              )}
            </p>
          </div>
        </div>

        <div className="mt-auto flex flex-wrap items-center gap-1.5 border-t border-line pt-3 text-xs text-ink-muted">
          {b.auto_remediate && (
            <Badge tone="accent" icon={<Wrench />}>
              Auto-remediate
            </Badge>
          )}
          {b.deploy_profile && <Badge icon={<ShieldCheck />}>Profile</Badge>}
          <span className="inline-flex items-center gap-1">
            <Clock className="size-3.5 text-ink-faint" aria-hidden="true" />
            {intervalLabel(b.scan_interval_hours)}
          </span>
          <span className="ml-auto">
            Updated <RelativeTime value={b.updated_at} />
          </span>
        </div>
      </Card>
    </Link>
  )
}

function EmptyIntro({ isAdmin, release }: { isAdmin: boolean; release?: string }) {
  return (
    <Card className="px-6 py-12">
      <div className="mx-auto flex max-w-xl flex-col items-center text-center">
        <div className="mb-4 flex size-11 items-center justify-center rounded-full bg-accent-500/12 text-accent-400 ring-1 ring-accent-500/25">
          <ShieldHalf className="size-5" aria-hidden="true" />
        </div>
        <h2 className="text-base font-semibold text-ink">Measure your Macs against a security benchmark</h2>
        <p className="mt-2 text-[13px] leading-relaxed text-ink-muted">
          The macOS Security Compliance Project (mSCP){release ? ` ${release}` : ''} is NIST&apos;s open-source set of macOS security rules, with ready-made baselines for CIS, NIST
          800-53, 800-171, DISA STIG, CMMC and more.
        </p>
        <ul className="mt-4 space-y-1.5 text-left text-[13px] text-ink-muted">
          <li>
            <span className="text-ink">Pick a benchmark</span>, then exempt rules or tune values to fit your organization.
          </li>
          <li>
            <span className="text-ink">The agent scans</span> each Mac in scope on a schedule, using mSCP&apos;s own checks.
          </li>
          <li>
            <span className="text-ink">Optionally enforce</span> with auto-remediation and a generated configuration profile.
          </li>
        </ul>
        <div className="mt-6">
          {isAdmin ? (
            <Link to="/compliance/new" data-shortcut="new">
              <Button variant="primary" icon={<Plus />}>
                Create a baseline
              </Button>
            </Link>
          ) : (
            <p className="text-xs text-ink-faint">An administrator can create the first baseline.</p>
          )}
        </div>
      </div>
    </Card>
  )
}
