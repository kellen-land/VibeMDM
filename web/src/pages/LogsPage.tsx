import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Archive, Download, Laptop, Server } from 'lucide-react'
import { useLogArchives } from '../api/queries'
import { downloadFile, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { LogArchive } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardHeader } from '../components/ui/Card'
import { Tabs } from '../components/ui/Tabs'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { RelativeTime } from '../components/ui/RelativeTime'
import { useToast } from '../components/ui/Toast'
import { LogExplorer } from '../components/logs/LogExplorer'
import { formatBytes } from '../lib/format'

const tabs = ['server', 'agents', 'archives'] as const
type Tab = (typeof tabs)[number]

export function LogsPage() {
  const { tab } = useParams()
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  let active: Tab = (tabs as readonly string[]).includes(tab ?? '') ? (tab as Tab) : 'server'
  if (active === 'archives' && !isAdmin) active = 'server'
  const items = [
    { key: 'server' as Tab, label: 'Server', icon: <Server /> },
    { key: 'agents' as Tab, label: 'Agents', icon: <Laptop /> },
    ...(isAdmin ? [{ key: 'archives' as Tab, label: 'Archives', icon: <Archive /> }] : []),
  ]
  return (
    <>
      <PageHeader title="Logs" description="Search and live-tail the VibeMDM server's own logs and the logs agents upload from each Mac." />
      <Tabs className="mb-5" value={active} onChange={(t) => navigate(`/logs/${t}`, { replace: true })} items={items} />
      <Card>
        {active === 'server' && <LogExplorer key="server" source="server" />}
        {active === 'agents' && <LogExplorer key="agent" source="agent" />}
        {active === 'archives' && <ArchivesTab />}
      </Card>
    </>
  )
}

function ArchivesTab() {
  const q = useLogArchives()
  const toast = useToast()
  const [busy, setBusy] = useState<string | null>(null)
  const download = async (a: LogArchive) => {
    setBusy(a.name)
    try {
      await downloadFile(`/api/v1/logs/archives/${encodeURIComponent(a.name)}`, { filename: a.name })
    } catch (e) {
      toast.error('Download failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }
  const rows = [...(q.data ?? [])].sort((a, b) => b.name.localeCompare(a.name))
  return (
    <>
      <CardHeader
        title="Log archives"
        description={
          <>
            Daily gzipped NDJSON files written to the server&apos;s data volume before old entries are pruned. Retention and archiving are set under{' '}
            <Link to="/settings" className="text-ink hover:underline">
              Settings → General
            </Link>
            . Downloads are audited.
          </>
        }
      />
      <Table<LogArchive>
        rows={rows}
        loading={q.isPending}
        error={q.error ? errorMessage(q.error) : null}
        rowKey={(r) => r.name}
        emptyTitle="No archives yet"
        emptyDescription="Archives appear once log retention prunes entries with archiving turned on."
        columns={[
          { key: 'name', header: 'File', render: (r) => <span className="font-mono text-xs text-ink">{r.name}</span> },
          { key: 'size', header: 'Size', align: 'right', render: (r) => <span className="tabular-nums text-ink-muted">{formatBytes(r.size_bytes)}</span> },
          { key: 'modified', header: 'Written', render: (r) => <RelativeTime value={r.modified_at} className="text-ink-muted" /> },
          {
            key: 'actions',
            header: '',
            align: 'right',
            render: (r) => (
              <Button size="xs" variant="ghost" icon={<Download />} loading={busy === r.name} onClick={() => download(r)}>
                Download
              </Button>
            ),
          },
        ]}
      />
    </>
  )
}
