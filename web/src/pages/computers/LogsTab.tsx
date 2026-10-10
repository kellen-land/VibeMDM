import { useState } from 'react'
import { Download, PackageSearch } from 'lucide-react'
import { qk, useDiagnostics, useInvalidate } from '../../api/queries'
import { api, downloadFile, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { ComputerDetail, DiagnosticBundle } from '../../api/types'
import { CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge, type BadgeTone } from '../../components/ui/Badge'
import { Table } from '../../components/ui/Table'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { Spinner } from '../../components/ui/Spinner'
import { useToast } from '../../components/ui/Toast'
import { LogExplorer } from '../../components/logs/LogExplorer'
import { formatBytes } from '../../lib/format'

export function ComputerLogsTab({ c }: { c: ComputerDetail }) {
  return (
    <>
      <CardHeader title="Agent logs" description={c.agent_enrolled ? 'Log entries uploaded by the VibeMDM agent on this Mac. Click a row for the full entry.' : 'The VibeMDM agent is not installed on this Mac, so no agent logs are collected.'} />
      <LogExplorer key={c.id} source="agent" computerId={c.id} height="56vh" />
      <div className="border-t border-line">
        <DiagnosticsSection c={c} />
      </div>
    </>
  )
}

const bundleTone: Record<string, BadgeTone> = { requested: 'info', uploaded: 'good', failed: 'critical' }

function BundleStatus({ status }: { status: string }) {
  return (
    <Badge tone={bundleTone[status] ?? 'neutral'} dot={status !== 'requested'} icon={status === 'requested' ? <Spinner className="size-3" /> : undefined}>
      {status === 'requested' ? 'Collecting' : status.replace(/^\w/, (x) => x.toUpperCase())}
    </Badge>
  )
}

function DiagnosticsSection({ c }: { c: ComputerDetail }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const q = useDiagnostics(c.id)
  const [requesting, setRequesting] = useState(false)
  const [downloading, setDownloading] = useState<string | null>(null)

  const collect = async () => {
    setRequesting(true)
    try {
      await api.post<DiagnosticBundle>(`/api/v1/computers/${c.id}/diagnostics`)
      toast.success('Diagnostics requested', c.agent_online ? 'The agent is collecting the bundle now.' : 'The bundle is collected when the agent next checks in.')
      await invalidate(qk.diagnostics(c.id), qk.computerTasks(c.id))
    } catch (e) {
      toast.error('Request failed', errorMessage(e))
    } finally {
      setRequesting(false)
    }
  }

  const download = async (b: DiagnosticBundle) => {
    setDownloading(b.id)
    try {
      await downloadFile(`/api/v1/diagnostics/${b.id}/download`, b.filename ? { filename: b.filename } : undefined)
    } catch (e) {
      toast.error('Download failed', errorMessage(e))
    } finally {
      setDownloading(null)
    }
  }

  return (
    <>
      <CardHeader
        title="Diagnostics bundles"
        description={
          <>
            A .tar.gz collected on demand by the agent: the agent log, the macOS <span className="font-mono">mdmclient</span> log for the last 2 hours, profile status and system information. The agent&apos;s device token is redacted. Downloads are audited.
          </>
        }
        actions={
          isAdmin && (
            <Button icon={<PackageSearch />} loading={requesting} onClick={collect} disabled={!c.agent_enrolled} title={c.agent_enrolled ? undefined : 'Requires the VibeMDM agent'}>
              Collect diagnostics
            </Button>
          )
        }
      />
      <Table<DiagnosticBundle>
        rows={q.data?.items ?? undefined}
        loading={q.isPending}
        error={q.error ? errorMessage(q.error) : null}
        rowKey={(b) => b.id}
        emptyTitle="No diagnostics collected"
        emptyDescription={isAdmin ? 'Use Collect diagnostics to request a bundle from this Mac.' : 'An administrator can request a bundle from this Mac.'}
        columns={[
          { key: 'status', header: 'Status', render: (b) => <BundleStatus status={b.status} /> },
          { key: 'requested_by', header: 'Requested by', render: (b) => <span className="text-ink">{b.requested_by || '—'}</span> },
          { key: 'created', header: 'Requested', render: (b) => <RelativeTime value={b.created_at} className="text-ink-muted" /> },
          { key: 'uploaded', header: 'Uploaded', render: (b) => <RelativeTime value={b.uploaded_at} emptyLabel="—" className="text-ink-muted" /> },
          { key: 'size', header: 'Size', align: 'right', render: (b) => <span className="tabular-nums text-ink-muted">{b.status === 'uploaded' ? formatBytes(b.size_bytes) : '—'}</span> },
          {
            key: 'detail',
            header: 'Details',
            render: (b) =>
              b.error ? (
                <span className="text-xs text-critical">{b.error}</span>
              ) : b.filename ? (
                <span className="font-mono text-xs text-ink-muted" title={b.sha256 ? `SHA-256 ${b.sha256}` : undefined}>
                  {b.filename}
                </span>
              ) : (
                <span className="text-ink-faint">—</span>
              ),
          },
          {
            key: 'actions',
            header: '',
            align: 'right',
            render: (b) =>
              isAdmin && b.status === 'uploaded' ? (
                <Button size="xs" variant="ghost" icon={<Download />} loading={downloading === b.id} onClick={() => download(b)}>
                  Download
                </Button>
              ) : null,
          },
        ]}
      />
    </>
  )
}
