import { Puzzle, Trash2 } from 'lucide-react'
import { qk, useExtensionAttributes, useInvalidate } from '../api/queries'
import { api } from '../api/client'
import type { ExtensionAttribute } from '../api/types'
import { Badge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'
import { IndexPage } from '../components/templates/IndexPage'

const plural = (n: number) => `${n} extension ${n === 1 ? 'attribute' : 'attributes'}`

export function ExtensionAttributesPage() {
  const eas = useExtensionAttributes()
  const invalidate = useInvalidate()

  return (
    <IndexPage<ExtensionAttribute>
      title="Extension attributes"
      description="Custom inventory fields collected by a script on each computer or entered by hand."
      newTo="/extension-attributes/new"
      newLabel="New attribute"
      rows={eas.data?.items}
      loading={eas.isPending}
      error={eas.error}
      rowKey={(e) => e.id}
      rowHref={(e) => `/extension-attributes/${e.id}`}
      rowClassName={(e) => (e.enabled ? undefined : 'opacity-70')}
      search={{ placeholder: 'Search extension attributes…', text: (e) => `${e.name} ${e.description}` }}
      filters={[
        { key: 'state', label: 'Any state', options: [{ value: 'on', label: 'Enabled' }, { value: 'off', label: 'Disabled' }], match: (e, v) => e.enabled === (v === 'on') },
        { key: 'input', label: 'Any input', options: [{ value: 'script', label: 'Script' }, { value: 'text', label: 'Text field' }], match: (e, v) => e.input_type === v },
      ]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.eas, qk.criteriaFields)}
      bulkActions={[
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${plural(n)}?`, description: 'Collected values are deleted and smart groups using these attributes will stop matching.' },
          run: (e) => api.del(`/api/v1/extension-attributes/${e.id}`),
          done: (n) => `${plural(n)} deleted`,
        },
      ]}
      empty={{ title: 'No extension attributes yet', description: 'Create one to collect custom data, then use it in smart group criteria.' }}
      columns={[
        {
          key: 'name',
          header: 'Name',
          sortValue: (e) => e.name,
          render: (e) => (
            <span className="flex items-center gap-2.5">
              <Puzzle className="size-4 text-ink-faint" />
              <span className="min-w-0">
                <span className="block font-medium text-ink">{e.name}</span>
                {e.description && <span className="block text-xs text-ink-muted truncate max-w-md">{e.description}</span>}
              </span>
            </span>
          ),
        },
        { key: 'data', header: 'Data type', sortValue: (e) => e.data_type, render: (e) => <Badge>{e.data_type}</Badge> },
        { key: 'input', header: 'Input', sortValue: (e) => e.input_type, render: (e) => <span className="text-ink-muted">{e.input_type === 'script' ? 'Script' : 'Text field'}</span> },
        { key: 'enabled', header: 'Status', sortValue: (e) => (e.enabled ? 0 : 1), render: (e) => <Badge tone={e.enabled ? 'good' : 'neutral'} dot>{e.enabled ? 'Enabled' : 'Disabled'}</Badge> },
        { key: 'updated', header: 'Updated', sortValue: (e) => e.updated_at, render: (e) => <RelativeTime value={e.updated_at} className="text-ink-muted" /> },
      ]}
    />
  )
}
