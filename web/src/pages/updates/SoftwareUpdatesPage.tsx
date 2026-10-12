import { useState, type ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { CalendarClock, CheckCircle2, CircleDashed, Plus, Power, PowerOff, SlidersHorizontal, Trash2 } from 'lucide-react'
import { qk, useInvalidate, useUpdatePlans, useUpdatesOverview } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { UpdatePlan, UpdateRow, UpdateSettings, UpdateState } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table } from '../../components/ui/Table'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Tabs } from '../../components/ui/Tabs'
import { Toggle } from '../../components/ui/Checkbox'
import { Select } from '../../components/ui/Input'
import { SearchInput } from '../../components/ui/SearchInput'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'
import { ScopeSummary } from '../../components/pickers/ScopeEditor'
import { ComplianceBar } from '../../components/charts/ComplianceBar'
import {
  failureReasonLabel,
  formatLocalDeadline,
  installStateLabel,
  pendingVersionLabel,
  StateLegend,
  SupervisionCallout,
  UpdateStateBadge,
  updateStateMeta,
  updateStates,
} from '../../components/updates/updateState'
import { compactNumber } from '../../lib/format'
import { SuperTab } from './SuperTab'
import { cn } from '../../lib/cn'
import { shortcutHint } from '../../lib/shortcuts'
import { IndexPage } from '../../components/templates/IndexPage'

type TabKey = 'overview' | 'plans' | 'super'

const pageTitle = 'Software updates'
const pageDescription = 'Enforce macOS versions and control update behavior with Declarative Device Management, or run the super workflow for friendlier deferrals.'

export function SoftwareUpdatesPage() {
  const { tab } = useParams()
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const active: TabKey = tab === 'plans' || tab === 'super' ? tab : 'overview'
  const plans = useUpdatePlans()
  const tabs = (
    <>
      {active !== 'super' && <SupervisionCallout className="mb-5" />}
      <Tabs<TabKey>
        items={[
          { key: 'overview', label: 'Overview' },
          { key: 'plans', label: 'Plans', count: plans.data?.items.length },
          { key: 'super', label: 'super' },
        ]}
        value={active}
        onChange={(k) => navigate(k === 'overview' ? '/updates' : `/updates/${k}`, { replace: true })}
        className="mb-5"
      />
    </>
  )
  // The Plans tab is a list: it uses the Index template, which brings its own header.
  if (active === 'plans') return <PlansTab above={tabs} />
  return (
    <>
      <PageHeader
        title={pageTitle}
        description={pageDescription}
        actions={
          isAdmin &&
          active !== 'super' && (
            <Link to="/updates/plans/new" data-shortcut="new" title={shortcutHint('new', 'New plan')}>
              <Button variant="primary" icon={<Plus />}>
                New plan
              </Button>
            </Link>
          )
        }
      />
      {tabs}
      {active === 'overview' ? <OverviewTab /> : <SuperTab />}
    </>
  )
}

function StateTile({ state, value, active, onClick }: { state: UpdateState; value: number; active: boolean; onClick: () => void }) {
  const m = updateStateMeta[state]
  const accent = value > 0 && m.tone !== 'neutral'
  return (
    <button type="button" onClick={onClick} aria-pressed={active} className="block h-full text-left focus-ring rounded-xl">
      <Card className={cn('group relative h-full overflow-hidden px-4 pt-3.5 pb-4 transition-colors hover:border-line-strong', active && 'border-accent-500/60')}>
        <div className="flex items-start justify-between gap-2">
          <p className="micro-label">{m.label}</p>
          <m.icon className={cn('size-4 shrink-0', accent ? m.ink : 'text-ink-faint')} aria-hidden="true" />
        </div>
        <p className="mt-2 text-[30px] font-semibold tracking-tight text-ink leading-9 tabular-nums" title={value.toLocaleString()}>
          {compactNumber(value)}
        </p>
        <span className={cn('absolute inset-x-0 bottom-0 h-[2px]', accent ? m.fill : 'bg-transparent group-hover:bg-accent-500/70')} aria-hidden="true" />
      </Card>
    </button>
  )
}

