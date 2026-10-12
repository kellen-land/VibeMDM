import { useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertTriangle, Clock, Laptop, Radio } from 'lucide-react'
import { useDashboard } from '../api/queries'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Alert } from '../components/ui/Alert'
import { LoadingBlock } from '../components/ui/Spinner'
import { BarList } from '../components/charts/BarList'
import { OutcomeBar } from '../components/charts/OutcomeBar'
import { Table } from '../components/ui/Table'
import { Tabs } from '../components/ui/Tabs'
import { RelativeTime } from '../components/ui/RelativeTime'
import { StatusBadge, Badge } from '../components/ui/Badge'
import { compactNumber, titleCase } from '../lib/format'
import { errorMessage } from '../api/client'
import type { AgentTask, Computer, PolicyLog } from '../api/types'
import { useNavigate } from 'react-router-dom'
import { MetricsCard, CalloutCard } from '../components/templates/Cards'
import { HealthCard } from './health/HealthPage'

export function DashboardPage() {
  const dash = useDashboard()
  const navigate = useNavigate()
  const d = dash.data
  const [recent, setRecent] = useState<'checkins' | 'policies' | 'tasks'>('checkins')
  // A fleet of only iPhones and iPads never enrolls a Mac: let the setup callout be dismissed.
  const [enrollHidden, setEnrollHidden] = useState(() => {
    try {
      return localStorage.getItem('vibemdm.dashboard.enrollCalloutHidden') === '1'
    } catch {
      return false
    }
  })
  const hideEnroll = () => {
    setEnrollHidden(true)
    try {
      localStorage.setItem('vibemdm.dashboard.enrollCalloutHidden', '1')
    } catch {
      /* private mode: hidden for this visit only */
    }
  }

  return (
    <>
      <PageHeader title="Dashboard" description="Fleet overview and recent activity." />
      {dash.isPending && <LoadingBlock />}
      {dash.error && <Alert tone="critical">{errorMessage(dash.error)}</Alert>}
      {d && (
        <div className="flex flex-col gap-6">
          {d.computers === 0 && !d.mobile_devices && !enrollHidden && (
            <CalloutCard title="Enroll your first Mac" icon={<Laptop />} action="Go to enrollment" actionTo="/enrollment" onDismiss={hideEnroll}>
              Computers show up here once they enroll with MDM or install the agent.
            </CalloutCard>
          )}
          <HealthCard />
          <div>
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
              <MetricsCard label="Computers" value={<Figure n={d.computers} />} icon={<Laptop />} to="/computers" note={`${d.mdm_enrolled} with MDM · ${d.agent_enrolled} with the agent`} />
              <MetricsCard label="Checked in (24h)" value={<Figure n={d.checked_in_24h} />} icon={<Radio />} note={`${d.agents_online ?? 0} online now`} />
              <MetricsCard label="Not seen in 30 days" value={<Figure n={d.not_seen_30d} />} icon={<Clock />} tone={d.not_seen_30d > 0 ? 'warning' : undefined} />
              <MetricsCard
                label="Failed policies (7d)"
                value={<Figure n={d.failed_policies_7d} />}
                icon={<AlertTriangle />}
                to="/activity/policy-logs"
                tone={d.failed_policies_7d > 0 ? 'critical' : undefined}
                note={`${d.completed_policies_7d} completed`}
              />
            </div>
            {d.pending_commands + d.pending_tasks > 0 && (
              <p className="mt-3 text-xs text-ink-muted">
                <Link to="/activity/commands" className="hover:text-ink hover:underline">
                  {d.pending_commands} MDM {d.pending_commands === 1 ? 'command' : 'commands'}
                </Link>{' '}
                and{' '}
                <Link to="/activity/tasks" className="hover:text-ink hover:underline">
                  {d.pending_tasks} agent {d.pending_tasks === 1 ? 'task' : 'tasks'}
                </Link>{' '}
                waiting
              </p>
            )}
          </div>

          <div className="grid gap-6 lg:grid-cols-3">
            <Card>
              <CardHeader title="macOS versions" description="Computers by reported OS version" />
              <CardBody>
                <BarList data={d.os_versions} emptyLabel="No computers enrolled yet" />
              </CardBody>
            </Card>
            <Card>
              <CardHeader title="Models" description="Computers by hardware model" />
              <CardBody>
                <BarList data={d.models} emptyLabel="No computers enrolled yet" />
              </CardBody>
            </Card>
            <Card>
              <CardHeader title="Policy outcomes" description="Runs in the last 7 days" />
              <CardBody>
                <OutcomeBar completed={d.completed_policies_7d} failed={d.failed_policies_7d} />
                <div className="mt-4 grid grid-cols-3 gap-3 border-t border-line pt-4 text-xs text-ink-muted">
                  <div>
                    <p className="text-ink text-base font-semibold">{d.policies_enabled}</p>
                    <p>of {d.policies} policies enabled</p>
                  </div>
                  <div>
                    <p className="text-ink text-base font-semibold">{d.profiles}</p>
                    <p>profiles</p>
                  </div>
                  <div>
                    <p className="text-ink text-base font-semibold">{d.groups}</p>
                    <p>groups</p>
                  </div>
                </div>
              </CardBody>
            </Card>
          </div>

          <Card>
            <CardHeader
              title="Recent activity"
              actions={
                <Link to={recent === 'checkins' ? '/computers' : recent === 'policies' ? '/activity/policy-logs' : '/activity/tasks'} className="text-xs text-accent-700 hover:underline dark:text-accent-300">
                  View all
                </Link>
              }
            />
            <div className="px-4 pt-2">
              <Tabs
                items={[
                  { key: 'checkins', label: 'Check-ins' },
                  { key: 'policies', label: 'Policy runs' },
                  { key: 'tasks', label: 'Agent tasks' },
                ]}
                value={recent}
                onChange={(k) => setRecent(k as typeof recent)}
              />
            </div>
            {recent === 'checkins' && (
              <Table<Computer>
                rows={d.recent_checkins}
                rowKey={(c) => c.id}
                dense
                onRowClick={(c) => navigate(`/computers/${c.id}`)}
                emptyTitle="No computers have checked in yet"
                columns={[
                  { key: 'name', header: 'Computer', render: (c) => <span className="font-medium text-ink">{c.name || c.hostname || c.serial_number}</span> },
                  { key: 'user', header: 'User', render: (c) => <span className="text-ink-muted">{c.last_user || '—'}</span> },
                  { key: 'os', header: 'macOS', render: (c) => <span className="tabular-nums">{c.os_version || '—'}</span> },
                  { key: 'last', header: 'Last check-in', render: (c) => <RelativeTime value={latest(c.agent_last_checkin, c.mdm_last_checkin)} className="text-ink-muted" /> },
                ]}
              />
            )}
            {recent === 'policies' && (
              <Table<PolicyLog>
                rows={d.recent_policy_logs}
                rowKey={(l) => l.id}
                dense
                onRowClick={(l) => navigate(`/policies/${l.policy_id}?tab=logs`)}
                emptyTitle="No policy runs yet"
                columns={[
                  { key: 'policy', header: 'Policy', render: (l) => <span className="font-medium text-ink">{l.policy_name || '—'}</span> },
                  { key: 'computer', header: 'Computer', render: (l) => <span className="text-ink-muted">{l.computer_name || '—'}</span> },
                  { key: 'trigger', header: 'Trigger', render: (l) => <Badge>{titleCase(l.trigger || 'manual')}</Badge> },
                  { key: 'status', header: 'Status', render: (l) => <StatusBadge status={l.status} /> },
                  { key: 'at', header: 'When', render: (l) => <RelativeTime value={l.completed_at ?? l.created_at} className="text-ink-muted" /> },
                ]}
              />
            )}
            {recent === 'tasks' && (
              <Table<AgentTask>
                rows={d.recent_tasks}
                rowKey={(t) => t.id}
                dense
                onRowClick={(t) => navigate(`/computers/${t.computer_id}/tasks`)}
                emptyTitle="No agent tasks yet"
                columns={[
                  { key: 'type', header: 'Task', render: (t) => <span className="font-medium text-ink">{taskLabel(t)}</span> },
                  { key: 'computer', header: 'Computer', render: (t) => <span className="text-ink-muted">{t.computer_name || '—'}</span> },
                  { key: 'by', header: 'Created by', render: (t) => <span className="text-ink-muted">{t.created_by || 'system'}</span> },
                  { key: 'status', header: 'Status', render: (t) => <StatusBadge status={t.status} /> },
                  { key: 'at', header: 'When', render: (t) => <RelativeTime value={t.completed_at ?? t.created_at} className="text-ink-muted" /> },
                ]}
              />
            )}
          </Card>
        </div>
      )}
    </>
  )
}

/** A headline figure, compact (1.2k) with the exact number on hover. */
function Figure({ n }: { n: number }) {
  return <span title={n.toLocaleString()}>{compactNumber(n)}</span>
}

export function latest(a: string | null, b: string | null): string | null {
  if (!a) return b
  if (!b) return a
  return new Date(a) > new Date(b) ? a : b
}

export function taskLabel(t: AgentTask): string {
  const p = t.payload ?? {}
  const name = typeof p.name === 'string' ? p.name : ''
  switch (t.type) {
    case 'run_script':
      return name ? `Run script: ${name}` : 'Run script'
    case 'run_policy':
      return name ? `Run policy: ${name}` : 'Run policy'
    case 'run_command':
      return 'Run command'
    case 'inventory':
      return 'Update inventory'
    case 'custom_trigger':
      return `Trigger: ${typeof p.trigger === 'string' ? p.trigger : ''}`
    case 'logout_user':
      return 'Log out user'
    case 'notify':
      return 'Send notification'
    default:
      return titleCase(t.type)
  }
}
