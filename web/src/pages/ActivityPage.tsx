import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { qk, useAllAppLogs, useAllPolicyLogs, useCommands, useInvalidate, useRestrictedEvents, useTasks, useShellSessions, useShellSession } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { Link } from 'react-router-dom'
import { Table } from '../components/ui/Table'
import { Badge, StatusBadge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'
import { Modal } from '../components/ui/Modal'
import { LoadingBlock } from '../components/ui/Spinner'
import { DescriptionList } from '../components/ui/DescriptionList'
import { TranscriptViewer } from '../components/shell/TranscriptViewer'
import { formatBytes, absoluteTime } from '../lib/format'
import type { AgentTask, MDMCommand, ShellSession } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardHeader } from '../components/ui/Card'
import { Tabs } from '../components/ui/Tabs'
import { Select } from '../components/ui/Input'
import { Pagination } from '../components/ui/Pagination'
import { useToast } from '../components/ui/Toast'
import { PolicyLogTable } from '../components/activity/PolicyLogTable'
import { TaskTable } from '../components/activity/TaskTable'
import { CommandTable } from '../components/activity/CommandTable'
import { CommandSearchResults } from '../components/activity/CommandSearch'
import { SearchInput } from '../components/ui/SearchInput'
import { AppInstallLogTable } from '../components/activity/AppInstallLogTable'
import { RestrictedEventTable } from '../components/activity/RestrictedEventTable'

const tabs = ['policy-logs', 'app-installs', 'restricted-events', 'commands', 'tasks', 'shell-sessions'] as const
type Tab = (typeof tabs)[number]

export function ActivityPage() {
  const { tab } = useParams()
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  let active: Tab = (tabs as readonly string[]).includes(tab ?? '') ? (tab as Tab) : 'policy-logs'
  if (active === 'shell-sessions' && !isAdmin) active = 'policy-logs'
  const items = [
    { key: 'policy-logs' as Tab, label: 'Policy logs' },
    { key: 'app-installs' as Tab, label: 'App installs' },
    { key: 'restricted-events' as Tab, label: 'Restricted software' },
    { key: 'commands' as Tab, label: 'MDM Commands' },
    { key: 'tasks' as Tab, label: 'Agent tasks' },
    ...(isAdmin ? [{ key: 'shell-sessions' as Tab, label: 'Shell sessions' }] : []),
  ]
  return (
    <>
      <PageHeader title="Activity" description="Policy runs, app installs, restricted software events, MDM commands, agent tasks and recorded shell sessions across the fleet. Refreshes automatically." />
      <Tabs className="mb-5" value={active} onChange={(t) => navigate(`/activity/${t}`, { replace: true })} items={items} />
      <Card>
        {active === 'policy-logs' && <PolicyLogsTab />}
        {active === 'app-installs' && <AppInstallsTab />}
        {active === 'restricted-events' && <RestrictedEventsTab />}
        {active === 'commands' && <CommandsTab />}
        {active === 'tasks' && <TasksTab />}
        {active === 'shell-sessions' && <ShellSessionsTab />}
      </Card>
    </>
  )
}

export function ShellSessionsTab({ computerId }: { computerId?: string }) {
  const [page, setPage] = useState({ limit: 50, offset: 0 })
  const q = useShellSessions({ computer_id: computerId, ...page })
  const [open, setOpen] = useState<string | null>(null)
  return (
    <>
      <CardHeader title="Shell sessions" description="Every remote shell session is recorded. Click a row to view the full transcript." />
      {q.error && <p className="px-5 py-4 text-sm text-critical">{errorMessage(q.error)}</p>}
      <Table<ShellSession>
        rows={q.data?.items}
        loading={q.isPending}
        rowKey={(r) => r.id}
        onRowClick={(r) => setOpen(r.id)}
        emptyTitle="No shell sessions recorded"
        emptyDescription="Sessions appear here once an administrator opens a remote shell."
        columns={[
          ...(computerId
            ? []
            : [
                {
                  key: 'computer',
                  header: 'Computer',
                  render: (r: ShellSession) => (
                    <Link to={`/computers/${r.computer_id}`} onClick={(e) => e.stopPropagation()} className="font-medium text-ink hover:underline">
                      {r.computer_name || r.computer_id}
                    </Link>
                  ),
                },
              ]),
          { key: 'actor', header: 'Operator', render: (r) => <span className="font-medium text-ink">{r.actor || 'unknown'}</span> },
          { key: 'ip', header: 'From', render: (r) => <span className="font-mono text-xs text-ink-muted">{r.actor_ip || '—'}</span> },
          { key: 'status', header: 'Status', render: (r) => <StatusBadge status={r.status} /> },
          { key: 'io', header: 'I/O', render: (r) => <span className="tabular-nums text-xs text-ink-muted">{formatBytes(r.bytes_in)} in · {formatBytes(r.bytes_out)} out</span> },
          { key: 'started', header: 'Started', render: (r) => <RelativeTime value={r.started_at} className="text-ink-muted" /> },
          { key: 'ended', header: 'Ended', render: (r) => <RelativeTime value={r.ended_at} emptyLabel="—" className="text-ink-muted" /> },
        ]}
        footer={<Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />}
      />
      <TranscriptModal id={open} onClose={() => setOpen(null)} />
    </>
  )
}