function OverviewTab() {
  const q = useUpdatesOverview()
  const [state, setState] = useState<UpdateState | ''>('')
  const [search, setSearch] = useState('')
  if (q.isPending) return <LoadingBlock />
  if (q.error || !q.data) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'Update overview unavailable'}</Alert>
  const counts = q.data.counts ?? {}
  const s = search.trim().toLowerCase()
  const rows = (q.data.computers ?? []).filter((r) => (!state || r.state === state) && (!s || r.computer_name.toLowerCase().includes(s) || r.os_version.includes(s)))
  const toggleState = (st: UpdateState) => setState((cur) => (cur === st ? '' : st))

  return (
    <div className="flex flex-col gap-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
        {updateStates.map((st) => (
          <StateTile key={st} state={st} value={counts[st] ?? 0} active={state === st} onClick={() => toggleState(st)} />
        ))}
      </div>

      <Card>
        <CardHeader title="Compliance" description="Every Mac by update state. Select a segment or legend item to filter the table." />
        <CardBody>
          <ComplianceBar counts={counts} active={state} onSelect={setState} />
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title="Computers"
          description={`${rows.length.toLocaleString()} of ${(q.data.computers ?? []).length.toLocaleString()} computers`}
          actions={
            <div className="flex flex-wrap items-center gap-2">
              <Select value={state} onChange={(e) => setState(e.target.value as UpdateState | '')} className="w-40" aria-label="Filter by state">
                <option value="">All states</option>
                {updateStates.map((st) => (
                  <option key={st} value={st}>
                    {updateStateMeta[st].label}
                  </option>
                ))}
              </Select>
              <SearchInput value={search} onChange={setSearch} placeholder="Filter by name or version…" className="w-56" />
            </div>
          }
        />
        <Table<UpdateRow>
          rows={rows}
          rowKey={(r) => r.computer_id}
          emptyTitle={state || s ? 'No computers match' : 'No computers enrolled'}
          columns={[
            {
              key: 'name',
              header: 'Computer',
              render: (r) => (
                <Link to={`/computers/${r.computer_id}/updates`} className="font-medium text-ink hover:underline">
                  {r.computer_name || r.computer_id}
                </Link>
              ),
            },
            {
              key: 'os',
              header: 'macOS',
              render: (r) => (
                <span className="whitespace-nowrap">
                  <span className="tabular-nums text-ink">{r.os_version || '—'}</span>
                  {r.os_build && <span className="ml-1.5 font-mono text-xs text-ink-muted">{r.os_build}</span>}
                </span>
              ),
            },
            { key: 'state', header: 'State', render: (r) => <UpdateStateBadge state={r.state} /> },
            {
              key: 'enforcement',
              header: 'Enforcement',
              render: (r) =>
                r.enforcement_plan ? (
                  <span className="block">
                    <span className="block text-ink">{r.enforcement_plan}</span>
                    <span className="block text-xs text-ink-muted whitespace-nowrap">
                      macOS <span className="tabular-nums">{r.target_os_version}</span> by {formatLocalDeadline(r.deadline)}
                    </span>
                  </span>
                ) : (
                  <span className="text-ink-faint">—</span>
                ),
            },
            { key: 'settings', header: 'Settings plan', render: (r) => (r.settings_plan ? <span className="text-ink-muted">{r.settings_plan}</span> : <span className="text-ink-faint">—</span>) },
            { key: 'install', header: 'Install state', render: (r) => (r.install_state ? <span className="text-ink-muted whitespace-nowrap">{installStateLabel(r.install_state)}</span> : <span className="text-ink-faint">—</span>) },
            { key: 'pending', header: 'Pending', render: (r) => <span className="tabular-nums text-ink-muted whitespace-nowrap">{pendingVersionLabel(r.pending_version) || '—'}</span> },
            {
              key: 'failure',
              header: 'Failure',
              render: (r) => {
                const f = failureReasonLabel(r.failure_reason)
                return f ? (
                  <span className="block max-w-56 truncate text-xs text-critical" title={f}>
                    {f}
                  </span>
                ) : (
                  <span className="text-ink-faint">—</span>
                )
              },
            },
            {
              key: 'sync',
              header: 'Declarations',
              render: (r) =>
                r.declarations_in_sync ? (
                  <Badge tone="good" icon={<CheckCircle2 />}>
                    In sync
                  </Badge>
                ) : r.mdm_enrolled ? (
                  <Badge tone="neutral" icon={<CircleDashed />} title="The device hasn't fetched the current declaration set yet">
                    Waiting
                  </Badge>
                ) : (
                  <span className="text-ink-faint">—</span>
                ),
            },
            { key: 'last', header: 'Last status', align: 'right', render: (r) => <RelativeTime value={r.last_status_at} className="text-ink-muted" /> },
          ]}
        />
      </Card>

      <Card>
        <CardHeader title="What the states mean" />
        <CardBody>
          <StateLegend />
        </CardBody>
      </Card>
    </div>
  )
}

