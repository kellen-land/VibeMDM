import { useState } from 'react'
import { useComputer, useCommands, useTasks, qk, useInvalidate } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useQuery } from '@tanstack/react-query'
import type { AgentTask, ListResponse, MDMCommand, PolicyLog } from '../../api/types'
import { useAuth } from '../../auth/AuthContext'
import { CardHeader } from '../../components/ui/Card'
import { Pagination } from '../../components/ui/Pagination'
import { Select } from '../../components/ui/Input'
import { useToast } from '../../components/ui/Toast'
import { PolicyLogTable } from '../../components/activity/PolicyLogTable'
import { TaskTable } from '../../components/activity/TaskTable'
import { CommandTable } from '../../components/activity/CommandTable'


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
  const q = useCommands({ computer_id: id, status: status || undefined, ...page })
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
        description="Command history from the MDM queue. Click a row to see the raw plist and result."
        actions={
          <Select
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
        }
      />
      <CommandTable
        commands={q.data?.items}
        loading={q.isPending}
        showComputer={false}
        onCancel={cancel}
        canCancel={isAdmin}
        footer={<Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />}
      />
    </>
  )
}
