import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { FileCode2, Plus, ShieldCheck, SlidersHorizontal, Trash2 } from 'lucide-react'
import { qk, useInvalidate, useProfiles } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { ConfigProfileWithStatus } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card } from '../components/ui/Card'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { SearchInput } from '../components/ui/SearchInput'
import { RelativeTime } from '../components/ui/RelativeTime'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { useToast } from '../components/ui/Toast'
import { ScopeSummary } from '../components/pickers/ScopeEditor'

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
  const navigate = useNavigate()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [q, setQ] = useState('')
  const [del, setDel] = useState<ConfigProfileWithStatus | null>(null)
  const items = (profiles.data?.items ?? []).filter((p) => !q || p.name.toLowerCase().includes(q.toLowerCase()) || p.identifier.toLowerCase().includes(q.toLowerCase()) || p.category.toLowerCase().includes(q.toLowerCase()))

  const doDelete = async () => {
    if (!del) return
    try {
      const r = await api.del<{ removed_from: number }>(`/api/v1/profiles/${del.id}`)
      toast.success('Profile deleted', r.removed_from ? `Removal queued on ${r.removed_from} computer${r.removed_from === 1 ? '' : 's'}.` : undefined)
      await invalidate(qk.profiles, qk.dashboard)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader
        title="Configuration Profiles"
        description="Managed settings delivered to scoped computers through MDM."
        actions={
          <>
            <Link to="/profiles/custom-settings">
              <Button icon={<SlidersHorizontal />}>Application &amp; Custom Settings</Button>
            </Link>
            {isAdmin && (
              <Link to="/profiles/new">
                <Button variant="primary" icon={<Plus />}>
                  New profile
                </Button>
              </Link>
            )}
          </>
        }
      />
      <Card>
        <div className="flex items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={setQ} placeholder="Filter profiles…" className="w-full sm:w-72" />
        </div>
        <Table<ConfigProfileWithStatus>
          rows={items}
          loading={profiles.isPending}
          error={profiles.error ? errorMessage(profiles.error) : null}
          rowKey={(p) => p.id}
          onRowClick={(p) => navigate(`/profiles/${p.id}`)}
          emptyTitle={q ? 'No profiles match' : 'No configuration profiles yet'}
          emptyDescription={q ? undefined : 'Upload a .mobileconfig to manage settings on your computers.'}
          emptyAction={
            isAdmin && !q ? (
              <Link to="/profiles/new">
                <Button variant="primary" size="sm" icon={<Plus />}>
                  New profile
                </Button>
              </Link>
            ) : undefined
          }
          columns={[
            {
              key: 'name',
              header: 'Name',
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
            { key: 'category', header: 'Category', render: (p) => (p.category ? <Badge>{p.category}</Badge> : <span className="text-ink-faint">—</span>) },
            { key: 'level', header: 'Level', render: (p) => <span className="text-ink-muted capitalize">{p.level || 'device'}</span> },
            { key: 'scope', header: 'Scope', render: (p) => <ScopeSummary scope={p.scope} /> },
            { key: 'status', header: 'Deployment', render: (p) => <DeploymentCounts status={p.status} /> },
            { key: 'updated', header: 'Updated', render: (p) => <RelativeTime value={p.updated_at} className="text-ink-muted" /> },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (p) =>
                isAdmin ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<Trash2 />}
                    aria-label="Delete profile"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDel(p)
                    }}
                  />
                ) : null,
            },
          ]}
        />
      </Card>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete "${del?.name}"?`} description="The profile is removed from every computer that has it installed, then deleted." confirmLabel="Delete profile" />
    </>
  )
}
