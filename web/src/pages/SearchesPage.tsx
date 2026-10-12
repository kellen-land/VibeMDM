import { Download, Search as SearchIcon, Trash2 } from 'lucide-react'
import { qk, useAdvancedSearches, useCriteriaFields, useInvalidate } from '../api/queries'
import { api, downloadFile } from '../api/client'
import type { AdvancedSearch, CriterionField } from '../api/types'
import { RelativeTime } from '../components/ui/RelativeTime'
import { IndexPage, type BulkAction, type IndexColumn } from '../components/templates/IndexPage'
import { describeCriteria } from '../components/pickers/CriteriaBuilder'

const ruleCount = (s: AdvancedSearch) => s.criteria?.length ?? 0

/** Columns shared by the computer and mobile device search lists. */
export function searchColumns(fields: CriterionField[] | undefined): IndexColumn<AdvancedSearch>[] {
  return [
    {
      key: 'name',
      header: 'Name',
      sortValue: (s) => s.name,
      render: (s) => (
        <span className="flex items-center gap-2.5">
          <SearchIcon className="size-4 text-ink-faint" />
          <span>
            <span className="block font-medium text-ink">{s.name}</span>
            <span className="block text-xs text-ink-muted font-mono truncate max-w-xl">{describeCriteria(s.criteria, fields)}</span>
          </span>
        </span>
      ),
    },
    { key: 'criteria', header: 'Criteria', sortValue: ruleCount, render: (s) => <span className="text-ink-muted tabular-nums">{ruleCount(s)} rule{ruleCount(s) === 1 ? '' : 's'}</span> },
    { key: 'fields', header: 'Columns', sortValue: (s) => s.display_fields?.length ?? 0, render: (s) => <span className="text-ink-muted tabular-nums">{s.display_fields?.length ? s.display_fields.length : 'Default'}</span> },
    { key: 'updated', header: 'Updated', sortValue: (s) => s.updated_at, render: (s) => <RelativeTime value={s.updated_at} className="text-ink-muted" /> },
  ]
}

/** Export CSV (viewers too) and Delete, for the selected searches. */
export function searchBulkActions(base: string, devices: string): BulkAction<AdvancedSearch>[] {
  return [
    {
      label: 'Export CSV',
      icon: <Download />,
      adminOnly: false,
      run: (s) => downloadFile(`${base}/${s.id}/export.csv`),
      done: (n) => `${n} ${n === 1 ? 'export' : 'exports'} downloaded`,
    },
    {
      label: 'Delete',
      icon: <Trash2 />,
      destructive: true,
      confirm: { title: (n) => `Delete ${n} ${n === 1 ? 'search' : 'searches'}?`, description: `Only the saved searches are removed; ${devices} are not affected.` },
      run: (s) => api.del(`${base}/${s.id}`),
      done: (n) => `${n} ${n === 1 ? 'search' : 'searches'} deleted`,
    },
  ]
}

export function SearchesPage() {
  const searches = useAdvancedSearches()
  const fields = useCriteriaFields()
  const invalidate = useInvalidate()

  return (
    <IndexPage<AdvancedSearch>
      title="Searches"
      description="Saved advanced searches: reusable inventory criteria with a chosen set of columns, viewable in the console or exported as CSV."
      newTo="/searches/new"
      newLabel="New search"
      rows={searches.data?.items}
      loading={searches.isPending}
      error={searches.error}
      rowKey={(s) => s.id}
      rowHref={(s) => `/searches/${s.id}`}
      search={{ placeholder: 'Search saved searches…', text: (s) => s.name }}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.searches)}
      bulkActions={searchBulkActions('/api/v1/advanced-searches', 'computers')}
      empty={{ title: 'No saved searches yet', description: 'Build criteria once, then run or export the results whenever you need them.' }}
      columns={searchColumns(fields.data)}
    />
  )
}
