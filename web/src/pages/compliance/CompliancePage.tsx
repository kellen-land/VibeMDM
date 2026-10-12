import { CircleOff, ShieldCheck, Trash2, Wrench } from 'lucide-react'
import { qk, useComplianceBaselines, useComplianceCatalog, useInvalidate } from '../../api/queries'
import { api } from '../../api/client'
import type { ComplianceBaselineRow } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ScopeSummary } from '../../components/pickers/ScopeEditor'
import { IndexPage } from '../../components/templates/IndexPage'
import { ComplianceAttribution, ScoreMeter, benchmarkName, intervalLabel, scoreText } from '../../components/compliance/compliance'

export function CompliancePage() {
  const q = useComplianceBaselines()
  const cat = useComplianceCatalog()
  const invalidate = useInvalidate()
  const titles = new Map((cat.data?.baselines ?? []).map((b) => [b.key, b.title]))
  const rows = q.data
  const benchmarks = [...new Set((rows ?? []).map((b) => b.benchmark))].sort()
  const nBaselines = (n: number) => `${n} ${n === 1 ? 'baseline' : 'baselines'}`

  return (
    <IndexPage<ComplianceBaselineRow>
      title="Compliance"
      description="Security baselines from the macOS Security Compliance Project, checked on every Mac by the VibeMDM agent."
      newTo="/compliance/new"
      newLabel="New baseline"
      rows={rows}
      loading={q.isPending}
      error={q.error}
      rowKey={(b) => b.id}
      rowHref={(b) => `/compliance/${b.id}`}
      rowClassName={(b) => (b.enabled ? undefined : 'opacity-70')}
      search={{ placeholder: 'Search baselines…', text: (b) => `${b.name} ${benchmarkName(b.benchmark, titles.get(b.benchmark))}` }}
      filters={[
        { key: 'state', label: 'Any state', options: [{ value: 'on', label: 'Enabled' }, { value: 'off', label: 'Disabled' }], match: (b, v) => b.enabled === (v === 'on') },
        ...(benchmarks.length > 1
          ? [{ key: 'benchmark', label: 'Any benchmark', options: benchmarks.map((k) => ({ value: k, label: benchmarkName(k, titles.get(k)) })), match: (b: ComplianceBaselineRow, v: string) => b.benchmark === v }]
          : []),
      ]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.compliance, qk.profiles)}
      bulkActions={[
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${nBaselines(n)}?`, description: 'Their scan results are removed, and any configuration profile they deploy is removed from Macs.' },
          run: (b) => api.del(`/api/v1/compliance/baselines/${b.id}`),
          done: (n) => `${nBaselines(n)} deleted`,
        },
      ]}
      empty={{
        title: 'No baselines yet',
        description: "Pick an mSCP benchmark (CIS, NIST 800-53, DISA STIG, …) and the agent checks each Mac in scope against it on a schedule.",
      }}
      footer={<ComplianceAttribution release={cat.data?.release} />}
      columns={[
        {
          key: 'name',
          header: 'Baseline',
          sortValue: (b) => b.name,
          render: (b) => (
            <span className="block min-w-0">
              <span className="flex items-center gap-2">
                <span className="font-medium text-ink">{b.name}</span>
                {!b.enabled && <Badge icon={<CircleOff />}>Disabled</Badge>}
              </span>
              <span className="block truncate text-xs text-ink-muted">{benchmarkName(b.benchmark, titles.get(b.benchmark))}</span>
            </span>
          ),
        },
        {
          key: 'score',
          header: 'Score',
          sortValue: (b) => (b.computers > 0 ? b.score : -1),
          render: (b) => (
            <span className="flex items-center gap-2 whitespace-nowrap">
              <ScoreMeter score={b.score} hasData={b.computers > 0} size="sm" showValue={false} />
              <span className="tabular-nums text-ink">{scoreText(b.score, b.computers > 0)}</span>
            </span>
          ),
        },
        {
          key: 'compliant',
          header: 'Fully compliant',
          sortValue: (b) => b.compliant,
          render: (b) =>
            b.computers > 0 ? (
              <span className="whitespace-nowrap text-ink-muted">
                <span className="tabular-nums text-ink">{b.compliant.toLocaleString()}</span> of <span className="tabular-nums">{b.computers.toLocaleString()}</span>
              </span>
            ) : (
              <span className="text-xs text-ink-faint">Not scanned yet</span>
            ),
        },
        { key: 'scope', header: 'Scope', render: (b) => <ScopeSummary scope={b.scope} /> },
        {
          key: 'enforcement',
          header: 'Enforcement',
          render: (b) =>
            b.auto_remediate || b.deploy_profile ? (
              <span className="flex flex-wrap gap-1">
                {b.auto_remediate && (
                  <Badge tone="accent" icon={<Wrench />}>
                    Auto-remediate
                  </Badge>
                )}
                {b.deploy_profile && <Badge icon={<ShieldCheck />}>Profile</Badge>}
              </span>
            ) : (
              <span className="text-xs text-ink-faint">Report only</span>
            ),
        },
        { key: 'interval', header: 'Scans', sortValue: (b) => b.scan_interval_hours, render: (b) => <span className="whitespace-nowrap text-ink-muted">{intervalLabel(b.scan_interval_hours)}</span> },
        { key: 'updated', header: 'Updated', sortValue: (b) => b.updated_at, render: (b) => <RelativeTime value={b.updated_at} className="text-ink-muted" /> },
      ]}
    />
  )
}
