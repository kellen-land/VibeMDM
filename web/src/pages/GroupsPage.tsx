import { Trash2, Users } from 'lucide-react'
import { qk, useGroups, useInvalidate } from '../api/queries'
import { api } from '../api/client'
import type { ComputerGroup } from '../api/types'
import { Badge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'
import { IndexPage, type IndexColumn, type IndexFilter } from '../components/templates/IndexPage'

/** Columns shared by the computer and mobile device group lists. */
export const groupColumns: IndexColumn<ComputerGroup>[] = [
  {
    key: 'name',
    header: 'Name',
    sortValue: (g) => g.name,
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
  { key: 'type', header: 'Type', sortValue: (g) => (g.is_smart ? 0 : 1), render: (g) => <Badge tone={g.is_smart ? 'accent' : 'neutral'}>{g.is_smart ? 'Smart' : 'Static'}</Badge> },
  {
    key: 'criteria',
    header: 'Criteria',
    sortValue: (g) => (g.is_smart ? g.criteria?.length ?? 0 : -1),
    render: (g) => <span className="text-ink-muted tabular-nums">{g.is_smart ? `${g.criteria?.length ?? 0} rule${(g.criteria?.length ?? 0) === 1 ? '' : 's'}` : '—'}</span>,
  },
  { key: 'members', header: 'Members', align: 'right', sortValue: (g) => g.member_count, render: (g) => <span className="tabular-nums font-medium">{g.member_count.toLocaleString()}</span> },
  { key: 'updated', header: 'Updated', sortValue: (g) => g.updated_at, render: (g) => <RelativeTime value={g.updated_at} className="text-ink-muted" /> },
]

export const groupTypeFilter: IndexFilter<ComputerGroup> = {
  key: 'type',
  label: 'Any type',
  options: [
    { value: 'smart', label: 'Smart' },
    { value: 'static', label: 'Static' },
  ],
  match: (g, v) => g.is_smart === (v === 'smart'),
}

export function GroupsPage() {
  const groups = useGroups()
  const invalidate = useInvalidate()

  return (
    <IndexPage<ComputerGroup>
      title="Computer groups"
      description="Smart groups update automatically from inventory criteria; static groups are curated by hand."
      newTo="/groups/new"
      newLabel="New group"
      rows={groups.data?.items}
      loading={groups.isPending}
      error={groups.error}
      rowKey={(g) => g.id}
      rowHref={(g) => `/groups/${g.id}`}
      search={{ placeholder: 'Search groups…', text: (g) => `${g.name} ${g.description}` }}
      filters={[groupTypeFilter]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.groups, qk.dashboard)}
      bulkActions={[
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${n} ${n === 1 ? 'group' : 'groups'}?`, description: 'Policies and profiles scoped to these groups will no longer target their members.' },
          run: (g) => api.del(`/api/v1/groups/${g.id}`),
          done: (n) => `${n} ${n === 1 ? 'group' : 'groups'} deleted`,
        },
      ]}
      empty={{ title: 'No groups yet', description: 'Create a smart group to target computers by criteria, or a static group to pick them by hand.' }}
      columns={groupColumns}
    />
  )
}