const actionLabel: Record<string, string> = { Allowed: 'user choice', AlwaysOn: 'always on', AlwaysOff: 'always off' }

export function settingsSummary(s: UpdateSettings | null | undefined): string[] {
  if (!s) return []
  const out: string[] = []
  const onOff = (v: boolean | undefined, label: string) => v !== undefined && out.push(`${label} ${v ? 'on' : 'off'}`)
  onOff(s.notifications, 'Notifications')
  if (s.allow_standard_user_os_updates !== undefined) out.push(s.allow_standard_user_os_updates ? 'Standard users can update' : 'Admins only')
  if (s.major_deferral_days) out.push(`Major deferred ${s.major_deferral_days}d`)
  if (s.minor_deferral_days) out.push(`Minor deferred ${s.minor_deferral_days}d`)
  if (s.system_deferral_days) out.push(`System deferred ${s.system_deferral_days}d`)
  if (s.auto_download) out.push(`Download: ${actionLabel[s.auto_download]}`)
  if (s.auto_install_os_updates) out.push(`OS updates: ${actionLabel[s.auto_install_os_updates]}`)
  if (s.auto_install_security_updates) out.push(`Security: ${actionLabel[s.auto_install_security_updates]}`)
  onOff(s.rapid_security_response, 'RSR')
  onOff(s.rapid_security_response_rollback, 'RSR rollback')
  return out
}

export function PlanKindBadge({ kind }: { kind: string }) {
  return kind === 'enforcement' ? (
    <Badge tone="accent" icon={<CalendarClock />}>
      Enforcement
    </Badge>
  ) : (
    <Badge icon={<SlidersHorizontal />}>Settings</Badge>
  )
}

