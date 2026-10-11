import { useMemo } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Smartphone, Tablet } from 'lucide-react'
import { useMobileDevices, useMobileGroups } from '../../api/queries'
import { errorMessage } from '../../api/client'
import type { MobileDevice } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card } from '../../components/ui/Card'
import { Table, type SortState } from '../../components/ui/Table'
import { SearchInput } from '../../components/ui/SearchInput'
import { Select } from '../../components/ui/Input'
import { Pagination } from '../../components/ui/Pagination'
import { Badge } from '../../components/ui/Badge'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useDebounced } from '../../lib/hooks'

export function DeviceIcon({ d, className = 'size-4 shrink-0 text-ink-faint' }: { d: Pick<MobileDevice, 'device_type'>; className?: string }) {
  return d.device_type === 'iPad' ? <Tablet className={className} /> : <Smartphone className={className} />
}

export function MobileStatusBadges({ d }: { d: Pick<MobileDevice, 'mdm_enrolled' | 'is_supervised'> }) {
  return (
    <span className="inline-flex gap-1">
      <Badge tone={d.mdm_enrolled ? 'good' : 'neutral'} dot>
        {d.mdm_enrolled ? 'Managed' : 'Not managed'}
      </Badge>
      {d.is_supervised && <Badge tone="accent">Supervised</Badge>}
    </span>
  )
}

export function MobileDevicesPage() {
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
      if (v === undefined || v === '') next.delete(k)
      else next.set(k, String(v))
    }
    setSp(next, { replace: true })
  }
  const params = useMemo(() => ({ q: dq, group_id: groupId, sort, order, limit, offset }), [dq, groupId, sort, order, limit, offset])
  const list = useMobileDevices(params)
  const groups = useMobileGroups()
  const sortState: SortState = { key: sort, dir: order }

  return (
    <>
      <PageHeader title="Mobile Devices" description={list.data ? `${list.data.total.toLocaleString()} iPhone and iPad ${list.data.total === 1 ? 'record' : 'records'}` : undefined} />
      <Card>
        <div className="flex flex-wrap items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={(v) => set({ q: v, offset: 0 })} placeholder="Search name, serial, model, user, phone…" className="w-full sm:w-80" />
          <Select value={groupId} onChange={(e) => set({ group_id: e.target.value, offset: 0 })} className="w-full sm:w-64" aria-label="Filter by group">
            <option value="">All groups</option>
            {(groups.data?.items ?? []).map((g) => (
              <option key={g.id} value={g.id}>
                {g.name} ({g.member_count})
              </option>
            ))}
          </Select>
        </div>
        <Table<MobileDevice>
          rows={list.data?.items}
          loading={list.isFetching}
          error={list.error ? errorMessage(list.error) : null}
          rowKey={(d) => d.id}
          onRowClick={(d) => navigate(`/mobile-devices/${d.id}`)}
          sort={sortState}
          onSort={(s) => set({ sort: s.key, order: s.dir, offset: 0 })}
          emptyTitle={q || groupId ? 'No mobile devices match your filters' : 'No iPhones or iPads enrolled yet'}
          emptyDescription={q || groupId ? 'Try a different search or group.' : 'Enroll one from the Mobile Device Enrollment page.'}
          columns={[
            {
              key: 'name',
              header: 'Name',
              sortable: true,
              render: (d) => (
                <span className="flex items-center gap-2.5">
                  <DeviceIcon d={d} />
                  <span className="block max-w-[240px] truncate font-medium text-ink">{d.name || d.serial_number || 'Unnamed'}</span>
                </span>
              ),
            },
            { key: 'model_name', header: 'Model', sortable: true, render: (d) => <span className="text-ink-muted">{d.model_name || d.model || '—'}</span> },
            { key: 'os_version', header: 'OS', sortable: true, render: (d) => <span className="tabular-nums">{d.os_version || '—'}</span> },
            { key: 'serial_number', header: 'Serial', sortable: true, render: (d) => <span className="font-mono text-xs">{d.serial_number || '—'}</span> },
            { key: 'assigned_user', header: 'User', sortable: true, render: (d) => <span className="text-ink-muted">{d.assigned_user || '—'}</span> },
            { key: 'last_checkin', header: 'Last check-in', sortable: true, render: (d) => <RelativeTime value={d.mdm_last_checkin} className="text-ink-muted" /> },
            { key: 'status', header: 'Status', render: (d) => <MobileStatusBadges d={d} /> },
          ]}
          footer={<Pagination total={list.data?.total ?? 0} limit={limit} offset={offset} onChange={(n) => set({ limit: n.limit, offset: n.offset })} />}
        />
      </Card>
    </>
  )
}
