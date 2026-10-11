import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Plus, Trash2, Users } from 'lucide-react'
import { qk, useMobileGroups, useInvalidate } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { ComputerGroup } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card } from '../components/ui/Card'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { SearchInput } from '../components/ui/SearchInput'
import { RelativeTime } from '../components/ui/RelativeTime'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { useToast } from '../components/ui/Toast'

export function MobileGroupsPage() {
  const { isAdmin } = useAuth()
  const groups = useMobileGroups()
  const navigate = useNavigate()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [q, setQ] = useState('')
  const [del, setDel] = useState<ComputerGroup | null>(null)
  const items = (groups.data?.items ?? []).filter((g) => !q || g.name.toLowerCase().includes(q.toLowerCase()) || g.description.toLowerCase().includes(q.toLowerCase()))

  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/mobile-device-groups/${del.id}`)
      toast.success('Group deleted')
      await invalidate(qk.mobileGroups, qk.dashboard)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader
        title="Mobile Device Groups"
        description="Smart groups update automatically from inventory criteria; static groups are curated by hand."
        actions={
          isAdmin && (
            <Link to="/mobile-groups/new">
              <Button variant="primary" icon={<Plus />}>
                New group
              </Button>
            </Link>
          )
        }
      />
      <Card>
        <div className="flex items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={setQ} placeholder="Filter groups…" className="w-full sm:w-72" />
        </div>
        <Table<ComputerGroup>
          rows={items}
          loading={groups.isPending}
          error={groups.error ? errorMessage(groups.error) : null}
          rowKey={(g) => g.id}
          onRowClick={(g) => navigate(`/mobile-groups/${g.id}`)}
          emptyTitle={q ? 'No groups match' : 'No groups yet'}
          emptyDescription={q ? undefined : 'Create a smart group to target iPhones and iPads by criteria, or a static group to pick them by hand.'}
          emptyAction={
            isAdmin && !q ? (
              <Link to="/mobile-groups/new">
                <Button variant="primary" size="sm" icon={<Plus />}>
                  New group
                </Button>
              </Link>
            ) : undefined
          }
          columns={[
            {
              key: 'name',
              header: 'Name',
              render: (g) => (
                <span className="flex items-center gap-2.5">
                  <Users className="size-4 text-ink-faint" />
                  <span>
                    <span className="block font-medium text-ink">{g.name}</span>
                    {g.description && <span className="block text-xs text-ink-muted truncate max-w-md">{g.description}</span>}
                  </span>
                </span>
              ),
            },
            { key: 'type', header: 'Type', render: (g) => <Badge tone={g.is_smart ? 'accent' : 'neutral'}>{g.is_smart ? 'Smart' : 'Static'}</Badge> },
            { key: 'criteria', header: 'Criteria', render: (g) => <span className="text-ink-muted tabular-nums">{g.is_smart ? `${g.criteria?.length ?? 0} rule${(g.criteria?.length ?? 0) === 1 ? '' : 's'}` : '—'}</span> },
            { key: 'members', header: 'Members', align: 'right', render: (g) => <span className="tabular-nums font-medium">{g.member_count.toLocaleString()}</span> },
            { key: 'updated', header: 'Updated', render: (g) => <RelativeTime value={g.updated_at} className="text-ink-muted" /> },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (g) =>
                isAdmin ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<Trash2 />}
                    aria-label="Delete group"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDel(g)
                    }}
                  />
                ) : null,
            },
          ]}
        />
      </Card>
      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={doDelete}
        destructive
        title={`Delete "${del?.name}"?`}
        description="Profiles and apps scoped to this group will no longer target its members."
        confirmLabel="Delete group"
      />
    </>
  )
}