function PlansTab({ above }: { above: ReactNode }) {
  const { isAdmin } = useAuth()
  const plans = useUpdatePlans()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [toggling, setToggling] = useState<string | null>(null)
  const nPlans = (n: number) => `${n} ${n === 1 ? 'plan' : 'plans'}`

  const setEnabled = async (p: UpdatePlan, enabled: boolean) => {
    await api.put(`/api/v1/updates/plans/${p.id}`, { ...p, enabled })
    await invalidate(qk.updatePlan(p.id))
  }
  const toggle = async (p: UpdatePlan, enabled: boolean) => {
    setToggling(p.id)
    try {
      await setEnabled(p, enabled)
      await invalidate(qk.updatePlans, qk.updatesOverview)
      toast.success(enabled ? 'Plan enabled' : 'Plan disabled', 'Affected Macs will be asked to sync within a minute.')
    } catch (e) {
      toast.error('Update failed', errorMessage(e))
    } finally {
      setToggling(null)
    }
  }

  return (
    <IndexPage<UpdatePlan>
      title={pageTitle}
      description={pageDescription}
      newTo="/updates/plans/new"
      newLabel="New plan"
      above={
        <>
          {above}
          <p className="mb-4 text-xs text-ink-muted">
            When several plans reach the same Mac, the enforcement plan with the <strong className="text-ink">highest target version</strong> wins (ties: earliest deadline), and the{' '}
            <strong className="text-ink">most recently updated</strong> settings plan wins.
          </p>
        </>
      }
      rows={plans.data?.items}
      loading={plans.isPending}
      error={plans.error}
      rowKey={(p) => p.id}
      rowHref={(p) => `/updates/plans/${p.id}`}
      rowClassName={(p) => (p.enabled ? undefined : 'opacity-70')}
      search={{ placeholder: 'Search plans…', text: (p) => `${p.name} ${p.target_os_version}` }}
      filters={[
        { key: 'state', label: 'Any state', options: [{ value: 'on', label: 'Enabled' }, { value: 'off', label: 'Disabled' }], match: (p, v) => p.enabled === (v === 'on') },
        { key: 'kind', label: 'Any kind', options: [{ value: 'enforcement', label: 'Enforcement' }, { value: 'settings', label: 'Settings' }], match: (p, v) => (v === 'enforcement' ? p.kind === 'enforcement' : p.kind !== 'enforcement') },
      ]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.updatePlans, qk.updatesOverview)}
      bulkActions={[
        { label: 'Enable', icon: <Power />, run: (p) => setEnabled(p, true), done: (n) => `${nPlans(n)} enabled` },
        { label: 'Disable', icon: <PowerOff />, run: (p) => setEnabled(p, false), done: (n) => `${nPlans(n)} disabled` },
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${nPlans(n)}?`, description: "The plans' declarations are removed from every Mac in scope at their next sync. Updates already installed are not affected." },
          run: (p) => api.del(`/api/v1/updates/plans/${p.id}`),
          done: (n) => `${nPlans(n)} deleted`,
        },
      ]}
      empty={{ title: 'No plans yet', description: 'Create an enforcement plan to require a macOS version by a deadline, or a settings plan to control update behavior.' }}
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
        { key: 'name', header: 'Name', sortValue: (p) => p.name, render: (p) => <span className="font-medium text-ink">{p.name}</span> },
        { key: 'kind', header: 'Kind', sortValue: (p) => p.kind, render: (p) => <PlanKindBadge kind={p.kind} /> },
        {
          key: 'detail',
          header: 'Target / settings',
          render: (p) => {
            if (p.kind === 'enforcement')
              return (
                <span className="text-sm whitespace-nowrap">
                  macOS <span className="tabular-nums text-ink">{p.target_os_version}</span>
                  {p.target_build_version && <span className="ml-1 font-mono text-xs text-ink-muted">({p.target_build_version})</span>}
                  <span className="text-ink-muted"> by {formatLocalDeadline(p.target_local_datetime)}</span>
                </span>
              )
            const s = settingsSummary(p.settings)
            return s.length ? (
              <span className="flex flex-wrap gap-1">
                {s.map((x) => (
                  <Badge key={x}>{x}</Badge>
                ))}
              </span>
            ) : (
              <span className="text-xs text-ink-faint">No settings set</span>
            )
          },
        },
        { key: 'scope', header: 'Scope', render: (p) => <ScopeSummary scope={p.scope} /> },
        { key: 'updated', header: 'Updated', sortValue: (p) => p.updated_at, render: (p) => <RelativeTime value={p.updated_at} className="text-ink-muted" /> },
      ]}
    />
  )
}

/** Inline list renderer for server validation errors (joined by newlines). */
export function ErrorList({ message }: { message: string }) {
  const lines = message.split('\n').filter(Boolean)
  if (lines.length <= 1) return <>{message}</>
  return (
    <ul className="list-disc pl-4 space-y-0.5">
      {lines.map((l) => (
        <li key={l}>{l}</li>
      ))}
    </ul>
  )
}

