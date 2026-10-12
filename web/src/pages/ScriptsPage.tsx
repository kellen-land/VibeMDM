import { Terminal, Trash2 } from 'lucide-react'
import { qk, useInvalidate, useScripts } from '../api/queries'
import { api } from '../api/client'
import type { Script } from '../api/types'
import { Badge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'
import { IndexPage } from '../components/templates/IndexPage'

const priorityLabel = (s: Script) => (s.priority === 'before' ? 'Before packages' : 'After packages')

export function ScriptsPage() {
  const scripts = useScripts()
  const invalidate = useInvalidate()
  const rows = scripts.data?.items
  const categories = [...new Set((rows ?? []).map((s) => s.category).filter(Boolean))].sort()

  return (
    <IndexPage<Script>
      title="Scripts"
      description="Shell scripts that run on computers via policies or on demand."
      newTo="/scripts/new"
      newLabel="New script"
      rows={rows}
      loading={scripts.isPending}
      error={scripts.error}
      rowKey={(s) => s.id}
      rowHref={(s) => `/scripts/${s.id}`}
      search={{ placeholder: 'Search scripts…', text: (s) => `${s.name} ${s.category} ${s.info}` }}
      filters={
        categories.length
          ? [{ key: 'category', label: 'Any category', options: categories.map((c) => ({ value: c, label: c })), match: (s, v) => s.category === v }]
          : undefined
      }
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.scripts, qk.dashboard)}
      bulkActions={[
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${n} ${n === 1 ? 'script' : 'scripts'}?`, description: 'Policies that include these scripts will no longer run them.' },
          run: (s) => api.del(`/api/v1/scripts/${s.id}`),
          done: (n) => `${n} ${n === 1 ? 'script' : 'scripts'} deleted`,
        },
      ]}
      empty={{ title: 'No scripts yet', description: 'Add a shell script to run it through policies or directly on computers.' }}
      columns={[
        {
          key: 'name',
          header: 'Name',
          sortValue: (s) => s.name,
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
        { key: 'category', header: 'Category', sortValue: (s) => s.category, render: (s) => (s.category ? <Badge>{s.category}</Badge> : <span className="text-ink-faint">—</span>) },
        { key: 'priority', header: 'Priority', sortValue: priorityLabel, render: (s) => <span className="text-ink-muted">{priorityLabel(s)}</span> },
        { key: 'params', header: 'Parameters', sortValue: (s) => (s.parameter_labels ?? []).filter(Boolean).length, render: (s) => <span className="text-ink-muted tabular-nums">{(s.parameter_labels ?? []).filter(Boolean).length} labelled</span> },
        { key: 'updated', header: 'Updated', sortValue: (s) => s.updated_at, render: (s) => <RelativeTime value={s.updated_at} className="text-ink-muted" /> },
      ]}
    />
  )
}