function TranscriptModal({ id, onClose }: { id: string | null; onClose: () => void }) {
  const q = useShellSession(id ?? undefined)
  const d = q.data
  return (
    <Modal open={!!id} onClose={onClose} title="Session transcript" description={id ?? undefined} size="full">
      {q.isPending && <LoadingBlock />}
      {q.error && <p className="text-sm text-critical">{errorMessage(q.error)}</p>}
      {d && (
        <div className="flex flex-col gap-4">
          <DescriptionList
            cols={3}
            items={[
              { label: 'Computer', value: <Link to={`/computers/${d.session.computer_id}`} className="hover:underline">{d.session.computer_name || d.session.computer_id}</Link> },
              { label: 'Operator', value: `${d.session.actor || 'unknown'}${d.session.actor_ip ? ` (${d.session.actor_ip})` : ''}` },
              { label: 'Status', value: <StatusBadge status={d.session.status} /> },
              { label: 'Started', value: absoluteTime(d.session.started_at) },
              { label: 'Connected', value: d.session.connected_at ? absoluteTime(d.session.connected_at) : '—' },
              { label: 'Ended', value: d.session.ended_at ? absoluteTime(d.session.ended_at) : '—' },
              { label: 'Bytes', value: <Badge>{formatBytes(d.session.bytes_in)} in · {formatBytes(d.session.bytes_out)} out</Badge> },
            ]}
          />
          <TranscriptViewer transcript={d.transcript} height="55vh" />
        </div>
      )}
    </Modal>
  )
}

function PolicyLogsTab() {
  const [status, setStatus] = useState('')
  const [page, setPage] = useState({ limit: 50, offset: 0 })
  const q = useAllPolicyLogs({ status: status || undefined, ...page })
  return (
    <>
      <CardHeader
        title="Policy logs"
        description="Expand a row to see per-step output."
        actions={
          <Select
            className="w-40"
            value={status}
            onChange={(e) => {
              setStatus(e.target.value)
              setPage({ ...page, offset: 0 })
            }}
            options={[
              { value: '', label: 'All statuses' },
              { value: 'completed', label: 'Completed' },
              { value: 'failed', label: 'Failed' },
              { value: 'pending', label: 'Pending' },
              { value: 'running', label: 'Running' },
            ]}
          />
        }
      />
      {q.error && <p className="px-5 py-4 text-sm text-critical">{errorMessage(q.error)}</p>}
      <PolicyLogTable logs={q.data?.items} loading={q.isPending} />
      <Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />
    </>
  )
}

export function AppInstallsTab({ computerId }: { computerId?: string }) {
  const [page, setPage] = useState({ limit: 50, offset: 0 })
  const q = useAllAppLogs({ computer_id: computerId, ...page })
  return (
    <>
      <CardHeader title="App installs" description="Installomator runs from the Mac apps catalog. Expand a row for the full output." />
      {q.error && <p className="px-5 py-4 text-sm text-critical">{errorMessage(q.error)}</p>}
      <AppInstallLogTable logs={q.data?.items} loading={q.isPending} showComputer={!computerId} />
      <Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />
    </>
  )
}

