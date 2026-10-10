import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Plus, Terminal, Trash2 } from 'lucide-react'
import { qk, useInvalidate, useScripts } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Script } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card } from '../components/ui/Card'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { SearchInput } from '../components/ui/SearchInput'
import { RelativeTime } from '../components/ui/RelativeTime'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { useToast } from '../components/ui/Toast'

export function ScriptsPage() {
  const { isAdmin } = useAuth()
  const scripts = useScripts()
  const navigate = useNavigate()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [q, setQ] = useState('')
  const [del, setDel] = useState<Script | null>(null)
  const items = (scripts.data?.items ?? []).filter((s) => !q || s.name.toLowerCase().includes(q.toLowerCase()) || s.category.toLowerCase().includes(q.toLowerCase()))

  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/scripts/${del.id}`)
      toast.success('Script deleted')
      await invalidate(qk.scripts, qk.dashboard)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader
        title="Scripts"
        description="Shell scripts that run on computers via policies or on demand."
        actions={
          isAdmin && (
            <Link to="/scripts/new">
              <Button variant="primary" icon={<Plus />}>
                New script
              </Button>
            </Link>
          )
        }
      />
      <Card>
        <div className="flex items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={setQ} placeholder="Filter scripts…" className="w-full sm:w-72" />
        </div>
        <Table<Script>
          rows={items}
          loading={scripts.isPending}
          error={scripts.error ? errorMessage(scripts.error) : null}
          rowKey={(s) => s.id}
          onRowClick={(s) => navigate(`/scripts/${s.id}`)}
          emptyTitle={q ? 'No scripts match' : 'No scripts yet'}
          emptyDescription={q ? undefined : 'Add a shell script to run it through policies or directly on computers.'}
          emptyAction={
            isAdmin && !q ? (
              <Link to="/scripts/new">
                <Button variant="primary" size="sm" icon={<Plus />}>
                  New script
                </Button>
              </Link>
            ) : undefined
          }
          columns={[
            {
              key: 'name',
              header: 'Name',
              render: (s) => (
                <span className="flex items-center gap-2.5">
                  <Terminal className="size-4 text-ink-faint" />
                  <span>
                    <span className="block font-medium text-ink">{s.name}</span>
                    {s.info && <span className="block text-xs text-ink-muted truncate max-w-md">{s.info}</span>}
                  </span>
                </span>
              ),
            },
            { key: 'category', header: 'Category', render: (s) => (s.category ? <Badge>{s.category}</Badge> : <span className="text-ink-faint">—</span>) },
            { key: 'priority', header: 'Priority', render: (s) => <span className="text-ink-muted">{s.priority === 'before' ? 'Before packages' : 'After packages'}</span> },
            { key: 'params', header: 'Parameters', render: (s) => <span className="text-ink-muted tabular-nums">{(s.parameter_labels ?? []).filter(Boolean).length} labelled</span> },
            { key: 'updated', header: 'Updated', render: (s) => <RelativeTime value={s.updated_at} className="text-ink-muted" /> },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (s) =>
                isAdmin ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<Trash2 />}
                    aria-label="Delete script"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDel(s)
                    }}
                  />
                ) : null,
            },
          ]}
        />
      </Card>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete "${del?.name}"?`} description="Policies that include this script will no longer run it." confirmLabel="Delete script" />
    </>
  )
}
