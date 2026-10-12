import { qk, useMobileSearches, useMobileCriteriaFields, useInvalidate } from '../api/queries'
import type { AdvancedSearch } from '../api/types'
import { IndexPage } from '../components/templates/IndexPage'
import { searchBulkActions, searchColumns } from './SearchesPage'

export function MobileSearchesPage() {
  const searches = useMobileSearches()
  const fields = useMobileCriteriaFields()
  const invalidate = useInvalidate()

  return (
    <IndexPage<AdvancedSearch>
      title="Mobile device searches"
      description="Saved searches over iPhones and iPads: reusable inventory criteria with a chosen set of columns, viewable in the console or exported as CSV."
      newTo="/mobile-searches/new"
      newLabel="New search"
      rows={searches.data?.items}
      loading={searches.isPending}
      error={searches.error}
      rowKey={(s) => s.id}
      rowHref={(s) => `/mobile-searches/${s.id}`}
      search={{ placeholder: 'Search saved searches…', text: (s) => s.name }}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.mobileSearches)}
      bulkActions={searchBulkActions('/api/v1/mobile-device-searches', 'devices')}
      empty={{ title: 'No saved searches yet', description: 'Build criteria once, then run or export the results whenever you need them.' }}
      columns={searchColumns(fields.data)}
    />
  )
}
