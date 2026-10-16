import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Power, PowerOff, Trash2 } from 'lucide-react'
import { AppIcon } from '../components/apps/AppIcon'
import { qk, useApps, useInvalidate } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { MacAppWithStatus } from '../api/types'
import { Badge } from '../components/ui/Badge'
import { Toggle } from '../components/ui/Checkbox'
import { useToast } from '../components/ui/Toast'
import { ScopeSummary } from '../components/pickers/ScopeEditor'
import { Tabs } from '../components/ui/Tabs'
import { IndexPage } from '../components/templates/IndexPage'
/** Catalog / Custom labels switch shared by the Mac apps pages. */
export function AppsTabs({ active }: { active: 'catalog' | 'custom-labels' }) {
  const navigate = useNavigate()
  return (
    <Tabs
      className="mb-5"
      value={active}
      onChange={(t) => navigate(t === 'catalog' ? '/apps' : '/apps/custom-labels')}
      items={[
        { key: 'catalog', label: 'Catalog' },
        { key: 'custom-labels', label: 'Custom labels' },
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
  const toast = useToast()
  const invalidate = useInvalidate()
  const [toggling, setToggling] = useState<string | null>(null)
  const rows = apps.data?.items
  const categories = [...new Set((rows ?? []).map((a) => a.category).filter(Boolean))].sort()

  const setEnabled = async (a: MacAppWithStatus, enabled: boolean) => {
    const { status: _s, ...body } = a
    void _s
    await api.put(`/api/v1/apps/${a.id}`, { ...body, enabled })
  }
  const toggle = async (a: MacAppWithStatus, enabled: boolean) => {
    setToggling(a.id)
    try {
      await setEnabled(a, enabled)
      await invalidate(qk.apps)
      toast.success(enabled ? 'App enabled' : 'App disabled')
    } catch (e) {
      toast.error('Update failed', errorMessage(e))
    } finally {
      setToggling(null)
    }
  }
  const nApps = (n: number) => `${n} ${n === 1 ? 'app' : 'apps'}`

  return (
    <IndexPage<MacAppWithStatus>
      title="Mac apps"
      description="Every app VibeMDM installs with Installomator, defined once: install it automatically, offer it in Self Service only, or use it in policies."
      newTo="/apps/new"
      newLabel="New app"
      above={<AppsTabs active="catalog" />}
      rows={rows}
      loading={apps.isPending}
      error={apps.error}
      rowKey={(a) => a.id}
      rowHref={(a) => `/apps/${a.id}`}
      rowClassName={(a) => (a.enabled ? undefined : 'opacity-70')}
      search={{ placeholder: 'Search apps…', text: (a) => `${a.name} ${a.label} ${a.category}` }}
      filters={[
        { key: 'state', label: 'Any state', options: [{ value: 'on', label: 'Enabled' }, { value: 'off', label: 'Disabled' }], match: (a, v) => a.enabled === (v === 'on') },
        { key: 'deployment', label: 'Any deployment', options: [{ value: 'automatic', label: 'Automatic' }, { value: 'self_service', label: 'Self Service only' }], match: (a, v) => (v === 'self_service' ? a.deployment === 'self_service' : a.deployment !== 'self_service') },
        ...(categories.length ? [{ key: 'category', label: 'Any category', options: categories.map((c) => ({ value: c, label: c })), match: (a: MacAppWithStatus, v: string) => a.category === v }] : []),
      ]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.apps)}
      bulkActions={[
        { label: 'Enable', icon: <Power />, run: (a) => setEnabled(a, true), done: (n) => `${nApps(n)} enabled` },
        { label: 'Disable', icon: <PowerOff />, run: (a) => setEnabled(a, false), done: (n) => `${nApps(n)} disabled` },
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${nApps(n)} from the catalog?`, description: 'Computers that already have the apps keep them; they are simply no longer installed or updated by VibeMDM.' },
          run: (a) => api.del(`/api/v1/apps/${a.id}`),
          done: (n) => `${nApps(n)} deleted`,
        },
      ]}
      empty={{ title: 'No apps yet', description: 'Add an app by its Installomator label (e.g. googlechrome, slack, zoom) and scope it to computers.' }}
      footer={<InstallomatorFooter />}
      columns={[
        {
          key: 'enabled',
          header: 'On',
          width: '60px',
          sortValue: (a) => (a.enabled ? 0 : 1),
          render: (a) => (
            <span onClick={(e) => e.stopPropagation()}>
              <Toggle size="sm" checked={a.enabled} onChange={(v) => toggle(a, v)} disabled={!isAdmin || toggling === a.id} />
            </span>
          ),
        },
        {
          key: 'name',
          header: 'App',
          sortValue: (a) => a.name,
          render: (a) => (
            <span className="flex items-center gap-2.5">
              <AppIcon appId={a.id} version={a.icon_version} />
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
        { key: 'update', header: 'Updates', sortValue: (a) => updateBehaviorLabels[a.update_behavior] ?? a.update_behavior, render: (a) => <Badge tone={a.update_behavior === 'keep_updated' ? 'accent' : 'neutral'}>{updateBehaviorLabels[a.update_behavior] ?? a.update_behavior}</Badge> },
        {
          key: 'deployment',
          header: 'Deployment',
          sortValue: (a) => a.deployment,
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
          sortValue: (a) => a.status?.failed ?? 0,
          render: (a) => (
            <span className="flex gap-1.5 text-xs tabular-nums">
              <Badge tone="good" title="Completed">{a.status?.completed ?? 0}</Badge>
              <Badge tone="critical" title="Failed">{a.status?.failed ?? 0}</Badge>
              <Badge tone="info" title="Pending">{(a.status?.pending ?? 0) + (a.status?.running ?? 0)}</Badge>
            </span>
          ),
        },
      ]}
    />
  )
}
