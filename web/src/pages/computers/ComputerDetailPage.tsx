import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Laptop, RefreshCw, Trash2, Wifi, WifiOff } from 'lucide-react'
import { useComputer, qk, useInvalidate, useSettings } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { PageHeader } from '../../components/ui/PageHeader'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Tabs } from '../../components/ui/Tabs'
import { Card } from '../../components/ui/Card'
import { LoadingBlock } from '../../components/ui/Spinner'
import { Alert } from '../../components/ui/Alert'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { useToast } from '../../components/ui/Toast'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ComputerStatusBadges } from './ComputersPage'
import { latest } from '../DashboardPage'
import { GeneralTab, HardwareTab, OSTab, SecurityTab, AppsTab, ProfilesTab, EATab, LocalUsersTab, GroupsTab } from './InventoryTabs'
import { ComputerPolicyLogsTab, ComputerTasksTab, ComputerCommandsTab, ComputerTimelineTab } from './ActivityTabs'
import { AppInstallsTab, RestrictedEventsTab } from '../ActivityPage'
import { ManagementTab } from './ManagementTab'
import { ComputerLogsTab } from './LogsTab'
import { ComputerUpdatesTab } from './UpdatesTab'
import { ComputerComplianceTab } from './ComplianceTab'
import { ShellTerminal } from '../../components/shell/ShellTerminal'
import { TerminalSquare } from 'lucide-react'
import { ComputerHealthStrip } from '../../components/health/ComputerHealthStrip'
import type { UpdateInventoryResponse } from '../../api/types'

const tabKeys = ['general', 'hardware', 'os', 'security', 'applications', 'profiles', 'extension-attributes', 'local-users', 'groups', 'policy-logs', 'app-installs', 'restricted-events', 'tasks', 'commands', 'timeline', 'logs', 'updates', 'compliance', 'management', 'shell'] as const
type TabKey = (typeof tabKeys)[number]

