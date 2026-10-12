import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ListChecks, Plus, Trash2 } from 'lucide-react'
import { qk, useInvalidate, usePolicies } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { PolicyTriggers, PolicyWithStatus } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card } from '../components/ui/Card'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { Toggle } from '../components/ui/Checkbox'
import { SearchInput } from '../components/ui/SearchInput'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { useToast } from '../components/ui/Toast'
import { ScopeSummary } from '../components/pickers/ScopeEditor'

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
  const navigate = useNavigate()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [q, setQ] = useState('')
  const [del, setDel] = useState<PolicyWithStatus | null>(null)
  const [toggling, setToggling] = useState<string | null>(null)
  const items = (policies.data?.items ?? []).filter((p) => !q || p.name.toLowerCase().includes(q.toLowerCase()) || p.category.toLowerCase().includes(q.toLowerCase()))

  const toggle = async (p: PolicyWithStatus, enabled: boolean) => {
    setToggling(p.id)
    try {
      const { status: _s, ...body } = p
      void _s
      await api.put(`/api/v1/policies/${p.id}`, { ...body, enabled })
      await invalidate(qk.policies, qk.dashboard)
      toast.success(enabled ? 'Policy enabled' : 'Policy disabled')
    } catch (e) {
      toast.error('Update failed', errorMessage(e))
    } finally {
      setToggling(null)
    }
  }

  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/policies/${del.id}`)
      toast.success('Policy deleted')
      await invalidate(qk.policies, qk.dashboard)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader
        title="Policies"
        description="Automate scripts, package installs and maintenance on scoped computers."
        actions={
          isAdmin && (
            <Link to="/policies/new" data-shortcut="new">
              <Button variant="primary" icon={<Plus />}>
                New policy
              </Button>
            </Link>
          )
        }
      />
      <Card>
        <div className="flex items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={setQ} placeholder="Filter policies…" className="w-full sm:w-72" />
        </div>
        <Table<PolicyWithStatus>
          rows={items}
          loading={policies.isPending}
          error={policies.error ? errorMessage(policies.error) : null}
          rowKey={(p) => p.id}
          onRowClick={(p) => navigate(`/policies/${p.id}`)}
          emptyTitle={q ? 'No policies match' : 'No policies yet'}
          emptyDescription={q ? undefined : 'Create a policy to run scripts or install packages on your computers.'}
          emptyAction={
            isAdmin && !q ? (
              <Link to="/policies/new" data-shortcut="new">
                <Button variant="primary" size="sm" icon={<Plus />}>
                  New policy
                </Button>
              </Link>
            ) : undefined
          }
          rowClassName={(p) => (p.enabled ? undefined : 'opacity-70')}
          columns={[
            {
              key: 'enabled',
              header: 'On',
              width: '60px',
              render: (p) => (
                <span onClick={(e) => e.stopPropagation()}>
                  <Toggle size="sm" checked={p.enabled} onChange={(v) => toggle(p, v)} disabled={!isAdmin || toggling === p.id} />
                </span>
              ),
            },
            {
              key: 'name',
              header: 'Name',
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
                  <span className="text-ink-faint text-xs">Manual only</span>
                )
              },
            },
            { key: 'frequency', header: 'Frequency', render: (p) => <span className="text-ink-muted">{frequencyLabels[p.frequency] ?? p.frequency}</span> },
            { key: 'scope', header: 'Scope', render: (p) => <ScopeSummary scope={p.scope} /> },
            {
              key: 'status',
              header: 'Status',
              render: (p) => (
                <span className="flex gap-1.5 text-xs tabular-nums">
                  <Badge tone="good" title="Completed">{p.status.completed ?? 0}</Badge>
                  <Badge tone="critical" title="Failed">{p.status.failed ?? 0}</Badge>
                  <Badge tone="info" title="Pending">{(p.status.pending ?? 0) + (p.status.running ?? 0)}</Badge>
                </span>
              ),
            },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (p) =>
                isAdmin ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<Trash2 />}
                    aria-label="Delete policy"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDel(p)
                    }}
                  />
                ) : null,
            },
          ]}
        />
      </Card>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete "${del?.name}"?`} description="The policy and its logs are removed. Computers that already ran it are not affected." confirmLabel="Delete policy" />
    </>
  )
}
