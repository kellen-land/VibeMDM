import { Link, useNavigate } from 'react-router-dom'
import { CircleDashed, CircleOff, ShieldHalf } from 'lucide-react'
import { useComputerCompliance } from '../../api/queries'
import { errorMessage } from '../../api/client'
import type { ComputerComplianceRow } from '../../api/types'
import { CardHeader } from '../../components/ui/Card'
import { Badge } from '../../components/ui/Badge'
import { Table } from '../../components/ui/Table'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ScanStatusBadge, ScoreMeter, benchmarkName } from '../../components/compliance/compliance'
import { ScanCounts } from '../compliance/BaselineReportPage'

/** Baselines that target (or have scanned) this Mac, with its latest result in each. */
export function ComputerComplianceTab({ id }: { id: string }) {
  const q = useComputerCompliance(id)
  const navigate = useNavigate()
  const scanned = (r: ComputerComplianceRow) => !!r.summary && r.summary.total > 0
  return (
    <>
      <CardHeader
        title="Compliance"
        description="mSCP baselines that apply to this Mac. Scans run through the VibeMDM agent."
        actions={
          <Link to="/compliance" className="text-xs text-accent-700 hover:underline dark:text-accent-300">
            All baselines
          </Link>
        }
      />
      <Table<ComputerComplianceRow>
        rows={q.data}
        loading={q.isPending}
        error={q.error ? errorMessage(q.error) : null}
        rowKey={(r) => r.baseline.id}
        onRowClick={(r) => navigate(`/compliance/${r.baseline.id}/computers/${id}`)}
        emptyTitle="No baselines apply to this Mac"
        emptyDescription="Add this Mac, or a group it belongs to, to a compliance baseline's scope."
        rowClassName={(r) => (!r.in_scope || !r.baseline.enabled ? 'opacity-70' : undefined)}
        columns={[
          {
            key: 'baseline',
            header: 'Baseline',
            render: (r) => (
              <span className="block">
                <Link to={`/compliance/${r.baseline.id}/computers/${id}`} onClick={(e) => e.stopPropagation()} className="block font-medium text-ink hover:underline">
                  {r.baseline.name}
                </Link>
                <span className="block text-xs text-ink-muted">{benchmarkName(r.baseline.benchmark)}</span>
              </span>
            ),
          },
          {
            key: 'state',
            header: 'State',
            render: (r) =>
              !r.in_scope ? (
                <Badge icon={<CircleOff />} title="Results are from before the Mac left the scope">
                  Out of scope
                </Badge>
              ) : !r.baseline.enabled ? (
                <Badge icon={<CircleOff />}>Baseline disabled</Badge>
              ) : !scanned(r) ? (
                <Badge icon={<CircleDashed />}>Not scanned yet</Badge>
              ) : (
                <Badge icon={<ShieldHalf />}>In scope</Badge>
              ),
          },
          { key: 'score', header: 'Score', render: (r) => (scanned(r) ? <ScoreMeter score={r.summary!.score} size="sm" /> : <span className="text-ink-faint">—</span>) },
          {
            key: 'counts',
            header: 'Results',
            render: (r) => (r.summary && scanned(r) ? <ScanCounts s={{ ...r.summary, status: 'completed' }} /> : <span className="text-ink-faint">—</span>),
          },
          {
            key: 'last',
            header: 'Last scan',
            align: 'right',
            render: (r) =>
              r.summary ? (
                <span className="inline-flex items-center justify-end gap-2">
                  {r.summary.last_scan_status && r.summary.last_scan_status !== 'completed' && <ScanStatusBadge status={r.summary.last_scan_status} />}
                  <RelativeTime value={r.summary.last_scan_at} className="text-ink-muted" emptyLabel="—" />
                </span>
              ) : (
                <span className="text-ink-faint">—</span>
              ),
          },
        ]}
      />
    </>
  )
}
