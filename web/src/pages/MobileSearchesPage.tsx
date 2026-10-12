import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Download, Plus, Search as SearchIcon, Trash2 } from 'lucide-react'
import { qk, useMobileSearches, useMobileCriteriaFields, useInvalidate } from '../api/queries'
import { api, downloadFile, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { AdvancedSearch } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card } from '../components/ui/Card'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { SearchInput } from '../components/ui/SearchInput'
import { RelativeTime } from '../components/ui/RelativeTime'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { useToast } from '../components/ui/Toast'
import { describeCriteria } from '../components/pickers/CriteriaBuilder'

export function MobileSearchesPage() {
  const { isAdmin } = useAuth()
  const searches = useMobileSearches()
  const fields = useMobileCriteriaFields()
  const navigate = useNavigate()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [q, setQ] = useState('')
  const [del, setDel] = useState<AdvancedSearch | null>(null)
  const [exporting, setExporting] = useState<string | null>(null)
  const items = (searches.data?.items ?? []).filter((s) => !q || s.name.toLowerCase().includes(q.toLowerCase()))

  const exportCsv = async (s: AdvancedSearch) => {
    setExporting(s.id)
    try {
      await downloadFile(`/api/v1/mobile-device-searches/${s.id}/export.csv`)
    } catch (e) {
      toast.error('Export failed', errorMessage(e))
    } finally {
      setExporting(null)
    }
  }
  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/mobile-device-searches/${del.id}`)
      toast.success('Search deleted')
      await invalidate(qk.mobileSearches)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader
        title="Mobile Device Searches"
        description="Saved searches over iPhones and iPads: reusable inventory criteria with a chosen set of columns, viewable in the console or exported as CSV."
        actions={
          isAdmin && (
            <Link to="/mobile-searches/new" data-shortcut="new">
              <Button variant="primary" icon={<Plus />}>
                New search
              </Button>
            </Link>
          )
        }
      />
      <Card>
        <div className="flex items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={setQ} placeholder="Filter searches…" className="w-full sm:w-72" />
        </div>
        <Table<AdvancedSearch>
          rows={items}
          loading={searches.isPending}
          error={searches.error ? errorMessage(searches.error) : null}
          rowKey={(s) => s.id}
          onRowClick={(s) => navigate(`/mobile-searches/${s.id}`)}
          emptyTitle={q ? 'No searches match' : 'No saved searches yet'}
          emptyDescription={q ? undefined : 'Build criteria once, then run or export the results whenever you need them.'}
          emptyAction={
            isAdmin && !q ? (
              <Link to="/mobile-searches/new" data-shortcut="new">
                <Button variant="primary" size="sm" icon={<Plus />}>
                  New search
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
                  <SearchIcon className="size-4 text-ink-faint" />
                  <span>
                    <span className="block font-medium text-ink">{s.name}</span>
                    <span className="block text-xs text-ink-muted font-mono truncate max-w-xl">{describeCriteria(s.criteria, fields.data)}</span>
                  </span>
                </span>
              ),
            },
            { key: 'criteria', header: 'Criteria', render: (s) => <span className="text-ink-muted tabular-nums">{s.criteria?.length ?? 0} rule{(s.criteria?.length ?? 0) === 1 ? '' : 's'}</span> },
            { key: 'fields', header: 'Columns', render: (s) => <span className="text-ink-muted tabular-nums">{s.display_fields?.length ? s.display_fields.length : 'Default'}</span> },
            { key: 'updated', header: 'Updated', render: (s) => <RelativeTime value={s.updated_at} className="text-ink-muted" /> },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (s) => (
                <span className="inline-flex gap-1" onClick={(e) => e.stopPropagation()}>
                  <Button size="xs" variant="ghost" icon={<Download />} aria-label="Export CSV" title="Export CSV" loading={exporting === s.id} onClick={() => exportCsv(s)} />
                  {isAdmin && <Button size="xs" variant="ghost" icon={<Trash2 />} aria-label="Delete search" onClick={() => setDel(s)} />}
                </span>
              ),
            },
          ]}
        />
      </Card>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete "${del?.name}"?`} description="Only the saved search is removed; devices are not affected." confirmLabel="Delete search" />
    </>
  )
}