export function RestrictedEventsTab({ computerId }: { computerId?: string }) {
  const [page, setPage] = useState({ limit: 50, offset: 0 })
  const q = useRestrictedEvents({ computer_id: computerId, ...page })
  return (
    <>
      <CardHeader title="Restricted software events" description="Every time a restricted process was seen on a scoped computer." />
      <RestrictedEventTable events={q.data?.items} loading={q.isPending} error={q.error ? errorMessage(q.error) : null} showComputer={!computerId} footer={<Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />} />
    </>
  )
}

function CommandsTab() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [status, setStatus] = useState('')
  const [page, setPage] = useState({ limit: 50, offset: 0 })
  const [search, setSearch] = useState('')
  const q = useCommands({ status: status || undefined, ...page })
  const searching = search.trim().length > 0
  const cancel = async (c: MDMCommand) => {
    try {
      await api.del(`/api/v1/commands/${c.command_uuid}`)
      toast.success('Command cancelled')
      await invalidate(['commands'], qk.dashboard)
    } catch (e) {
      toast.error('Cancel failed', errorMessage(e))
    }
  }
  return (
    <>
      <CardHeader
        title="MDM commands"
        description="Click a row to open the inspector: timeline, summaries and (redacted) plists. Pending commands can be cancelled before the device fetches them."
        actions={
          <>
            <SearchInput value={search} onChange={setSearch} placeholder="Search plists…" className="w-56" aria-label="Search commands" />
            <Select
              disabled={searching}
              className="w-44"
              value={status}
              onChange={(e) => {
                setStatus(e.target.value)
                setPage({ ...page, offset: 0 })
              }}
              options={[
                { value: '', label: 'All statuses' },
                { value: 'Pending', label: 'Pending' },
                { value: 'Acknowledged', label: 'Acknowledged' },
                { value: 'Error', label: 'Error' },
                { value: 'NotNow', label: 'Not now' },
                { value: 'CommandFormatError', label: 'Format error' },
              ]}
            />
          </>
        }
      />
      {searching ? (
        <CommandSearchResults query={search} />
      ) : (
        <>
          {q.error && <p className="px-5 py-4 text-sm text-critical">{errorMessage(q.error)}</p>}
          <CommandTable commands={q.data?.items} loading={q.isPending} onCancel={cancel} canCancel={isAdmin} footer={<Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />} />
        </>
      )}
    </>
  )
}

function TasksTab() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [status, setStatus] = useState('')
  const [type, setType] = useState('')
  const [page, setPage] = useState({ limit: 50, offset: 0 })
  const q = useTasks({ status: status || undefined, type: type || undefined, ...page })
  const cancel = async (t: AgentTask) => {
    try {
      await api.post(`/api/v1/tasks/${t.id}/cancel`)
      toast.success('Task cancelled')
      await invalidate(['tasks'], qk.dashboard)
    } catch (e) {
      toast.error('Cancel failed', errorMessage(e))
    }
  }
  return (
    <>
      <CardHeader
        title="Agent tasks"
        description="Expand a row to see output. Queued tasks can be cancelled before the agent picks them up."
        actions={
          <>
            <Select
              className="w-40"
              value={type}
              onChange={(e) => {
                setType(e.target.value)
                setPage({ ...page, offset: 0 })
              }}
              options={[
                { value: '', label: 'All types' },
                { value: 'run_script', label: 'Run script' },
                { value: 'run_command', label: 'Run command' },
                { value: 'run_policy', label: 'Run policy' },
                { value: 'inventory', label: 'Inventory' },
                { value: 'custom_trigger', label: 'Custom trigger' },
                { value: 'notify', label: 'Notification' },
                { value: 'restart', label: 'Restart' },
                { value: 'shutdown', label: 'Shut down' },
                { value: 'logout_user', label: 'Log out user' },
              ]}
            />
            <Select
              className="w-40"
              value={status}
              onChange={(e) => {
                setStatus(e.target.value)
                setPage({ ...page, offset: 0 })
              }}
              options={[
                { value: '', label: 'All statuses' },
                { value: 'queued', label: 'Queued' },
                { value: 'sent', label: 'Sent' },
                { value: 'completed', label: 'Completed' },
                { value: 'failed', label: 'Failed' },
                { value: 'cancelled', label: 'Cancelled' },
              ]}
            />
          </>
        }
      />
      {q.error && <p className="px-5 py-4 text-sm text-critical">{errorMessage(q.error)}</p>}
      <TaskTable tasks={q.data?.items} loading={q.isPending} onCancel={cancel} canCancel={isAdmin} />
      <Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />
    </>
  )
}
