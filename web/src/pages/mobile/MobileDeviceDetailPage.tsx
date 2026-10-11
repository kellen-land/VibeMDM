import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Pencil, RefreshCw, Save, Trash2, X } from 'lucide-react'
import { qk, useBuildings, useDepartments, useInvalidate, useMobileDevice } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { MobileDeviceDetail } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge, BoolBadge } from '../../components/ui/Badge'
import { Alert } from '../../components/ui/Alert'
import { Tabs } from '../../components/ui/Tabs'
import { SectionNav } from '../../components/ui/SectionNav'
import { DescriptionList } from '../../components/ui/DescriptionList'
import { Table } from '../../components/ui/Table'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select, Textarea } from '../../components/ui/Input'
import { LoadingBlock } from '../../components/ui/Spinner'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'
import { absoluteTime } from '../../lib/format'
import { ComputerCommandsTab } from '../computers/ActivityTabs'
import { DeviceIcon, MobileStatusBadges } from './MobileDevicesPage'

type Section = 'general' | 'hardware' | 'network' | 'security' | 'apps' | 'profiles' | 'groups' | 'actions' | 'commands'
const sectionKeys: readonly string[] = ['general', 'hardware', 'network', 'security', 'apps', 'profiles', 'groups', 'actions', 'commands']

