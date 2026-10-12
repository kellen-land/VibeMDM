import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AppWindow, Plus, Trash2 } from 'lucide-react'
import { qk, useApps, useInvalidate } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { MacAppWithStatus } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card } from '../components/ui/Card'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { Toggle } from '../components/ui/Checkbox'
import { SearchInput } from '../components/ui/SearchInput'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { useToast } from '../components/ui/Toast'
import { ScopeSummary } from '../components/pickers/ScopeEditor'
import { Tabs } from '../components/ui/Tabs'

/** Catalog / Custom Labels switch shared by the Mac Apps pages. */
export function AppsTabs({ active }: { active: 'catalog' | 'custom-labels' }) {
  const navigate = useNavigate()
  return (
    <Tabs
      className="mb-5"
      value={active}
      onChange={(t) => navigate(t === 'catalog' ? '/apps' : '/apps/custom-labels')}
      items={[
        { key: 'catalog', label: 'Catalog' },
        { key: 'custom-labels', label: 'Custom Labels' },
      ]}
    />
  )
}

export const updateBehaviorLabels: Record<string, string> = { install_once: 'Install once', keep_updated: 'Keep up to date' }

export function InstallomatorFooter() {
  return (
    <p className="mt-4 text-xs text-ink-faint">
      Powered by{' '}
      <a href="/agent/installomator-license" target="_blank" rel="noreferrer" className="underline hover:text-ink">
        Installomator (Apache-2.0)
      </a>
      . Labels are resolved on the Mac at install time; the download comes directly from the vendor.
    </p>
  )
}

export function AppsPage() {
  const { isAdmin } = useAuth()
  const apps = useApps()
  const navigate = useNavigate()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [q, setQ] = useState('')
  const [del, setDel] = useState<MacAppWithStatus | null>(null)
  const [toggling, setToggling] = useState<string | null>(null)
  const items = (apps.data?.items ?? []).filter((a) => !q || a.name.toLowerCase().includes(q.toLowerCase()) || a.label.toLowerCase().includes(q.toLowerCase()) || a.category.toLowerCase().includes(q.toLowerCase()))

  const toggle = async (a: MacAppWithStatus, enabled: boolean) => {
    setToggling(a.id)
    try {
      const { status: _s, ...body } = a
      void _s
      await api.put(`/api/v1/apps/${a.id}`, { ...body, enabled })
      await invalidate(qk.apps)
      toast.success(enabled ? 'App enabled' : 'App disabled')
    } catch (e) {
      toast.error('Update failed', errorMessage(e))
    } finally {
      setToggling(null)
    }
  }
  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/apps/${del.id}`)
      toast.success('App removed from catalog')
      await invalidate(qk.apps)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader
        title="Mac Apps"
        description="Every app VibeMDM installs with Installomator, defined once: install it automatically, offer it in Self Service only, or use it in policies."
        actions={
          isAdmin && (
            <Link to="/apps/new">
              <Button variant="primary" icon={<Plus />}>
                Add app
              </Button>
            </Link>
          )
        }
      />
      <AppsTabs active="catalog" />
      <Card>
        <div className="flex items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={setQ} placeholder="Filter apps…" className="w-full sm:w-72" />
        </div>
        <Table<MacAppWithStatus>
          rows={items}
          loading={apps.isPending}
          error={apps.error ? errorMessage(apps.error) : null}
          rowKey={(a) => a.id}
          onRowClick={(a) => navigate(`/apps/${a.id}`)}
          emptyTitle={q ? 'No apps match' : 'No apps in the catalog yet'}
          emptyDescription={q ? undefined : 'Add an app by its Installomator label (e.g. googlechrome, slack, zoom) and scope it to computers.'}
          emptyAction={
            isAdmin && !q ? (
              <Link to="/apps/new">
                <Button variant="primary" size="sm" icon={<Plus />}>
                  Add app
                </Button>
              </Link>
            ) : undefined
          }
          rowClassName={(a) => (a.enabled ? undefined : 'opacity-70')}
          columns={[
            {
              key: 'enabled',
              header: 'On',
              width: '60px',
              render: (a) => (
                <span onClick={(e) => e.stopPropagation()}>
                  <Toggle size="sm" checked={a.enabled} onChange={(v) => toggle(a, v)} disabled={!isAdmin || toggling === a.id} />
                </span>
              ),
            },
            {
              key: 'name',
              header: 'App',
              render: (a) => (
                <span className="flex items-center gap-2.5">
                  <AppWindow className="size-4 text-ink-faint" />
                  <span>
                    <span className="block font-medium text-ink">{a.name}</span>
                    <span className="block text-xs text-ink-muted">
                      <span className="font-mono">{a.label}</span>
                      {a.category && ` · ${a.category}`}
                    </span>
                  </span>
                </span>
              ),
            },
            { key: 'update', header: 'Updates', render: (a) => <Badge tone={a.update_behavior === 'keep_updated' ? 'accent' : 'neutral'}>{updateBehaviorLabels[a.update_behavior] ?? a.update_behavior}</Badge> },
            {
              key: 'deployment',
              header: 'Deployment',
              render: (a) =>
                a.deployment === 'self_service' ? (
                  <Badge tone="info">Self Service only</Badge>
                ) : (
                  <span className="flex items-center gap-1.5">
                    <Badge>Automatic</Badge>
                    {a.self_service?.enabled && <span className="text-xs text-ink-muted">+ Self Service</span>}
                  </span>
                ),
            },
            { key: 'scope', header: 'Scope', render: (a) => <ScopeSummary scope={a.scope} /> },
            {
              key: 'status',
              header: 'Installs',
              render: (a) => (
                <span className="flex gap-1.5 text-xs tabular-nums">
                  <Badge tone="good" title="Completed">{a.status?.completed ?? 0}</Badge>
                  <Badge tone="critical" title="Failed">{a.status?.failed ?? 0}</Badge>
                  <Badge tone="info" title="Pending">{(a.status?.pending ?? 0) + (a.status?.running ?? 0)}</Badge>
                </span>
              ),
            },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (a) =>
                isAdmin ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<Trash2 />}
                    aria-label="Delete app"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDel(a)
                    }}
                  />
                ) : null,
            },
          ]}
        />
      </Card>
      <InstallomatorFooter />
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Remove "${del?.name}" from the catalog?`} description="Computers that already have the app keep it; it is simply no longer installed or updated by VibeMDM." confirmLabel="Remove app" />
    </>
  )
}
