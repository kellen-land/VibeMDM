import { useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Download, Laptop } from 'lucide-react'
import { useComputers, useGroups } from '../../api/queries'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card } from '../../components/ui/Card'
import { Table, type SortState } from '../../components/ui/Table'
import { SearchInput } from '../../components/ui/SearchInput'
import { Select } from '../../components/ui/Input'
import { Pagination } from '../../components/ui/Pagination'
import { Badge } from '../../components/ui/Badge'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useDebounced } from '../../lib/hooks'
import { buildQuery, downloadFile, errorMessage } from '../../api/client'
import { Button } from '../../components/ui/Button'
import { useToast } from '../../components/ui/Toast'
import type { Computer } from '../../api/types'
import { latest } from '../DashboardPage'

export function ComputerStatusBadges({ c, compact }: { c: Pick<Computer, 'mdm_enrolled' | 'agent_enrolled'>; compact?: boolean }) {
  return (
    <span className="inline-flex gap-1">
      <Badge tone={c.mdm_enrolled ? 'good' : 'neutral'} dot title={c.mdm_enrolled ? 'MDM enrolled' : 'Not MDM enrolled'}>
        {compact ? 'MDM' : c.mdm_enrolled ? 'MDM' : 'No MDM'}
      </Badge>
      <Badge tone={c.agent_enrolled ? 'good' : 'neutral'} dot title={c.agent_enrolled ? 'Agent installed' : 'Agent not installed'}>
        {compact ? 'Agent' : c.agent_enrolled ? 'Agent' : 'No agent'}
      </Badge>
    </span>
  )
}

/** Columns included in the Computers list CSV export (keys from GET /advanced-searches/fields). */
const exportFields = ['name', 'serial_number', 'model_name', 'os_version', 'last_user', 'building', 'department', 'ip_address', 'mdm_enrolled', 'agent_enrolled', 'filevault_enabled', 'last_checkin']

export function ComputersPage() {
  const [sp, setSp] = useSearchParams()
  const navigate = useNavigate()
  const q = sp.get('q') ?? ''
  const groupId = sp.get('group_id') ?? ''
  const sort = sp.get('sort') ?? 'name'
  const order = (sp.get('order') ?? 'asc') as 'asc' | 'desc'
  const limit = Number(sp.get('limit') ?? 50)
  const offset = Number(sp.get('offset') ?? 0)
  const dq = useDebounced(q, 300)

  const set = (patch: Record<string, string | number | undefined>) => {
    const next = new URLSearchParams(sp)
    for (const [k, v] of Object.entries(patch)) {
      if (v === undefined || v === '' || v === null) next.delete(k)
      else next.set(k, String(v))
    }
    setSp(next, { replace: true })
  }

  const params = useMemo(() => ({ q: dq, group_id: groupId, sort, order, limit, offset }), [dq, groupId, sort, order, limit, offset])
  const list = useComputers(params)
  const groups = useGroups()
  const toast = useToast()
  const [exporting, setExporting] = useState(false)
  const exportCsv = async () => {
    setExporting(true)
    try {
      await downloadFile(`/api/v1/computers-export.csv${buildQuery({ q: dq, group_id: groupId, sort, order, fields: exportFields })}`)
    } catch (e) {
      toast.error('Export failed', errorMessage(e))
    } finally {
      setExporting(false)
    }
  }

  const sortState: SortState = { key: sort, dir: order }

  return (
    <>
      <PageHeader
        title="Computers"
        description={list.data ? `${list.data.total.toLocaleString()} enrolled ${list.data.total === 1 ? 'computer' : 'computers'}` : undefined}
        actions={
          <Button icon={<Download />} loading={exporting} onClick={exportCsv} title="Download the filtered list as CSV">
            Export CSV
          </Button>
        }
      />
      <Card>
        <div className="flex flex-wrap items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={(v) => set({ q: v, offset: 0 })} placeholder="Search name, serial, user, model, IP…" className="w-full sm:w-80" />
          <Select value={groupId} onChange={(e) => set({ group_id: e.target.value, offset: 0 })} className="w-full sm:w-64" aria-label="Filter by group">
            <option value="">All groups</option>
            {(groups.data?.items ?? []).map((g) => (
              <option key={g.id} value={g.id}>
                {g.name} ({g.member_count})
              </option>
            ))}
          </Select>
        </div>
        <Table<Computer>
          rows={list.data?.items}
          loading={list.isFetching}
          error={list.error ? errorMessage(list.error) : null}
          rowKey={(c) => c.id}
          onRowClick={(c) => navigate(`/computers/${c.id}`)}
          sort={sortState}
          onSort={(s) => set({ sort: s.key, order: s.dir, offset: 0 })}
          emptyTitle={q || groupId ? 'No computers match your filters' : 'No computers enrolled yet'}
          emptyDescription={q || groupId ? 'Try a different search or group.' : 'Enroll a Mac from the Enrollment page to get started.'}
          columns={[
            {
              key: 'name',
              header: 'Name',
              sortable: true,
              render: (c) => (
                <span className="flex items-center gap-2.5">
                  <Laptop className="size-4 shrink-0 text-ink-faint" />
                  <span className="min-w-0">
                    <span className="block font-medium text-ink truncate max-w-[240px]">{c.name || c.hostname || c.serial_number || 'Unnamed'}</span>
                    {c.hostname && c.hostname !== c.name && <span className="block text-xs text-ink-muted truncate max-w-[240px]">{c.hostname}</span>}
                  </span>
                </span>
              ),
            },
            { key: 'serial_number', header: 'Serial', sortable: true, render: (c) => <span className="font-mono text-xs">{c.serial_number || '—'}</span> },
            { key: 'model_name', header: 'Model', sortable: true, render: (c) => <span className="text-ink-muted">{c.model_name || c.model || '—'}</span> },
            { key: 'os_version', header: 'macOS', sortable: true, render: (c) => <span className="tabular-nums">{c.os_version || '—'}</span> },
            { key: 'last_user', header: 'Last user', sortable: true, render: (c) => <span className="text-ink-muted">{c.last_user || '—'}</span> },
            { key: 'last_checkin', header: 'Last check-in', sortable: true, render: (c) => <RelativeTime value={latest(c.agent_last_checkin, c.mdm_last_checkin)} className="text-ink-muted" /> },
            { key: 'status', header: 'Status', render: (c) => <ComputerStatusBadges c={c} /> },
          ]}
          footer={<Pagination total={list.data?.total ?? 0} limit={limit} offset={offset} onChange={(n) => set({ limit: n.limit, offset: n.offset })} />}
        />
      </Card>
    </>
  )
}