/** A mobile device record, laid out like a computer's: Inventory, Management and History. */
export function MobileDeviceDetailPage() {
  const { id = '', tab } = useParams()
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const q = useMobileDevice(id)
  const d = q.data
  const active = (sectionKeys.includes(tab ?? '') ? tab : 'general') as Section
  const setSection = (k: string) => navigate(`/mobile-devices/${id}${k === 'general' ? '' : `/${k}`}`, { replace: true })
  const [refreshing, setRefreshing] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const sections = useMemo(
    () => ({
      inventory: [
        {
          items: [
            { key: 'general', label: 'General' },
            { key: 'hardware', label: 'Hardware' },
            { key: 'network', label: 'Network' },
            { key: 'security', label: 'Security' },
            { key: 'apps', label: 'Apps', count: d?.apps.length },
            { key: 'profiles', label: 'Profiles', count: d?.profiles.length },
            { key: 'groups', label: 'Groups', count: d?.groups.length },
          ],
        },
      ],
      management: [{ items: [{ key: 'actions', label: 'Actions' }] }],
      history: [{ items: [{ key: 'commands', label: 'MDM commands' }] }],
    }),
    [d],
  )
  type Area = keyof typeof sections
  const area: Area = (Object.keys(sections) as Area[]).find((k) => sections[k][0].items.some((i) => i.key === active)) ?? 'inventory'

  const updateInventory = async () => {
    setRefreshing(true)
    try {
      await api.post(`/api/v1/mobile-devices/${id}/update-inventory`)
      toast.success('Inventory update requested', 'The device answers when it next checks in with MDM.')
      await invalidate(qk.mobileDevice(id))
    } catch (e) {
      toast.error('Inventory update failed', errorMessage(e))
    } finally {
      setRefreshing(false)
    }
  }
  const doDelete = async () => {
    try {
      await api.del(`/api/v1/mobile-devices/${id}`)
      toast.success('Mobile device record deleted')
      await invalidate(['mobile-devices'])
      navigate('/mobile-devices')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (q.isPending) return <LoadingBlock />
  if (q.error || !d) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'Not found'}</Alert>

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Mobile Devices', to: '/mobile-devices' }, { label: d.name || d.serial_number }]}
        title={
          <span className="inline-flex items-center gap-3">
            <DeviceIcon d={d} className="size-6 text-ink-faint" />
            {d.name || d.serial_number || 'Unnamed device'}
          </span>
        }
        meta={
          <>
            <MobileStatusBadges d={d} />
            <span className="text-xs text-ink-muted">
              {d.model_name || d.model} · {d.device_type === 'iPad' ? 'iPadOS' : 'iOS'} {d.os_version || '?'} · <span className="font-mono">{d.serial_number}</span>
            </span>
            <span className="text-xs text-ink-muted">
              Last check-in <RelativeTime value={d.mdm_last_checkin} />
            </span>
          </>
        }
        actions={
          isAdmin && (
            <Button icon={<RefreshCw />} loading={refreshing} onClick={updateInventory}>
              Update inventory
            </Button>
          )
        }
      />
      <Tabs
        items={[
          { key: 'inventory', label: 'Inventory' },
          { key: 'management', label: 'Management' },
          { key: 'history', label: 'History' },
        ]}
        value={area}
        onChange={(k) => setSection(sections[k as Area][0].items[0].key)}
        className="mb-5"
      />
      <div className="flex flex-col gap-6 lg:flex-row">
        <SectionNav label={`${area} sections`} groups={sections[area]} value={active} onChange={setSection} />
        <Card className="min-w-0 flex-1">
          {active === 'general' && <GeneralSection d={d} />}
          {active === 'hardware' && <HardwareSection d={d} />}
          {active === 'network' && <NetworkSection d={d} />}
          {active === 'security' && <SecuritySection d={d} />}
          {active === 'apps' && <AppsSection d={d} />}
          {active === 'profiles' && <ProfilesSection d={d} />}
          {active === 'groups' && <GroupsSection d={d} />}
          {active === 'actions' && <ActionsSection d={d} onDelete={() => setDeleteOpen(true)} />}
          {active === 'commands' && <ComputerCommandsTab id={d.id} mobile />}
        </Card>
      </div>
      <ConfirmDialog
        open={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        onConfirm={doDelete}
        destructive
        title="Delete this mobile device record?"
        description="This removes the record from VibeMDM only. The device stays enrolled in MDM and is added again when it next checks in; to unenroll it, remove the management profile on the device or erase it."
        confirmLabel="Delete record"
      />
    </>
  )
}

const gb = (n: number | null) => (n === null ? '' : `${Math.round(n * 10) / 10} GB`)

const editable = [
  { key: 'asset_tag', label: 'Asset tag' },
  { key: 'assigned_user', label: 'Assigned username' },
  { key: 'assigned_real_name', label: 'Assigned full name' },
  { key: 'assigned_email', label: 'Assigned email' },
  { key: 'department', label: 'Department', list: 'department' },
  { key: 'building', label: 'Building', list: 'building' },
  { key: 'room', label: 'Room' },
  { key: 'site', label: 'Site' },
] as const
type EditKey = (typeof editable)[number]['key'] | 'notes'

function GeneralSection({ d }: { d: MobileDeviceDetail }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<Record<EditKey, string>>({} as Record<EditKey, string>)
  const [saving, setSaving] = useState(false)
  const buildings = useBuildings({ enabled: editing })
  const departments = useDepartments({ enabled: editing })
  useEffect(() => {
    const f = { notes: d.notes } as Record<EditKey, string>
    for (const e of editable) f[e.key] = d[e.key] ?? ''
    setForm(f)
  }, [d])
  const options = (list: 'building' | 'department', current: string) => {
    const names = ((list === 'building' ? buildings.data : departments.data)?.items ?? []).map((i) => i.name)
    if (current && !names.includes(current)) names.unshift(current)
    return [{ value: '', label: '— None —' }, ...names.map((n) => ({ value: n, label: n }))]
  }
  const save = async () => {
    setSaving(true)
    try {
      const patch: Partial<Record<EditKey, string>> = {}
      for (const k of [...editable.map((e) => e.key), 'notes'] as EditKey[]) if ((form[k] ?? '') !== (d[k] ?? '')) patch[k] = form[k]
      if (Object.keys(patch).length > 0) {
        await api.patch(`/api/v1/mobile-devices/${d.id}`, patch)
        await invalidate(qk.mobileDevice(d.id), ['mobile-devices'])
        toast.success('Mobile device updated')
      }
      setEditing(false)
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }
  return (
    <>
      <CardHeader title="Identity" />
      <CardBody>
        <DescriptionList
          cols={3}
          items={[
            { label: 'Serial number', value: d.serial_number, mono: true },
            { label: 'UDID', value: d.udid, mono: true },
            { label: 'Device type', value: d.device_type },
            { label: 'Model', value: d.model_name },
            { label: 'Model identifier', value: d.model, mono: true },
            { label: d.device_type === 'iPad' ? 'iPadOS' : 'iOS', value: d.os_build ? `${d.os_version} (${d.os_build})` : d.os_version },
          ]}
        />
      </CardBody>
      <CardHeader title="Management" className="border-t border-line" />
      <CardBody>
        <DescriptionList
          cols={3}
          items={[
            { label: 'MDM', value: <BoolBadge value={d.mdm_enrolled} yes="Managed" no="Not managed" /> },
            { label: 'Supervised', value: <BoolBadge value={d.is_supervised} /> },
            { label: 'Shared iPad', value: d.device_type === 'iPad' && d.is_shared_ipad !== null ? (d.is_shared_ipad ? 'Yes' : 'No') : '' },
            { label: 'Enrolled', value: d.mdm_enrolled_at ? absoluteTime(d.mdm_enrolled_at) : '' },
            { label: 'Last check-in', value: d.mdm_last_checkin ? <RelativeTime value={d.mdm_last_checkin} /> : '' },
            { label: 'Last inventory', value: d.last_inventory_at ? <RelativeTime value={d.last_inventory_at} /> : '' },
          ]}
        />
      </CardBody>
      <CardHeader
        title="User and location"
        className="border-t border-line"
        actions={
          isAdmin &&
          (editing ? (
            <>
              <Button size="sm" variant="ghost" icon={<X />} onClick={() => setEditing(false)}>
                Cancel
              </Button>
              <Button size="sm" variant="primary" icon={<Save />} loading={saving} onClick={save}>
                Save
              </Button>
            </>
          ) : (
            <Button size="sm" icon={<Pencil />} onClick={() => setEditing(true)}>
              Edit
            </Button>
          ))
        }
      />
      <CardBody>
        {editing ? (
          <FormGrid cols={3}>
            {editable.map((e) => (
              <Field key={e.key} label={e.label}>
                {'list' in e ? (
                  <Select value={form[e.key]} onChange={(ev) => setForm({ ...form, [e.key]: ev.target.value })} options={options(e.list, form[e.key])} />
                ) : (
                  <Input value={form[e.key]} onChange={(ev) => setForm({ ...form, [e.key]: ev.target.value })} />
                )}
              </Field>
            ))}
            <Field label="Notes" className="md:col-span-3">
              <Textarea value={form.notes} onChange={(ev) => setForm({ ...form, notes: ev.target.value })} rows={3} />
            </Field>
          </FormGrid>
        ) : (
          <DescriptionList cols={3} items={[...editable.map((e) => ({ label: e.label, value: d[e.key] })), { label: 'Notes', value: d.notes, span: true }]} />
        )}
      </CardBody>
    </>
  )
}

function HardwareSection({ d }: { d: MobileDeviceDetail }) {
  return (
    <>
      <CardHeader title="Hardware" />
      <CardBody>
        <DescriptionList
          cols={3}
          items={[
            { label: 'Model', value: d.model_name },
            { label: 'Capacity', value: gb(d.capacity_gb) },
            { label: 'Available', value: gb(d.available_gb) },
            { label: 'Battery', value: d.battery_level === null ? '' : `${Math.round(d.battery_level * 100)}%` },
            { label: 'Wi-Fi MAC address', value: d.wifi_mac, mono: true },
            { label: 'Bluetooth MAC address', value: d.bluetooth_mac, mono: true },
          ]}
        />
      </CardBody>
    </>
  )
}

function NetworkSection({ d }: { d: MobileDeviceDetail }) {
  const cellular = d.imei || d.meid || d.phone_number
  return (
    <>
      <CardHeader title="Network" />
      <CardBody>
        {cellular ? (
          <DescriptionList
            cols={3}
            items={[
              { label: 'Phone number', value: d.phone_number },
              { label: 'Carrier', value: d.carrier },
              { label: 'Cellular technology', value: d.cellular_technology },
              { label: 'IMEI', value: d.imei, mono: true },
              { label: 'MEID', value: d.meid, mono: true },
              { label: 'Roaming', value: d.is_roaming === null ? '' : <BoolBadge value={d.is_roaming} /> },
            ]}
          />
        ) : (
          <p className="text-sm text-ink-muted">No cellular details: this is a Wi-Fi-only device, or inventory has not arrived yet.</p>
        )}
      </CardBody>
    </>
  )
}

function SecuritySection({ d }: { d: MobileDeviceDetail }) {
  return (
    <>
      <CardHeader title="Security" />
      <CardBody>
        <DescriptionList
          cols={3}
          items={[
            { label: 'Passcode set', value: <BoolBadge value={d.passcode_present} /> },
            { label: 'Passcode compliant', value: <BoolBadge value={d.passcode_compliant} /> },
            { label: 'Data protection', value: d.hardware_encryption === null ? '' : d.hardware_encryption >= 2 ? 'Block and file level' : d.hardware_encryption === 1 ? 'Block level' : 'None' },
            { label: 'Supervised', value: <BoolBadge value={d.is_supervised} /> },
            { label: 'Activation Lock', value: <BoolBadge value={d.activation_lock_enabled} yes="On" no="Off" /> },
            { label: 'Find My', value: <BoolBadge value={d.find_my_enabled} yes="On" no="Off" /> },
            { label: 'Lost Mode', value: d.lost_mode_enabled ? <Badge tone="warning" dot>On</Badge> : <BoolBadge value={d.lost_mode_enabled} yes="On" no="Off" /> },
            { label: 'Last iCloud backup', value: d.last_backup_at ? <RelativeTime value={d.last_backup_at} /> : '' },
          ]}
        />
      </CardBody>
    </>
  )
}

function AppsSection({ d }: { d: MobileDeviceDetail }) {
  return (
    <>
      <CardHeader title="Apps" description="From the device's installed and managed app lists." />
      <Table
        rows={d.apps}
        rowKey={(a) => a.bundle_id + a.name}
        dense
        emptyTitle="No apps reported yet"
        columns={[
          { key: 'name', header: 'Name', render: (a) => <span className="font-medium text-ink">{a.name || a.bundle_id}</span> },
          { key: 'version', header: 'Version', render: (a) => <span className="tabular-nums text-ink-muted">{a.version || '—'}</span> },
          { key: 'bundle', header: 'Identifier', render: (a) => <span className="font-mono text-xs text-ink-muted">{a.bundle_id}</span> },
          { key: 'managed', header: 'Managed', render: (a) => (a.managed ? <Badge tone="accent">{a.managed_status || 'Managed'}</Badge> : <span className="text-ink-faint">—</span>) },
        ]}
      />
    </>
  )
}

function ProfilesSection({ d }: { d: MobileDeviceDetail }) {
  return (
    <>
      <CardHeader title="Profiles" description="Configuration profiles installed on the device." />
      <Table
        rows={d.profiles}
        rowKey={(p) => p.identifier + p.uuid}
        dense
        emptyTitle="No profiles reported yet"
        columns={[
          { key: 'name', header: 'Name', render: (p) => <span className="font-medium text-ink">{p.display_name || p.identifier}</span> },
          { key: 'id', header: 'Identifier', render: (p) => <span className="font-mono text-xs text-ink-muted">{p.identifier}</span> },
          { key: 'org', header: 'Organization', render: (p) => <span className="text-ink-muted">{p.organization || '—'}</span> },
          { key: 'managed', header: 'Managed', render: (p) => <BoolBadge value={p.is_managed} /> },
        ]}
      />
    </>
  )
}

function GroupsSection({ d }: { d: MobileDeviceDetail }) {
  const navigate = useNavigate()
  return (
    <>
      <CardHeader title="Groups" />
      <Table
        rows={d.groups}
        rowKey={(g) => g.id}
        dense
        onRowClick={(g) => navigate(`/mobile-groups/${g.id}`)}
        emptyTitle="Not a member of any mobile device group"
        columns={[
          { key: 'name', header: 'Name', render: (g) => <span className="font-medium text-ink">{g.name}</span> },
          { key: 'type', header: 'Type', render: (g) => <Badge tone={g.is_smart ? 'accent' : 'neutral'}>{g.is_smart ? 'Smart' : 'Static'}</Badge> },
        ]}
      />
    </>
  )
}

function ActionsSection({ d, onDelete }: { d: MobileDeviceDetail; onDelete: () => void }) {
  const { isAdmin } = useAuth()
  return (
    <>
      <CardHeader title="Actions" description="Remote commands for this device." />
      <CardBody className="flex flex-col gap-4">
        <p className="text-sm text-ink-muted">
          {d.mdm_enrolled ? 'Lock, Lost Mode, erase and the other iPhone and iPad commands arrive in a later update.' : 'This device is not enrolled in MDM, so no commands can be sent.'}
        </p>
        {isAdmin && (
          <div>
            <Button variant="danger-outline" icon={<Trash2 />} onClick={onDelete}>
              Delete record
            </Button>
          </div>
        )}
      </CardBody>
    </>
  )
}
