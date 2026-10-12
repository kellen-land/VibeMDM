import { useState } from 'react'
import { ListChecks, Power, PowerOff, Trash2 } from 'lucide-react'
import { qk, useInvalidate, usePolicies } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { PolicyTriggers, PolicyWithStatus } from '../api/types'
import { Badge } from '../components/ui/Badge'
import { Toggle } from '../components/ui/Checkbox'
import { useToast } from '../components/ui/Toast'
import { ScopeSummary } from '../components/pickers/ScopeEditor'
import { IndexPage } from '../components/templates/IndexPage'

export const frequencyLabels: Record<string, string> = {
  ongoing: 'Ongoing',
  once_per_computer: 'Once per computer',
  once_per_day: 'Once every day',
  once_per_week: 'Once every week',
  once_per_month: 'Once every month',
}

export function triggerSummary(t: PolicyTriggers): string[] {
  const out: string[] = []
  if (t.startup) out.push('Startup')
  if (t.login) out.push('Login')
  if (t.checkin) out.push('Check-in')
  if (t.enrollment_complete) out.push('Enrollment')
  if (t.network_change) out.push('Network change')
  if (t.custom) out.push(`Custom: ${t.custom}`)
  return out
}

export function PoliciesPage() {
  const { isAdmin } = useAuth()
  const policies = usePolicies()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [toggling, setToggling] = useState<string | null>(null)
  const rows = policies.data?.items
  const categories = [...new Set((rows ?? []).map((p) => p.category).filter(Boolean))].sort()

  const setEnabled = async (p: PolicyWithStatus, enabled: boolean) => {
    const { status: _s, ...body } = p
    void _s
    await api.put(`/api/v1/policies/${p.id}`, { ...body, enabled })
  }
  const toggle = async (p: PolicyWithStatus, enabled: boolean) => {
    setToggling(p.id)
    try {
      await setEnabled(p, enabled)
      await invalidate(qk.policies, qk.dashboard)
      toast.success(enabled ? 'Policy enabled' : 'Policy disabled')
    } catch (e) {
      toast.error('Update failed', errorMessage(e))
    } finally {
      setToggling(null)
    }
  }

  return (
    <IndexPage<PolicyWithStatus>
      title="Policies"
      description="Automate scripts, package installs and maintenance on scoped computers."
      newTo="/policies/new"
      newLabel="New policy"
      rows={rows}
      loading={policies.isPending}
      error={policies.error}
      rowKey={(p) => p.id}
      rowHref={(p) => `/policies/${p.id}`}
      rowClassName={(p) => (p.enabled ? undefined : 'opacity-70')}
      search={{ placeholder: 'Search policies…', text: (p) => `${p.name} ${p.category}` }}
      filters={[
        { key: 'state', label: 'Any state', options: [{ value: 'on', label: 'Enabled' }, { value: 'off', label: 'Disabled' }], match: (p, v) => p.enabled === (v === 'on') },
        ...(categories.length ? [{ key: 'category', label: 'Any category', options: categories.map((c) => ({ value: c, label: c })), match: (p: PolicyWithStatus, v: string) => p.category === v }] : []),
        { key: 'failed', label: 'Any status', options: [{ value: 'failed', label: 'With failures' }], match: (p) => (p.status.failed ?? 0) > 0 },
      ]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.policies, qk.dashboard)}
      bulkActions={[
        { label: 'Enable', icon: <Power />, run: (p) => setEnabled(p, true), done: (n) => `${n} ${n === 1 ? 'policy' : 'policies'} enabled` },
        { label: 'Disable', icon: <PowerOff />, run: (p) => setEnabled(p, false), done: (n) => `${n} ${n === 1 ? 'policy' : 'policies'} disabled` },
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${n} ${n === 1 ? 'policy' : 'policies'}?`, description: 'The policies and their logs are removed. Computers that already ran them are not affected.' },
          run: (p) => api.del(`/api/v1/policies/${p.id}`),
          done: (n) => `${n} ${n === 1 ? 'policy' : 'policies'} deleted`,
        },
      ]}
      empty={{ title: 'No policies yet', description: 'Create a policy to run scripts or install packages on your computers.' }}
      columns={[
        {
          key: 'enabled',
          header: 'On',
          width: '60px',
          sortValue: (p) => (p.enabled ? 0 : 1),
          render: (p) => (
            <span onClick={(e) => e.stopPropagation()}>
              <Toggle size="sm" checked={p.enabled} onChange={(v) => toggle(p, v)} disabled={!isAdmin || toggling === p.id} />
            </span>
          ),
        },
        {
          key: 'name',
          header: 'Name',
          sortValue: (p) => p.name,
          render: (p) => (
            <span className="flex items-center gap-2.5">
              <ListChecks className="size-4 text-ink-faint" />
              <span>
                <span className="block font-medium text-ink">{p.name}</span>
                {p.category && <span className="block text-xs text-ink-muted">{p.category}</span>}
              </span>
            </span>
          ),
        },
        {
          key: 'triggers',
          header: 'Triggers',
          render: (p) => {
            const t = triggerSummary(p.triggers)
            return t.length ? (
              <span className="flex flex-wrap gap-1">
                {t.map((x) => (
                  <Badge key={x}>{x}</Badge>
                ))}
              </span>
            ) : (
              <span className="text-xs text-ink-faint">Manual only</span>
            )
          },
        },
        { key: 'frequency', header: 'Frequency', sortValue: (p) => frequencyLabels[p.frequency] ?? p.frequency, render: (p) => <span className="text-ink-muted">{frequencyLabels[p.frequency] ?? p.frequency}</span> },
        { key: 'scope', header: 'Scope', render: (p) => <ScopeSummary scope={p.scope} /> },
        {
          key: 'status',
          header: 'Status',
          sortValue: (p) => p.status.failed ?? 0,
          render: (p) => (
            <span className="flex gap-1.5 text-xs tabular-nums">
              <Badge tone="good" title="Completed">{p.status.completed ?? 0}</Badge>
              <Badge tone="critical" title="Failed">{p.status.failed ?? 0}</Badge>
              <Badge tone="info" title="Pending">{(p.status.pending ?? 0) + (p.status.running ?? 0)}</Badge>
            </span>
          ),
        },
      ]}
    />
  )
}