export function ComputerDetailPage() {
  const { id = '', tab } = useParams()
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const settings = useSettings()
  const shellEnabled = !!settings.data?.remote_shell_enabled
  const toast = useToast()
  const invalidate = useInvalidate()
  const q = useComputer(id)
  const c = q.data
  let active: TabKey = (tabKeys as readonly string[]).includes(tab ?? '') ? (tab as TabKey) : 'general'
  if (active === 'shell' && !(shellEnabled && isAdmin && c?.agent_enrolled)) active = 'management'
  const setTab = (k: TabKey) => navigate(`/computers/${id}${k === 'general' ? '' : `/${k}`}`, { replace: true })
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [refreshing, setRefreshing] = useState(false)

  const tabs = useMemo(
    () => [
      { key: 'general' as TabKey, label: 'General' },
      { key: 'hardware' as TabKey, label: 'Hardware' },
      { key: 'os' as TabKey, label: 'Operating System' },
      { key: 'security' as TabKey, label: 'Security' },
      { key: 'applications' as TabKey, label: 'Applications', count: c?.apps?.length },
      { key: 'profiles' as TabKey, label: 'Profiles', count: c?.profiles?.length },
      { key: 'extension-attributes' as TabKey, label: 'Extension Attributes', count: c?.extension_attributes?.length },
      { key: 'local-users' as TabKey, label: 'Local Users', count: c?.local_users?.length },
      { key: 'groups' as TabKey, label: 'Groups', count: c?.groups?.length },
      { key: 'policy-logs' as TabKey, label: 'Policy Logs' },
      { key: 'app-installs' as TabKey, label: 'App Installs' },
      { key: 'restricted-events' as TabKey, label: 'Restricted Events' },
      { key: 'tasks' as TabKey, label: 'Tasks' },
      { key: 'commands' as TabKey, label: 'MDM Commands' },
      { key: 'timeline' as TabKey, label: 'Timeline' },
      { key: 'logs' as TabKey, label: 'Logs' },
      { key: 'updates' as TabKey, label: 'Updates' },
      { key: 'compliance' as TabKey, label: 'Compliance' },
      { key: 'management' as TabKey, label: 'Management' },
      ...(shellEnabled && isAdmin && c?.agent_enrolled ? [{ key: 'shell' as TabKey, label: 'Shell', icon: <TerminalSquare /> }] : []),
    ],
    [c, shellEnabled, isAdmin],
  )

  const updateInventory = async () => {
    setRefreshing(true)
    try {
      const r = await api.post<UpdateInventoryResponse>(`/api/v1/computers/${id}/update-inventory`)
      const problems = [r.mdm_error && `MDM: ${r.mdm_error}`, r.agent_error && `Agent: ${r.agent_error}`].filter(Boolean)
      if (problems.length === 2) toast.error('Inventory update could not be requested', problems.join(' · '))
      else if (problems.length === 1) toast.warning('Inventory update partially requested', problems[0] as string)
      else toast.success('Inventory update requested')
      await invalidate(qk.computer(id), qk.computerCommands(id), qk.computerTasks(id))
    } catch (e) {
      toast.error('Inventory update failed', errorMessage(e))
    } finally {
      setRefreshing(false)
    }
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/computers/${id}`)
      toast.success('Computer deleted')
      await invalidate(['computers'], qk.dashboard, qk.groups)
      navigate('/computers')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (q.isPending) return <LoadingBlock />
  if (q.error || !c) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'Computer not found'}</Alert>

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Computers', to: '/computers' }, { label: c.name || c.serial_number }]}
        title={
          <span className="inline-flex items-center gap-3">
            <Laptop className="size-6 text-ink-faint" />
            {c.name || c.hostname || c.serial_number || 'Unnamed computer'}
          </span>
        }
        meta={
          <>
            <ComputerStatusBadges c={c} />
            {c.agent_enrolled && (
              <Badge tone={c.agent_online ? 'good' : 'neutral'} icon={c.agent_online ? <Wifi /> : <WifiOff />}>
                {c.agent_online ? 'Agent online' : 'Agent offline'}
              </Badge>
            )}
            <span className="text-xs text-ink-muted">
              {c.model_name || c.model} · macOS {c.os_version || '?'} · <span className="font-mono">{c.serial_number}</span>
            </span>
            <span className="text-xs text-ink-muted">
              Last check-in <RelativeTime value={latest(c.agent_last_checkin, c.mdm_last_checkin)} />
            </span>
          </>
        }
        actions={
          isAdmin && (
            <>
              <Button icon={<RefreshCw />} loading={refreshing} onClick={updateInventory}>
                Update inventory
              </Button>
              <Button variant="danger-outline" icon={<Trash2 />} onClick={() => setDeleteOpen(true)}>
                Delete
              </Button>
            </>
          )
        }
      />
      <ComputerHealthStrip computerId={c.id} className="mb-5" />
      <Tabs items={tabs} value={active} onChange={setTab} className="mb-5" />
      <Card>
        {active === 'general' && <GeneralTab c={c} />}
        {active === 'hardware' && <HardwareTab c={c} />}
        {active === 'os' && <OSTab c={c} />}
        {active === 'security' && <SecurityTab c={c} />}
        {active === 'applications' && <AppsTab c={c} />}
        {active === 'profiles' && <ProfilesTab c={c} />}
        {active === 'extension-attributes' && <EATab c={c} />}
        {active === 'local-users' && <LocalUsersTab c={c} />}
        {active === 'groups' && <GroupsTab c={c} />}
        {active === 'policy-logs' && <ComputerPolicyLogsTab id={c.id} />}
        {active === 'app-installs' && <AppInstallsTab computerId={c.id} />}
        {active === 'restricted-events' && <RestrictedEventsTab computerId={c.id} />}
        {active === 'tasks' && <ComputerTasksTab id={c.id} />}
        {active === 'commands' && <ComputerCommandsTab id={c.id} />}
        {active === 'timeline' && <ComputerTimelineTab id={c.id} />}
        {active === 'logs' && <ComputerLogsTab c={c} />}
        {active === 'updates' && <ComputerUpdatesTab id={c.id} />}
        {active === 'compliance' && <ComputerComplianceTab id={c.id} />}
        {active === 'management' && <ManagementTab c={c} />}
        {active === 'shell' && (
          <div className="p-4">
            <ShellTerminal computerId={c.id} computerName={c.name || c.hostname || c.serial_number || 'mac'} />
          </div>
        )}
      </Card>
      <ConfirmDialog
        open={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        onConfirm={doDelete}
        destructive
        title="Delete this computer record?"
        description="This removes the inventory record and history from VibeMDM. It does not unenroll the device; use Remove MDM Profile first if you want to unmanage it."
        confirmLabel="Delete computer"
        typeToConfirm={c.name || c.serial_number}
      />
    </>
  )
}
