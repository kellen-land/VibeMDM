import { Link } from 'react-router-dom'
import { AlertTriangle, Clock, Cpu, Laptop, ListChecks, Radio, ShieldCheck, Timer, Wifi, type LucideIcon } from 'lucide-react'
import { useDashboard, useEnrollmentStatus } from '../api/queries'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Alert } from '../components/ui/Alert'
import { Button } from '../components/ui/Button'
import { LoadingBlock } from '../components/ui/Spinner'
import { BarList } from '../components/charts/BarList'
import { OutcomeBar } from '../components/charts/OutcomeBar'
import { Table } from '../components/ui/Table'
import { RelativeTime } from '../components/ui/RelativeTime'
import { StatusBadge, Badge } from '../components/ui/Badge'
import { compactNumber, titleCase } from '../lib/format'
import { errorMessage } from '../api/client'
import type { AgentTask, Computer, PolicyLog } from '../api/types'
import { useNavigate } from 'react-router-dom'
import { cn } from '../lib/cn'

function StatTile({ label, value, icon: Icon, to, tone }: { label: string; value: number; icon: LucideIcon; to?: string; tone?: 'warn' | 'bad' }) {
  const alert = (tone === 'bad' || tone === 'warn') && value > 0
  const body = (
    <Card className={cn('group relative h-full overflow-hidden px-4 pt-3.5 pb-4 transition-colors', to && 'hover:border-line-strong')}>
      <div className="flex items-start justify-between gap-2">
        <p className="micro-label">{label}</p>
        <Icon className={cn('size-4 shrink-0', tone === 'bad' && value > 0 ? 'text-critical' : tone === 'warn' && value > 0 ? 'text-warning' : 'text-ink-faint group-hover:text-accent-400')} aria-hidden="true" />
      </div>
      <p className="mt-2 text-[30px] font-semibold tracking-tight text-ink leading-9 tabular-nums" title={value.toLocaleString()}>
        {compactNumber(value)}
      </p>
      <span
        className={cn(
          'absolute inset-x-0 bottom-0 h-[2px] transition-all',
          alert ? (tone === 'bad' ? 'bg-critical/70' : 'bg-warning/70') : 'bg-accent-500/0 group-hover:bg-accent-500/70',
          to && !alert && 'group-hover:bg-accent-500/70',
        )}
        aria-hidden="true"
      />
    </Card>
  )
  return to ? (
    <Link to={to} className="block focus-ring rounded-xl">
      {body}
    </Link>
  ) : (
    body
  )
}

export function DashboardPage() {
  const dash = useDashboard()
  const enroll = useEnrollmentStatus()
  const navigate = useNavigate()
  const d = dash.data

  return (
    <>
      <PageHeader title="Dashboard" description="Fleet overview and recent activity." />
      {enroll.data && enroll.data.issues.length > 0 && (
        <Alert
          tone="warning"
          title="Setup is not complete"
          className="mb-6"
          actions={
            <Link to="/settings">
              <Button size="sm" variant="secondary">
                Open Settings
              </Button>
            </Link>
          }
        >
          <ul className="list-disc pl-4 space-y-0.5">
            {enroll.data.issues.map((i) => (
              <li key={i}>{i}</li>
            ))}
          </ul>
        </Alert>
      )}
      {dash.isPending && <LoadingBlock />}
      {dash.error && <Alert tone="critical">{errorMessage(dash.error)}</Alert>}
      {d && (
        <div className="flex flex-col gap-6">
          <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-5">
            <StatTile label="Computers" value={d.computers} icon={Laptop} to="/computers" />
            <StatTile label="MDM enrolled" value={d.mdm_enrolled} icon={ShieldCheck} />
            <StatTile label="Agent installed" value={d.agent_enrolled} icon={Cpu} />
            <StatTile label="Agents online" value={d.agents_online ?? 0} icon={Wifi} />
            <StatTile label="Checked in (24h)" value={d.checked_in_24h} icon={Radio} />
            <StatTile label="Not seen in 30 days" value={d.not_seen_30d} icon={Clock} tone="warn" />
            <StatTile label="Pending MDM commands" value={d.pending_commands} icon={Timer} to="/activity/commands" />
            <StatTile label="Pending agent tasks" value={d.pending_tasks} icon={ListChecks} to="/activity/tasks" />
            <StatTile label="Failed policies (7d)" value={d.failed_policies_7d} icon={AlertTriangle} to="/activity/policy-logs" tone="bad" />
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

          <div className="grid gap-6 xl:grid-cols-2">
            <Card>
              <CardHeader
                title="Recent check-ins"
                actions={
                  <Link to="/computers" className="text-xs text-accent-700 hover:underline dark:text-accent-300">
                    View all
                  </Link>
                }
              />
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
            </Card>
            <Card>
              <CardHeader
                title="Recent policy runs"
                actions={
                  <Link to="/activity/policy-logs" className="text-xs text-accent-700 hover:underline dark:text-accent-300">
                    View all
                  </Link>
                }
              />
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
            </Card>
            <Card className="xl:col-span-2">
              <CardHeader
                title="Recent agent tasks"
                actions={
                  <Link to="/activity/tasks" className="text-xs text-accent-700 hover:underline dark:text-accent-300">
                    View all
                  </Link>
                }
              />
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
            </Card>
          </div>
        </div>
      )}
    </>
  )
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
