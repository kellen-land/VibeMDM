import { useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { Ban, Plus, Trash2 } from 'lucide-react'
import { qk, useInvalidate, useRestrictedEvents, useRestrictedSoftware } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { RestrictedSoftware } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardHeader } from '../components/ui/Card'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { Toggle } from '../components/ui/Checkbox'
import { Tabs } from '../components/ui/Tabs'
import { SearchInput } from '../components/ui/SearchInput'
import { Pagination } from '../components/ui/Pagination'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { useToast } from '../components/ui/Toast'
import { ScopeSummary } from '../components/pickers/ScopeEditor'
import { RestrictedEventTable } from '../components/activity/RestrictedEventTable'

export const matchTypeLabels: Record<string, string> = { exact: 'Exact name', contains: 'Contains', regex: 'Regular expression' }

const tabs = ['rules', 'events'] as const
type Tab = (typeof tabs)[number]

export function RestrictedSoftwarePage() {
  const loc = useLocation()
  const navigate = useNavigate()
  const tab = loc.pathname.split('/')[2] ?? ''
  const active: Tab = (tabs as readonly string[]).includes(tab) ? (tab as Tab) : 'rules'
  const { isAdmin } = useAuth()
  return (
    <>
      <PageHeader
        title="Restricted Software"
        description="Block or report processes on scoped computers. The agent watches running processes and can kill them and notify the user."
        actions={
          isAdmin && (
            <Link to="/restricted-software/new">
              <Button variant="primary" icon={<Plus />}>
                New rule
              </Button>
            </Link>
          )
        }
      />
      <Tabs className="mb-5" value={active} onChange={(t) => navigate(`/restricted-software${t === 'rules' ? '' : `/${t}`}`, { replace: true })} items={[{ key: 'rules', label: 'Rules' }, { key: 'events', label: 'Events' }]} />
      {active === 'rules' ? <RulesTab /> : <EventsTab />}
    </>
  )
}

function RulesTab() {
  const { isAdmin } = useAuth()
  const list = useRestrictedSoftware()
  const navigate = useNavigate()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [q, setQ] = useState('')
  const [del, setDel] = useState<RestrictedSoftware | null>(null)
  const [toggling, setToggling] = useState<string | null>(null)
  const items = (list.data?.items ?? []).filter((r) => !q || r.name.toLowerCase().includes(q.toLowerCase()) || r.process_name.toLowerCase().includes(q.toLowerCase()))

  const toggle = async (r: RestrictedSoftware, enabled: boolean) => {
    setToggling(r.id)
    try {
      await api.put(`/api/v1/restricted-software/${r.id}`, { ...r, enabled })
      await invalidate(qk.restricted)
      toast.success(enabled ? 'Rule enabled' : 'Rule disabled')
    } catch (e) {
      toast.error('Update failed', errorMessage(e))
    } finally {
      setToggling(null)
    }
  }
  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/restricted-software/${del.id}`)
      toast.success('Rule deleted')
      await invalidate(qk.restricted)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <Card>
        <div className="flex items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={setQ} placeholder="Filter rules…" className="w-full sm:w-72" />
        </div>
        <Table<RestrictedSoftware>
          rows={items}
          loading={list.isPending}
          error={list.error ? errorMessage(list.error) : null}
          rowKey={(r) => r.id}
          onRowClick={(r) => navigate(`/restricted-software/${r.id}`)}
          emptyTitle={q ? 'No rules match' : 'No restricted software rules yet'}
          emptyDescription={q ? undefined : 'Create a rule to block a process by name, substring or regular expression.'}
          emptyAction={
            isAdmin && !q ? (
              <Link to="/restricted-software/new">
                <Button variant="primary" size="sm" icon={<Plus />}>
                  New rule
                </Button>
              </Link>
            ) : undefined
          }
          rowClassName={(r) => (r.enabled ? undefined : 'opacity-70')}
          columns={[
            {
              key: 'enabled',
              header: 'On',
              width: '60px',
              render: (r) => (
                <span onClick={(e) => e.stopPropagation()}>
                  <Toggle size="sm" checked={r.enabled} onChange={(v) => toggle(r, v)} disabled={!isAdmin || toggling === r.id} />
                </span>
              ),
            },
            {
              key: 'name',
              header: 'Name',
              render: (r) => (
                <span className="flex items-center gap-2.5">
                  <Ban className="size-4 text-ink-faint" />
                  <span>
                    <span className="block font-medium text-ink">{r.name}</span>
                    <span className="block text-xs text-ink-muted font-mono">{r.process_name}</span>
                  </span>
                </span>
              ),
            },
            { key: 'match', header: 'Match', render: (r) => <Badge>{matchTypeLabels[r.match_type] ?? r.match_type}</Badge> },
            {
              key: 'action',
              header: 'Action',
              render: (r) => (
                <span className="flex gap-1">
                  {r.kill_process ? <Badge tone="critical">Kill</Badge> : <Badge tone="warning">Report only</Badge>}
                  {r.notify_user && <Badge tone="info">Notify</Badge>}
                </span>
              ),
            },
            { key: 'scope', header: 'Scope', render: (r) => <ScopeSummary scope={r.scope} /> },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (r) =>
                isAdmin ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<Trash2 />}
                    aria-label="Delete rule"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDel(r)
                    }}
                  />
                ) : null,
            },
          ]}
        />
      </Card>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete "${del?.name}"?`} description="Agents stop enforcing the rule at their next check-in. Recorded events are kept." confirmLabel="Delete rule" />
    </>
  )
}

function EventsTab() {
  const [page, setPage] = useState({ limit: 50, offset: 0 })
  const q = useRestrictedEvents(page)
  return (
    <Card>
      <CardHeader title="Restricted software events" description="Every time a restricted process was seen on a scoped computer. Refreshes automatically." />
      <RestrictedEventTable events={q.data?.items} loading={q.isPending} error={q.error ? errorMessage(q.error) : null} footer={<Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />} />
    </Card>
  )
}
