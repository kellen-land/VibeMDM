import { Link } from 'react-router-dom'
import { FileCode2, ShieldCheck, SlidersHorizontal, Trash2, Upload } from 'lucide-react'
import { qk, useInvalidate, useProfiles } from '../api/queries'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { ConfigProfileWithStatus } from '../api/types'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'
import { ScopeSummary } from '../components/pickers/ScopeEditor'
import { IndexPage } from '../components/templates/IndexPage'

export function DeploymentCounts({ status }: { status: Record<string, number> }) {
  const items: { k: string; tone: 'good' | 'info' | 'critical' | 'neutral' | 'warning'; label: string }[] = [
    { k: 'installed', tone: 'good', label: 'Installed' },
    { k: 'pending', tone: 'info', label: 'Pending' },
    { k: 'failed', tone: 'critical', label: 'Failed' },
    { k: 'removing', tone: 'warning', label: 'Removing' },
    { k: 'removed', tone: 'neutral', label: 'Removed' },
  ]
  const shown = items.filter((i) => (status[i.k] ?? 0) > 0)
  if (shown.length === 0) return <span className="text-xs text-ink-faint">Not deployed</span>
  return (
    <span className="flex flex-wrap gap-1 tabular-nums">
      {shown.map((i) => (
        <Badge key={i.k} tone={i.tone} title={i.label}>
          {status[i.k]} {i.label.toLowerCase()}
        </Badge>
      ))}
    </span>
  )
}

export function ProfilesPage() {
  const { isAdmin } = useAuth()
  const profiles = useProfiles()
  const invalidate = useInvalidate()
  const rows = profiles.data?.items
  const categories = [...new Set((rows ?? []).map((p) => p.category).filter(Boolean))].sort()
  const plural = (n: number) => `${n} ${n === 1 ? 'profile' : 'profiles'}`

  return (
    <IndexPage<ConfigProfileWithStatus>
      title="Configuration profiles"
      description="Managed settings delivered to scoped computers through MDM."
      newTo="/profiles/new"
      newLabel="New profile"
      secondaryActions={
        <>
          <Link to="/profiles/custom-settings">
            <Button icon={<SlidersHorizontal />}>Application &amp; custom settings</Button>
          </Link>
          {isAdmin && (
            <Link to="/profiles/new?upload=1">
              <Button variant="ghost" icon={<Upload />}>
                Upload
              </Button>
            </Link>
          )}
        </>
      }
      rows={rows}
      loading={profiles.isPending}
      error={profiles.error}
      rowKey={(p) => p.id}
      rowHref={(p) => `/profiles/${p.id}`}
      search={{ placeholder: 'Search profiles…', text: (p) => `${p.name} ${p.identifier} ${p.category}` }}
      filters={[
        ...(categories.length ? [{ key: 'category', label: 'Any category', options: categories.map((c) => ({ value: c, label: c })), match: (p: ConfigProfileWithStatus, v: string) => p.category === v }] : []),
        { key: 'level', label: 'Any level', options: [{ value: 'device', label: 'Device' }, { value: 'user', label: 'User' }], match: (p, v) => (p.level || 'device') === v },
        { key: 'failed', label: 'Any status', options: [{ value: 'failed', label: 'With failures' }], match: (p) => (p.status.failed ?? 0) > 0 },
      ]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.profiles, qk.dashboard)}
      bulkActions={[
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${plural(n)}?`, description: 'Each profile is removed from every computer that has it installed, then deleted.' },
          run: (p) => api.del(`/api/v1/profiles/${p.id}`),
          done: (n) => `${plural(n)} deleted`,
        },
      ]}
      empty={{ title: 'No profiles yet', description: 'Profiles deliver managed settings such as Wi-Fi, restrictions or privacy permissions to your computers.' }}
      columns={[
        {
          key: 'name',
          header: 'Name',
          sortValue: (p) => p.name,
          render: (p) => (
            <span className="flex items-center gap-2.5">
              <ShieldCheck className="size-4 text-ink-faint" />
              <span className="min-w-0">
                <span className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-ink">{p.name}</span>
                  {p.schema_source && (
                    <Badge icon={<FileCode2 />} title="Built from a JSON schema form">
                      Form
                    </Badge>
                  )}
                </span>
                <span className="block text-xs text-ink-muted font-mono truncate max-w-xs">{p.identifier}</span>
              </span>
            </span>
          ),
        },
        { key: 'category', header: 'Category', sortValue: (p) => p.category, render: (p) => (p.category ? <Badge>{p.category}</Badge> : <span className="text-ink-faint">—</span>) },
        { key: 'level', header: 'Level', sortValue: (p) => p.level || 'device', render: (p) => <span className="text-ink-muted capitalize">{p.level || 'device'}</span> },
        { key: 'scope', header: 'Scope', render: (p) => <ScopeSummary scope={p.scope} /> },
        { key: 'status', header: 'Deployment', sortValue: (p) => p.status.failed ?? 0, render: (p) => <DeploymentCounts status={p.status} /> },
        { key: 'updated', header: 'Updated', sortValue: (p) => p.updated_at, render: (p) => <RelativeTime value={p.updated_at} className="text-ink-muted" /> },
      ]}
    />
  )
}
