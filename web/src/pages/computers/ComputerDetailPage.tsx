import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Download, Laptop, RefreshCw, Wifi, WifiOff } from 'lucide-react'
import { useComputer, qk, useInvalidate, useSettings } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { PageHeader } from '../../components/ui/PageHeader'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Tabs } from '../../components/ui/Tabs'
import { SectionNav } from '../../components/ui/SectionNav'
import { Card } from '../../components/ui/Card'
import { LoadingBlock } from '../../components/ui/Spinner'
import { Alert } from '../../components/ui/Alert'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { ObjectBar } from '../../components/editor/ObjectBar'
import { useEditMode } from '../../components/editor/useEditMode'
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
import { ComputerOnboardingTab } from './OnboardingTab'
import { ShellTerminal } from '../../components/shell/ShellTerminal'
import { TerminalSquare } from 'lucide-react'
import { ComputerHealthStrip } from '../../components/health/ComputerHealthStrip'
import type { UpdateInventoryResponse } from '../../api/types'

const tabKeys = ['general', 'hardware', 'os', 'security', 'applications', 'profiles', 'extension-attributes', 'local-users', 'groups', 'policy-logs', 'app-installs', 'restricted-events', 'tasks', 'commands', 'timeline', 'logs', 'updates', 'compliance', 'onboarding', 'management', 'shell'] as const
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

  // Jamf-style record: three top tabs, each with a short list of sections.
  const sections = useMemo(
    () => ({
      inventory: [
        {
          items: [
            { key: 'general', label: 'General' },
            { key: 'hardware', label: 'Hardware' },
            { key: 'os', label: 'Operating system' },
            { key: 'security', label: 'Security' },
            { key: 'applications', label: 'Applications', count: c?.apps?.length },
            { key: 'profiles', label: 'Profiles', count: c?.profiles?.length },
            { key: 'extension-attributes', label: 'Extension attributes', count: c?.extension_attributes?.length },
            { key: 'local-users', label: 'Local users', count: c?.local_users?.length },
            { key: 'groups', label: 'Groups', count: c?.groups?.length },
          ],
        },
      ],
      management: [
        {
          items: [
            { key: 'management', label: 'Actions' },
            { key: 'updates', label: 'Software updates' },
            { key: 'compliance', label: 'Compliance' },
            { key: 'onboarding', label: 'Onboarding' },
            ...(shellEnabled && isAdmin && c?.agent_enrolled ? [{ key: 'shell', label: 'Shell', icon: <TerminalSquare /> }] : []),
          ],
        },
      ],
      history: [
        {
          items: [
            { key: 'timeline', label: 'Timeline' },
            { key: 'policy-logs', label: 'Policy logs' },
            { key: 'app-installs', label: 'App installs' },
            { key: 'tasks', label: 'Agent tasks' },
            { key: 'commands', label: 'MDM commands' },
            { key: 'restricted-events', label: 'Restricted software' },
            { key: 'logs', label: 'Agent logs' },
          ],
        },
      ],
    }),
    [c, shellEnabled, isAdmin],
  )
  type Area = keyof typeof sections
  const area: Area = (Object.keys(sections) as Area[]).find((k) => sections[k][0].items.some((i) => i.key === active)) ?? 'inventory'

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

  const [installing, setInstalling] = useState(false)
  const installAgent = async () => {
    setInstalling(true)
    try {
      await api.post(`/api/v1/computers/${id}/agent-install`)
      toast.success('Agent install sent', 'The Mac installs the agent package and enrolls it in a minute or two.')
      await invalidate(qk.computer(id), qk.computerCommands(id))
    } catch (e) {
      toast.error('Could not install the agent', errorMessage(e))
    } finally {
      setInstalling(false)
    }
  }

  const ed = useEditMode({ isNew: false, canEdit: false, snapshot: null, reset: () => {} })
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
              {c.mdm_enrolled && !c.agent_enrolled && (
                <Button icon={<Download />} loading={installing} onClick={installAgent}>
                  Install agent
                </Button>
              )}
              <Button icon={<RefreshCw />} loading={refreshing} onClick={updateInventory}>
                Update inventory
              </Button>
            </>
          )
        }
      />
      <ComputerHealthStrip computerId={c.id} className="mb-5" />
      <Tabs
        items={[
          { key: 'inventory', label: 'Inventory' },
          { key: 'management', label: 'Management' },
          { key: 'history', label: 'History' },
        ]}
        value={area}
        onChange={(k) => setTab(sections[k as Area][0].items[0].key as TabKey)}
        className="mb-5"
      />
      <div className="flex flex-col gap-6 lg:flex-row">
      <SectionNav label={`${area} sections`} groups={sections[area]} value={active} onChange={(k) => setTab(k as TabKey)} />
      <Card className="min-w-0 flex-1">
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
        {active === 'onboarding' && <ComputerOnboardingTab c={c} />}
        {active === 'management' && <ManagementTab c={c} />}
        {active === 'shell' && (
          <div className="p-4">
            <ShellTerminal computerId={c.id} computerName={c.name || c.hostname || c.serial_number || 'mac'} />
          </div>
        )}
      </Card>
      </div>
      <ObjectBar ed={ed} isNew={false} listPath="/computers" noun="computer" name={c.name} onSave={async () => true} noEdit onDeleteRequest={() => setDeleteOpen(true)} historyId={c.id} />
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
