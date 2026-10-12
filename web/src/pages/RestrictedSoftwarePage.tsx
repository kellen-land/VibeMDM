import { useState, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { Ban, Plus, Power, PowerOff, Trash2 } from 'lucide-react'
import { qk, useInvalidate, useRestrictedEvents, useRestrictedSoftware } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { RestrictedSoftware } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardHeader } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { Toggle } from '../components/ui/Checkbox'
import { Tabs } from '../components/ui/Tabs'
import { Pagination } from '../components/ui/Pagination'
import { useToast } from '../components/ui/Toast'
import { ScopeSummary } from '../components/pickers/ScopeEditor'
import { RestrictedEventTable } from '../components/activity/RestrictedEventTable'
import { IndexPage } from '../components/templates/IndexPage'
import { shortcutHint } from '../lib/shortcuts'

export const matchTypeLabels: Record<string, string> = { exact: 'Exact name', contains: 'Contains', regex: 'Regular expression' }

const tabs = ['rules', 'events'] as const
type Tab = (typeof tabs)[number]

const title = 'Restricted software'
const description = 'Block or report processes on scoped computers. The agent watches running processes and can kill them and notify the user.'
const plural = (n: number) => `${n} ${n === 1 ? 'rule' : 'rules'}`

export function RestrictedSoftwarePage() {
  const loc = useLocation()
  const navigate = useNavigate()
  const tab = loc.pathname.split('/')[2] ?? ''
  const active: Tab = (tabs as readonly string[]).includes(tab) ? (tab as Tab) : 'rules'
  const { isAdmin } = useAuth()
  const tabBar = <Tabs className="mb-5" value={active} onChange={(t) => navigate(`/restricted-software${t === 'rules' ? '' : `/${t}`}`, { replace: true })} items={[{ key: 'rules', label: 'Rules' }, { key: 'events', label: 'Events' }]} />
  if (active === 'rules') return <RulesTab tabBar={tabBar} />
  // The events tab is a paginated log, not an object list: same header, its own table.
  return (
    <>
      <PageHeader
        title={title}
        description={description}
        actions={
          isAdmin && (
            <Link to="/restricted-software/new" data-shortcut="new" title={shortcutHint('new', 'New rule')}>
              <Button variant="primary" icon={<Plus />}>
                New rule
              </Button>
            </Link>
          )
        }
      />
      {tabBar}
      <EventsTab />
    </>
  )
}

function RulesTab({ tabBar }: { tabBar: ReactNode }) {
  const { isAdmin } = useAuth()
  const list = useRestrictedSoftware()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [toggling, setToggling] = useState<string | null>(null)

  const setEnabled = (r: RestrictedSoftware, enabled: boolean) => api.put(`/api/v1/restricted-software/${r.id}`, { ...r, enabled })
  const toggle = async (r: RestrictedSoftware, enabled: boolean) => {
    setToggling(r.id)
    try {
      await setEnabled(r, enabled)
      await invalidate(qk.restricted)
      toast.success(enabled ? 'Rule enabled' : 'Rule disabled')
    } catch (e) {
      toast.error('Update failed', errorMessage(e))
    } finally {
      setToggling(null)
    }
  }

  return (
    <IndexPage<RestrictedSoftware>
      title={title}
      description={description}
      newTo="/restricted-software/new"
      newLabel="New rule"
      above={tabBar}
      rows={list.data?.items}
      loading={list.isPending}
      error={list.error}
      rowKey={(r) => r.id}
      rowHref={(r) => `/restricted-software/${r.id}`}
      rowClassName={(r) => (r.enabled ? undefined : 'opacity-70')}
      search={{ placeholder: 'Search rules…', text: (r) => `${r.name} ${r.process_name}` }}
      filters={[
        { key: 'state', label: 'Any state', options: [{ value: 'on', label: 'Enabled' }, { value: 'off', label: 'Disabled' }], match: (r, v) => r.enabled === (v === 'on') },
      ]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.restricted)}
      bulkActions={[
        { label: 'Enable', icon: <Power />, run: (r) => setEnabled(r, true), done: (n) => `${plural(n)} enabled` },
        { label: 'Disable', icon: <PowerOff />, run: (r) => setEnabled(r, false), done: (n) => `${plural(n)} disabled` },
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${plural(n)}?`, description: 'Agents stop enforcing the rules at their next check-in. Recorded events are kept.' },
          run: (r) => api.del(`/api/v1/restricted-software/${r.id}`),
          done: (n) => `${plural(n)} deleted`,
        },
      ]}
      empty={{ title: 'No rules yet', description: 'Create a rule to block a process by name, substring or regular expression.' }}
      columns={[
        {
          key: 'enabled',
          header: 'On',
          width: '60px',
          sortValue: (r) => (r.enabled ? 0 : 1),
          render: (r) => (
            <span onClick={(e) => e.stopPropagation()}>
              <Toggle size="sm" checked={r.enabled} onChange={(v) => toggle(r, v)} disabled={!isAdmin || toggling === r.id} />
            </span>
          ),
        },
        {
          key: 'name',
          header: 'Name',
          sortValue: (r) => r.name,
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
        { key: 'match', header: 'Match', sortValue: (r) => matchTypeLabels[r.match_type] ?? r.match_type, render: (r) => <Badge>{matchTypeLabels[r.match_type] ?? r.match_type}</Badge> },
        {
          key: 'action',
          header: 'Action',
          sortValue: (r) => (r.kill_process ? 0 : 1),
          render: (r) => (
            <span className="flex gap-1">
              {r.kill_process ? <Badge tone="critical">Kill</Badge> : <Badge tone="warning">Report only</Badge>}
              {r.notify_user && <Badge tone="info">Notify</Badge>}
            </span>
          ),
        },
        { key: 'scope', header: 'Scope', render: (r) => <ScopeSummary scope={r.scope} /> },
      ]}
    />
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
