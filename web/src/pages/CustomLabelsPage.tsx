import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Plus, Tag, Trash2 } from 'lucide-react'
import { qk, useCustomLabels, useInvalidate } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { CustomLabel } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card } from '../components/ui/Card'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { SearchInput } from '../components/ui/SearchInput'
import { RelativeTime } from '../components/ui/RelativeTime'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { useToast } from '../components/ui/Toast'
import { AppsTabs, InstallomatorFooter } from './AppsPage'

function host(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}

export function CustomLabelsPage() {
  const { isAdmin } = useAuth()
  const q = useCustomLabels()
  const navigate = useNavigate()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [filter, setFilter] = useState('')
  const [del, setDel] = useState<CustomLabel | null>(null)
  const [deleting, setDeleting] = useState(false)
  const f = filter.trim().toLowerCase()
  const items = (q.data?.items ?? []).filter((l) => !f || l.key.includes(f) || l.name.toLowerCase().includes(f) || l.download_url.toLowerCase().includes(f))

  const doDelete = async () => {
    if (!del) return
    setDeleting(true)
    try {
      await api.del(`/api/v1/custom-labels/${del.id}`)
      toast.success('Custom label deleted')
      await invalidate(qk.customLabels, qk.appLabels)
      setDel(null)
    } catch (e) {
      // 409 {error: "in use by: app X, policy Y"}
      toast.error('Delete failed', errorMessage(e))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <>
      <PageHeader
        title="Mac Apps"
        description="Custom Installomator labels for apps the shipped labels don't cover. Reference one as custom:<key> in the catalog or in a policy."
        actions={
          isAdmin && (
            <Link to="/apps/custom-labels/new" data-shortcut="new">
              <Button variant="primary" icon={<Plus />}>
                New custom label
              </Button>
            </Link>
          )
        }
      />
      <AppsTabs active="custom-labels" />
      <Card>
        <div className="flex items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={filter} onChange={setFilter} placeholder="Filter custom labels…" className="w-full sm:w-72" />
        </div>
        <Table<CustomLabel>
          rows={items}
          loading={q.isPending}
          error={q.error ? errorMessage(q.error) : null}
          rowKey={(l) => l.id}
          onRowClick={(l) => navigate(`/apps/custom-labels/${l.id}`)}
          emptyTitle={f ? 'No custom labels match' : 'No custom labels yet'}
          emptyDescription={f ? undefined : 'Define a download URL, type and Apple Developer Team ID; Installomator verifies the signature before installing.'}
          columns={[
            {
              key: 'label',
              header: 'Label',
              render: (l) => (
                <span className="flex items-center gap-2.5">
                  <Tag className="size-4 text-ink-faint" />
                  <span>
                    <span className="block font-medium text-ink">{l.name}</span>
                    <span className="block font-mono text-xs text-ink-muted">custom:{l.key}</span>
                  </span>
                </span>
              ),
            },
            { key: 'type', header: 'Type', render: (l) => <Badge className="font-mono">{l.type}</Badge> },
            { key: 'team', header: 'Team ID', render: (l) => <span className="font-mono text-xs text-ink">{l.expected_team_id}</span> },
            { key: 'url', header: 'Download from', render: (l) => <span className="font-mono text-xs text-ink-muted" title={l.download_url}>{host(l.download_url)}</span> },
            { key: 'updated', header: 'Updated', render: (l) => <RelativeTime value={l.updated_at} className="text-ink-muted" /> },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (l) =>
                isAdmin ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<Trash2 />}
                    aria-label="Delete custom label"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDel(l)
                    }}
                  />
                ) : null,
            },
          ]}
        />
      </Card>
      <InstallomatorFooter />
      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={doDelete}
        loading={deleting}
        destructive
        title={`Delete custom:${del?.key}?`}
        description="Labels still used by a Mac App or a policy can't be deleted; remove them there first."
        confirmLabel="Delete label"
      />
    </>
  )
}
