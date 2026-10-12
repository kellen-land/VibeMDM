import { Trash2 } from 'lucide-react'
import { qk, useMobileGroups, useInvalidate } from '../api/queries'
import { api } from '../api/client'
import type { ComputerGroup } from '../api/types'
import { IndexPage } from '../components/templates/IndexPage'
import { groupColumns, groupTypeFilter } from './GroupsPage'

export function MobileGroupsPage() {
  const groups = useMobileGroups()
  const invalidate = useInvalidate()

  return (
    <IndexPage<ComputerGroup>
      title="Mobile device groups"
      description="Smart groups update automatically from inventory criteria; static groups are curated by hand."
      newTo="/mobile-groups/new"
      newLabel="New group"
      rows={groups.data?.items}
      loading={groups.isPending}
      error={groups.error}
      rowKey={(g) => g.id}
      rowHref={(g) => `/mobile-groups/${g.id}`}
      search={{ placeholder: 'Search groups…', text: (g) => `${g.name} ${g.description}` }}
      filters={[groupTypeFilter]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.mobileGroups, qk.dashboard)}
      bulkActions={[
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${n} ${n === 1 ? 'group' : 'groups'}?`, description: 'Profiles and apps scoped to these groups will no longer target their members.' },
          run: (g) => api.del(`/api/v1/mobile-device-groups/${g.id}`),
          done: (n) => `${n} ${n === 1 ? 'group' : 'groups'} deleted`,
        },
      ]}
      empty={{ title: 'No groups yet', description: 'Create a smart group to target iPhones and iPads by criteria, or a static group to pick them by hand.' }}
      columns={groupColumns}
    />
  )
}
