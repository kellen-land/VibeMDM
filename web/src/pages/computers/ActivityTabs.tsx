import { useState } from 'react'
import { useComputer, useCommands, useTasks, useComputerTimeline, qk, useInvalidate } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useQuery } from '@tanstack/react-query'
import type { AgentTask, CommandEvent, ListResponse, MDMCommand, PolicyLog } from '../../api/types'
import { useAuth } from '../../auth/AuthContext'
import { CardHeader } from '../../components/ui/Card'
import { Pagination } from '../../components/ui/Pagination'
import { Select } from '../../components/ui/Input'
import { useToast } from '../../components/ui/Toast'
import { PolicyLogTable } from '../../components/activity/PolicyLogTable'
import { TaskTable } from '../../components/activity/TaskTable'
import { CommandTable } from '../../components/activity/CommandTable'
import { CommandSearchResults } from '../../components/activity/CommandSearch'
import { SearchInput } from '../../components/ui/SearchInput'
import { CommandInspectorModal, EventBadge, eventStyle } from '../../components/activity/CommandInspector'
import { LoadingBlock } from '../../components/ui/Spinner'
import { EmptyState } from '../../components/ui/EmptyState'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { absoluteTime, timeWithMs } from '../../lib/format'


export function ComputerPolicyLogsTab({ id }: { id: string }) {
  const [page, setPage] = useState({ limit: 25, offset: 0 })
  const q = useQuery({
    queryKey: qk.computerPolicyLogs(id, page),
    queryFn: () => api.get<ListResponse<PolicyLog>>(`/api/v1/computers/${id}/policy-logs`, { query: page }),
    placeholderData: (p) => p,
    refetchInterval: 15_000,
  })
  return (
    <>
      <CardHeader title="Policy logs" description="Every policy run recorded for this computer." />
      <PolicyLogTable logs={q.data?.items} loading={q.isPending} showComputer={false} />
      <Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />
    </>
  )
}

export function ComputerTasksTab({ id }: { id: string }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [page, setPage] = useState({ limit: 25, offset: 0 })
  const q = useTasks({ computer_id: id, ...page })
  useComputer(id)
  const cancel = async (t: AgentTask) => {
    try {
      await api.post(`/api/v1/tasks/${t.id}/cancel`)
      toast.success('Task cancelled')
      await invalidate(['tasks'], qk.computer(id))
    } catch (e) {
      toast.error('Cancel failed', errorMessage(e))
    }
  }
  return (
    <>
      <CardHeader title="Agent tasks" description="Scripts, commands and policy runs queued for the agent on this computer." />
      <TaskTable tasks={q.data?.items} loading={q.isPending} showComputer={false} onCancel={cancel} canCancel={isAdmin} />
      <Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />
    </>
  )
}

export function ComputerCommandsTab({ id }: { id: string }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [status, setStatus] = useState('')
  const [page, setPage] = useState({ limit: 25, offset: 0 })
  const [search, setSearch] = useState('')
  const q = useCommands({ computer_id: id, status: status || undefined, ...page })
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
        description="Command history from the MDM queue. Click a row to open the inspector."
        actions={
          <>
            <SearchInput value={search} onChange={setSearch} placeholder="Search plists…" className="w-56" aria-label="Search commands" />
            <Select
              disabled={searching}
              value={status}
              onChange={(e) => {
                setStatus(e.target.value)
                setPage({ ...page, offset: 0 })
              }}
              className="w-44"
              aria-label="Filter status"
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
        <CommandSearchResults query={search} computerId={id} />
      ) : (
        <CommandTable
          commands={q.data?.items}
          loading={q.isPending}
          showComputer={false}
          onCancel={cancel}
          canCancel={isAdmin}
          footer={<Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />}
        />
      )}
    </>
  )
}

interface TimelineGroup {
  key: string
  uuid: string
  requestType: string
  events: CommandEvent[]
}

/** Group consecutive events of the same command (the server returns newest first). */
function groupTimeline(items: CommandEvent[]): TimelineGroup[] {
  const out: TimelineGroup[] = []
  for (const e of items) {
    const last = out[out.length - 1]
    if (last && last.uuid === e.command_uuid) last.events.push(e)
    else out.push({ key: `${e.command_uuid}-${e.id}`, uuid: e.command_uuid, requestType: e.request_type, events: [e] })
  }
  return out
}

export function ComputerTimelineTab({ id }: { id: string }) {
  const [page, setPage] = useState({ limit: 100, offset: 0 })
  const [open, setOpen] = useState<string | null>(null)
  const q = useComputerTimeline(id, page)
  const groups = groupTimeline(q.data?.items ?? [])
  return (
    <>
      <CardHeader title="Command timeline" description="Every MDM command event for this Mac, newest first, grouped by command. Click a command to open the inspector. Events are kept for 90 days." />
      {q.error && <p className="px-5 py-4 text-sm text-critical">{errorMessage(q.error)}</p>}
      {q.isPending ? (
        <LoadingBlock />
      ) : groups.length === 0 ? (
        <EmptyState compact title="No command events" description="Events appear as commands are queued, pushed, delivered and answered." />
      ) : (
        <ul className="divide-y divide-line">
          {groups.map((g) => (
            <li key={g.key}>
              <button type="button" onClick={() => setOpen(g.uuid)} className="flex w-full flex-col gap-2 px-4 py-3 text-left hover:bg-line/20 focus-ring">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium text-ink">{g.requestType || 'Command'}</span>
                  <EventBadge event={g.events[0]!.event} />
                  <span className="ml-auto text-xs text-ink-muted">
                    <RelativeTime value={g.events[0]!.at} />
                  </span>
                </div>
                <ol className="flex flex-col gap-1 border-l border-line pl-3">
                  {g.events.map((e) => (
                    <li key={e.id} className="flex flex-wrap items-baseline gap-x-2 text-xs">
                      <time dateTime={e.at} title={absoluteTime(e.at)} className="font-mono tabular-nums text-ink-faint">
                        {timeWithMs(e.at)}
                      </time>
                      <span className="font-medium text-ink">{eventStyle(e.event).label}</span>
                      <span className="text-ink-muted">{e.description}</span>
                    </li>
                  ))}
                </ol>
              </button>
            </li>
          ))}
        </ul>
      )}
      <Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />
      <CommandInspectorModal uuid={open} onClose={() => setOpen(null)} />
    </>
  )
}
