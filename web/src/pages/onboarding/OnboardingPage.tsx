import { CircleOff, Trash2 } from 'lucide-react'
import { qk, useInvalidate, useOnboardingWorkflows } from '../../api/queries'
import { api } from '../../api/client'
import type { OnboardingWorkflow } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ScopeSummary } from '../../components/pickers/ScopeEditor'
import { IndexPage } from '../../components/templates/IndexPage'

export function OnboardingPage() {
  const q = useOnboardingWorkflows()
  const invalidate = useInvalidate()
  const rows = q.data ? (q.data.items ?? []) : undefined
  const nWorkflows = (n: number) => `${n} ${n === 1 ? 'workflow' : 'workflows'}`

  return (
    <IndexPage<OnboardingWorkflow>
      title="Onboarding"
      description="White-glove setup that greets the first user of a newly enrolled Mac, collects details, names the Mac and installs what they need."
      newTo="/onboarding/new"
      newLabel="New workflow"
      rows={rows}
      loading={q.isPending}
      error={q.error}
      rowKey={(w) => w.id}
      rowHref={(w) => `/onboarding/${w.id}`}
      rowClassName={(w) => (!w.enabled ? 'opacity-70' : undefined)}
      search={{ placeholder: 'Search workflows…', text: (w) => `${w.name} ${w.config.title ?? ''}` }}
      filters={[{ key: 'state', label: 'Any state', options: [{ value: 'on', label: 'Enabled' }, { value: 'off', label: 'Disabled' }], match: (w, v) => w.enabled === (v === 'on') }]}
      defaultSort={{ key: 'priority', dir: 'asc' }}
      onChanged={() => invalidate(qk.onboarding)}
      bulkActions={[
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${nWorkflows(n)}?`, description: 'Macs that already ran them are not affected; new Macs in scope are no longer onboarded by them.' },
          run: (w) => api.del(`/api/v1/onboarding/workflows/${w.id}`),
          done: (n) => `${nWorkflows(n)} deleted`,
        },
      ]}
      empty={{
        title: 'No workflows yet',
        description: 'A workflow welcomes the first user of a newly enrolled Mac, collects details such as the asset tag, names the Mac and installs apps, policies and scripts.',
      }}
      footer={(rows?.length ?? 0) > 1 && <p className="mt-3 text-xs text-ink-muted">When several enabled workflows include the same Mac, only the one with the lowest priority number runs, once per Mac.</p>}
      columns={[
        {
          key: 'name',
          header: 'Workflow',
          sortValue: (w) => w.name,
          render: (w) => (
            <span className="block">
              <span className="block font-medium text-ink">{w.name}</span>
              <span className="block truncate text-xs text-ink-muted">{w.config.title || 'Setting up your Mac'}</span>
            </span>
          ),
        },
        {
          key: 'state',
          header: 'State',
          sortValue: (w) => (w.enabled ? 0 : 1),
          render: (w) =>
            w.enabled ? (
              <Badge tone="good" dot>
                Enabled
              </Badge>
            ) : (
              <Badge icon={<CircleOff />}>Disabled</Badge>
            ),
        },
        { key: 'priority', header: 'Priority', align: 'right', sortValue: (w) => w.priority, render: (w) => <span className="tabular-nums text-ink-muted">{w.priority}</span> },
        { key: 'scope', header: 'Scope', render: (w) => <ScopeSummary scope={w.scope} /> },
        { key: 'fields', header: 'Fields', align: 'right', sortValue: (w) => w.config.fields?.length ?? 0, render: (w) => <span className="tabular-nums text-ink-muted">{w.config.fields?.length ?? 0}</span> },
        { key: 'steps', header: 'Steps', align: 'right', sortValue: (w) => w.config.steps?.length ?? 0, render: (w) => <span className="tabular-nums text-ink-muted">{w.config.steps?.length ?? 0}</span> },
        { key: 'updated', header: 'Updated', align: 'right', sortValue: (w) => w.updated_at, render: (w) => <RelativeTime value={w.updated_at} className="text-ink-muted" /> },
      ]}
    />
  )
}
