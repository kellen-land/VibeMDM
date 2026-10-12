import { Tag, Trash2 } from 'lucide-react'
import { qk, useCustomLabels, useInvalidate } from '../api/queries'
import { api } from '../api/client'
import type { CustomLabel } from '../api/types'
import { Badge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'
import { IndexPage } from '../components/templates/IndexPage'
import { AppsTabs, InstallomatorFooter } from './AppsPage'

function host(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}

export function CustomLabelsPage() {
  const q = useCustomLabels()
  const invalidate = useInvalidate()
  const rows = q.data?.items
  const types = [...new Set((rows ?? []).map((l) => l.type).filter(Boolean))].sort()
  const nLabels = (n: number) => `${n} custom ${n === 1 ? 'label' : 'labels'}`

  return (
    <IndexPage<CustomLabel>
      title="Mac apps"
      description="Custom Installomator labels for apps the shipped labels don't cover. Reference one as custom:<key> in the catalog or in a policy."
      newTo="/apps/custom-labels/new"
      newLabel="New custom label"
      above={<AppsTabs active="custom-labels" />}
      rows={rows}
      loading={q.isPending}
      error={q.error}
      rowKey={(l) => l.id}
      rowHref={(l) => `/apps/custom-labels/${l.id}`}
      search={{ placeholder: 'Search custom labels…', text: (l) => `${l.key} ${l.name} ${l.download_url}` }}
      filters={types.length > 1 ? [{ key: 'type', label: 'Any type', options: types.map((t) => ({ value: t, label: t })), match: (l, v) => l.type === v }] : undefined}
      defaultSort={{ key: 'label', dir: 'asc' }}
      onChanged={() => invalidate(qk.customLabels, qk.appLabels)}
      bulkActions={[
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${nLabels(n)}?`, description: "Labels still used by a Mac app or a policy can't be deleted; remove them there first." },
          run: (l) => api.del(`/api/v1/custom-labels/${l.id}`),
          done: (n) => `${nLabels(n)} deleted`,
        },
      ]}
      empty={{ title: 'No custom labels yet', description: 'Define a download URL, type and Apple Developer Team ID; Installomator verifies the signature before installing.' }}
      footer={<InstallomatorFooter />}
      columns={[
        {
          key: 'label',
          header: 'Label',
          sortValue: (l) => l.name,
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
        { key: 'type', header: 'Type', sortValue: (l) => l.type, render: (l) => <Badge className="font-mono">{l.type}</Badge> },
        { key: 'team', header: 'Team ID', sortValue: (l) => l.expected_team_id, render: (l) => <span className="font-mono text-xs text-ink">{l.expected_team_id}</span> },
        { key: 'url', header: 'Download from', sortValue: (l) => host(l.download_url), render: (l) => <span className="font-mono text-xs text-ink-muted" title={l.download_url}>{host(l.download_url)}</span> },
        { key: 'updated', header: 'Updated', sortValue: (l) => l.updated_at, render: (l) => <RelativeTime value={l.updated_at} className="text-ink-muted" /> },
      ]}
    />
  )
}
