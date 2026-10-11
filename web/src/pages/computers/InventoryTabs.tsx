import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Pencil, Save, X } from 'lucide-react'
import type { ComputerApp, ComputerDetail, ComputerEdit, ComputerProfile, EAValue, LocalUser, ProfileDeployment } from '../../api/types'
import { api, errorMessage } from '../../api/client'
import { qk, useBuildings, useDepartments, useExtensionAttributes, useInvalidate } from '../../api/queries'
import { useAuth } from '../../auth/AuthContext'
import { CardBody, CardHeader } from '../../components/ui/Card'
import { DescriptionList, KeyValueTable } from '../../components/ui/DescriptionList'
import { Button } from '../../components/ui/Button'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select, Textarea } from '../../components/ui/Input'
import { Badge, BoolBadge, StatusBadge } from '../../components/ui/Badge'
import { Table } from '../../components/ui/Table'
import { SearchInput } from '../../components/ui/SearchInput'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'
import { FileVaultPanel } from './FileVaultPanel'
import { LapsPanel } from './LapsPanel'
import { absoluteTime, dash, formatDuration, formatMB } from '../../lib/format'

const editableFields: { key: keyof ComputerEdit; label: string; textarea?: boolean; list?: 'building' | 'department' }[] = [
  { key: 'name', label: 'Computer name' },
  { key: 'asset_tag', label: 'Asset tag' },
  { key: 'site', label: 'Site' },
  { key: 'department', label: 'Department', list: 'department' },
  { key: 'building', label: 'Building', list: 'building' },
  { key: 'room', label: 'Room' },
  { key: 'assigned_user', label: 'Assigned username' },
  { key: 'assigned_real_name', label: 'Assigned full name' },
  { key: 'assigned_email', label: 'Assigned email' },
  { key: 'assigned_phone', label: 'Assigned phone' },
  { key: 'assigned_position', label: 'Position' },
  { key: 'notes', label: 'Notes', textarea: true },
]

export function GeneralTab({ c }: { c: ComputerDetail }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<ComputerEdit>({})
  const [saving, setSaving] = useState(false)
  const buildings = useBuildings({ enabled: editing })
  const departments = useDepartments({ enabled: editing })
  /** Options from Settings → Buildings and networks, always including the computer's current value so nothing is lost. */
  const listOptions = (list: 'building' | 'department', current: string) => {
    const names = ((list === 'building' ? buildings.data : departments.data)?.items ?? []).map((i) => i.name)
    if (current && !names.includes(current)) names.unshift(current)
    return [{ value: '', label: '— None —' }, ...names.map((n) => ({ value: n, label: n }))]
  }
  useEffect(() => {
    const f: ComputerEdit = {}
    for (const e of editableFields) f[e.key] = c[e.key] ?? ''
    setForm(f)
  }, [c])

  const save = async () => {
    setSaving(true)
    try {
      const patch: ComputerEdit = {}
      for (const e of editableFields) if ((form[e.key] ?? '') !== (c[e.key] ?? '')) patch[e.key] = form[e.key] ?? ''
      if (Object.keys(patch).length > 0) {
        await api.patch(`/api/v1/computers/${c.id}`, patch)
        await invalidate(qk.computer(c.id), ['computers'])
        toast.success('Computer updated')
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
      <CardHeader
        title="Identity"
        description="Device identifiers reported by MDM and the agent."
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
        <DescriptionList
          cols={3}
          items={[
            { label: 'Serial number', value: c.serial_number, mono: true },
            { label: 'UDID', value: c.udid, mono: true },
            { label: 'Hostname', value: c.hostname },
            { label: 'Local hostname', value: c.local_hostname },
            { label: 'IP address', value: c.ip_address, mono: true },
            { label: 'MAC address', value: c.mac_address, mono: true },
            { label: 'Last user', value: c.last_user_full_name ? `${c.last_user_full_name} (${c.last_user})` : c.last_user },
            { label: 'Agent version', value: c.agent_version },
            { label: 'Record created', value: absoluteTime(c.created_at) },
          ]}
        />
      </CardBody>
      <CardHeader title="Management status" className="border-t border-line" />
      <CardBody>
        <DescriptionList
          cols={3}
          items={[
            { label: 'MDM enrolled', value: <BoolBadge value={c.mdm_enrolled} yes="Enrolled" no="Not enrolled" /> },
            { label: 'MDM enrolled at', value: c.mdm_enrolled_at ? absoluteTime(c.mdm_enrolled_at) : '—' },
            { label: 'MDM last check-in', value: <RelativeTime value={c.mdm_last_checkin} /> },
            { label: 'Agent installed', value: <BoolBadge value={c.agent_enrolled} yes="Installed" no="Not installed" /> },
            { label: 'Agent enrolled at', value: c.agent_enrolled_at ? absoluteTime(c.agent_enrolled_at) : '—' },
            { label: 'Agent last check-in', value: <RelativeTime value={c.agent_last_checkin} /> },
            { label: 'Last inventory', value: <RelativeTime value={c.last_inventory_at} /> },
            { label: 'MDM profile identifier', value: c.mdm_profile_identifier, mono: true },
            { label: 'MDM unenrolled at', value: c.mdm_unenrolled_at ? absoluteTime(c.mdm_unenrolled_at) : '—' },
          ]}
        />
      </CardBody>
      <CardHeader title="User and location" description="Editable fields for asset tracking." className="border-t border-line" />
      <CardBody>
        {editing ? (
          <FormGrid cols={3}>
            {editableFields.map((e) => (
              <Field key={e.key} label={e.label} className={e.textarea ? 'md:col-span-3' : undefined} hint={e.list ? 'Managed under Settings → Buildings and networks.' : undefined}>
                {e.textarea ? (
                  <Textarea value={form[e.key] ?? ''} onChange={(ev) => setForm({ ...form, [e.key]: ev.target.value })} rows={3} />
                ) : e.list ? (
                  <Select value={form[e.key] ?? ''} onChange={(ev) => setForm({ ...form, [e.key]: ev.target.value })} options={listOptions(e.list, form[e.key] ?? '')} />
                ) : (
                  <Input value={form[e.key] ?? ''} onChange={(ev) => setForm({ ...form, [e.key]: ev.target.value })} />
                )}
              </Field>
            ))}
          </FormGrid>
        ) : (
          <DescriptionList cols={3} items={editableFields.map((e) => ({ label: e.label, value: e.textarea ? <span className="whitespace-pre-wrap">{c[e.key]}</span> : c[e.key], span: e.textarea }))} />
        )}
      </CardBody>
    </>
  )
}

export function HardwareTab({ c }: { c: ComputerDetail }) {
  const diskUsed = c.disk_total_mb != null && c.disk_free_mb != null ? c.disk_total_mb - c.disk_free_mb : null
  const diskPct = diskUsed != null && c.disk_total_mb ? Math.round((diskUsed / c.disk_total_mb) * 100) : null
  return (
    <>
      <CardHeader title="Hardware" />
      <CardBody>
        <DescriptionList
          cols={3}
          items={[
            { label: 'Model', value: c.model_name || c.model },
            { label: 'Model identifier', value: c.model_identifier, mono: true },
            { label: 'Product name', value: c.product_name },
            { label: 'Processor', value: c.cpu },
            { label: 'Architecture', value: c.architecture },
            { label: 'Cores', value: dash(c.cpu_cores) },
            { label: 'Memory', value: formatMB(c.memory_mb) },
            {
              label: 'Boot volume',
              value:
                c.disk_total_mb != null ? (
                  <span>
                    {formatMB(diskUsed)} used of {formatMB(c.disk_total_mb)} {diskPct != null && <span className="text-ink-muted">({diskPct}%)</span>}
                    {diskPct != null && (
                      <span className="mt-1 block h-1.5 w-40 overflow-hidden rounded-full bg-accent-100 dark:bg-accent-950">
                        <span className={diskPct > 90 ? 'block h-full bg-critical' : 'block h-full bg-accent-600'} style={{ width: `${diskPct}%` }} />
                      </span>
                    )}
                  </span>
                ) : (
                  '—'
                ),
            },
            { label: 'Available space', value: formatMB(c.disk_free_mb) },
            { label: 'Battery', value: c.battery_percent != null ? `${c.battery_percent}%` : '—' },
            { label: 'Uptime', value: formatDuration(c.uptime_seconds) },
          ]}
        />
      </CardBody>
      <CardHeader title="Raw hardware report" description="All key/values reported by the agent." className="border-t border-line" />
      <CardBody>
        <KeyValueTable data={c.hardware} />
      </CardBody>
    </>
  )
}

export function OSTab({ c }: { c: ComputerDetail }) {
  return (
    <>
      <CardHeader title="Operating system" />
      <CardBody>
        <DescriptionList
          cols={3}
          items={[
            { label: 'macOS version', value: c.os_version },
            { label: 'Build', value: c.os_build, mono: true },
            { label: 'Architecture', value: c.architecture },
            { label: 'Supervised', value: <BoolBadge value={c.is_supervised} /> },
            { label: 'Last user', value: c.last_user },
            { label: 'Uptime', value: formatDuration(c.uptime_seconds) },
          ]}
        />
      </CardBody>
      <CardHeader title="MDM device information" description="Latest DeviceInformation response." className="border-t border-line" />
      <CardBody>
        <KeyValueTable data={c.mdm_device_info} />
      </CardBody>
    </>
  )
}

/** "not supported" → "Not supported" */
function titleCaseWords(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

export function SecurityTab({ c }: { c: ComputerDetail }) {
  // Whether the Secure Enclave lets the bootstrap token authorize updates and kernel extensions. Not escrow.
  const bootstrapAuth = typeof c.security_info?.BootstrapTokenAllowedForAuthentication === 'string' ? (c.security_info.BootstrapTokenAllowedForAuthentication as string) : ''
  return (
    <>
      <CardHeader title="Security" />
      <CardBody>
        <DescriptionList
          cols={3}
          items={[
            { label: 'FileVault', value: <BoolBadge value={c.filevault_enabled} yes="Enabled" no="Disabled" /> },
            { label: 'System Integrity Protection', value: <BoolBadge value={c.sip_enabled} yes="Enabled" no="Disabled" /> },
            { label: 'Firewall', value: <BoolBadge value={c.firewall_enabled} yes="Enabled" no="Disabled" /> },
            { label: 'Gatekeeper', value: <BoolBadge value={c.gatekeeper_enabled} yes="Enabled" no="Disabled" /> },
            { label: 'Secure boot level', value: c.secure_boot_level },
            { label: 'Supervised', value: <BoolBadge value={c.is_supervised} /> },
            { label: 'Activation Lock', value: <BoolBadge value={c.activation_lock_enabled} yes="Enabled" no="Disabled" /> },
            { label: 'Bootstrap token escrowed', value: <BoolBadge value={c.bootstrap_token_escrowed} /> },
            ...(bootstrapAuth ? [{ label: 'Bootstrap token for authentication', value: titleCaseWords(bootstrapAuth) }] : []),
          ]}
        />
        {c.mdm_enrolled && c.bootstrap_token_escrowed !== true && (
          <p className="mt-4 text-xs text-ink-muted">
            macOS escrows the bootstrap token when a Secure Token user logs in. To send it now, run <code className="font-mono text-ink">sudo profiles install -type bootstraptoken</code> on
            the Mac and sign in as a Secure Token administrator; <code className="font-mono text-ink">sudo profiles status -type bootstraptoken</code> shows the result.
          </p>
        )}
      </CardBody>
      <FileVaultPanel c={c} />
      <LapsPanel c={c} />
      <CardHeader title="MDM security information" description="Latest SecurityInfo response." className="border-t border-line" />
      <CardBody>
        <KeyValueTable data={c.security_info} />
      </CardBody>
    </>
  )
}

export function AppsTab({ c }: { c: ComputerDetail }) {
  const [q, setQ] = useState('')
  const apps = useMemo(() => {
    const all = c.apps ?? []
    const s = q.trim().toLowerCase()
    const f = s ? all.filter((a) => a.name.toLowerCase().includes(s) || a.bundle_id.toLowerCase().includes(s) || a.path.toLowerCase().includes(s)) : all
    return [...f].sort((a, b) => a.name.localeCompare(b.name))
  }, [c.apps, q])
  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-3">
        <p className="text-sm text-ink-muted">
          {apps.length} of {(c.apps ?? []).length} applications
        </p>
        <SearchInput value={q} onChange={setQ} placeholder="Filter applications…" className="w-full sm:w-72" />
      </div>
      <Table<ComputerApp>
        rows={apps}
        rowKey={(a) => `${a.bundle_id}|${a.path}|${a.name}`}
        dense
        emptyTitle={q ? 'No applications match' : 'No applications reported'}
        columns={[
          { key: 'name', header: 'Name', render: (a) => <span className="font-medium text-ink">{a.name}</span> },
          { key: 'version', header: 'Version', render: (a) => <span className="font-mono text-xs">{a.version || '—'}</span> },
          { key: 'bundle', header: 'Bundle ID', render: (a) => <span className="font-mono text-xs text-ink-muted">{a.bundle_id || '—'}</span> },
          { key: 'path', header: 'Path', render: (a) => <span className="text-xs text-ink-muted truncate block max-w-[320px]" title={a.path}>{a.path || '—'}</span> },
          { key: 'source', header: 'Source', render: (a) => (a.source ? <Badge>{a.source}</Badge> : null) },
        ]}
      />
    </>
  )
}

export function ProfilesTab({ c }: { c: ComputerDetail }) {
  const deployments = c.profile_deployments ?? []
  return (
    <>
      <CardHeader title="Managed profile deployments" description="Profiles VibeMDM has pushed to this computer." />
      <Table<ProfileDeployment>
        rows={deployments}
        rowKey={(d) => d.profile_id}
        dense
        emptyTitle="No profiles have been deployed to this computer"
        columns={[
          {
            key: 'name',
            header: 'Profile',
            render: (d) => (
              <Link to={`/profiles/${d.profile_id}`} className="font-medium text-ink hover:underline">
                {d.profile_name || d.profile_id}
              </Link>
            ),
          },
          { key: 'status', header: 'Status', render: (d) => <StatusBadge status={d.status} /> },
          { key: 'error', header: 'Error', render: (d) => <span className="text-xs text-critical">{d.error}</span> },
          { key: 'updated', header: 'Updated', render: (d) => <RelativeTime value={d.updated_at} className="text-ink-muted" /> },
        ]}
      />
      <CardHeader title="Installed profiles" description="All configuration profiles present on the device, as reported by MDM." className="border-t border-line" />
      <Table<ComputerProfile>
        rows={c.profiles ?? []}
        rowKey={(p) => p.uuid || p.identifier}
        dense
        emptyTitle="No installed profiles reported"
        columns={[
          { key: 'name', header: 'Name', render: (p) => <span className="font-medium text-ink">{p.display_name || p.identifier}</span> },
          { key: 'identifier', header: 'Identifier', render: (p) => <span className="font-mono text-xs text-ink-muted">{p.identifier}</span> },
          { key: 'org', header: 'Organization', render: (p) => <span className="text-ink-muted">{p.organization || '—'}</span> },
          { key: 'managed', header: 'Managed', render: (p) => (p.is_managed ? <Badge tone="accent">Managed</Badge> : <Badge>Unmanaged</Badge>) },
          { key: 'source', header: 'Source', render: (p) => <span className="text-xs text-ink-muted">{p.source}</span> },
        ]}
      />
    </>
  )
}

export function EATab({ c }: { c: ComputerDetail }) {
  const { isAdmin } = useAuth()
  const eas = useExtensionAttributes()
  const toast = useToast()
  const invalidate = useInvalidate()
  const byId = useMemo(() => new Map((eas.data?.items ?? []).map((e) => [e.id, e])), [eas.data])
  const [editing, setEditing] = useState<string | null>(null)
  const [val, setVal] = useState('')
  const [saving, setSaving] = useState(false)

  // Show every defined EA, even if the computer has no value yet.
  const rows: EAValue[] = useMemo(() => {
    const have = new Map((c.extension_attributes ?? []).map((v) => [v.ea_id, v]))
    const out: EAValue[] = []
    for (const e of eas.data?.items ?? []) {
      out.push(have.get(e.id) ?? { ea_id: e.id, name: e.name, data_type: e.data_type, value: '', updated_at: '' })
      have.delete(e.id)
    }
    for (const v of have.values()) out.push(v)
    return out
  }, [c.extension_attributes, eas.data])

  const save = async (eaId: string) => {
    setSaving(true)
    try {
      await api.post(`/api/v1/computers/${c.id}/ea-values/${eaId}`, { value: val })
      await invalidate(qk.computer(c.id))
      toast.success('Value saved')
      setEditing(null)
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <CardHeader title="Extension attributes" description="Custom inventory fields. Script-based values are collected by the agent; text values can be set here." />
      <Table<EAValue>
        rows={rows}
        loading={eas.isPending}
        rowKey={(v) => v.ea_id}
        dense
        emptyTitle="No extension attributes defined"
        emptyAction={
          isAdmin ? (
            <Link to="/extension-attributes/new">
              <Button size="sm" variant="primary">
                Create one
              </Button>
            </Link>
          ) : undefined
        }
        columns={[
          {
            key: 'name',
            header: 'Attribute',
            render: (v) => (
              <Link to={`/extension-attributes/${v.ea_id}`} className="font-medium text-ink hover:underline">
                {v.name}
              </Link>
            ),
          },
          { key: 'type', header: 'Type', render: (v) => <Badge>{byId.get(v.ea_id)?.input_type === 'text' ? 'text' : 'script'} · {v.data_type}</Badge> },
          {
            key: 'value',
            header: 'Value',
            render: (v) =>
              editing === v.ea_id ? (
                <span className="flex items-center gap-2">
                  <Input value={val} onChange={(e) => setVal(e.target.value)} className="h-8 w-64" autoFocus type={v.data_type === 'integer' ? 'number' : v.data_type === 'date' ? 'date' : 'text'} />
                  <Button size="xs" variant="primary" loading={saving} onClick={() => save(v.ea_id)}>
                    Save
                  </Button>
                  <Button size="xs" variant="ghost" onClick={() => setEditing(null)}>
                    Cancel
                  </Button>
                </span>
              ) : (
                <span className={v.value ? 'font-mono text-xs' : 'text-ink-faint'}>{v.value || '—'}</span>
              ),
          },
          { key: 'updated', header: 'Updated', render: (v) => <RelativeTime value={v.updated_at || null} emptyLabel="—" className="text-ink-muted" /> },
          {
            key: 'actions',
            header: '',
            align: 'right',
            render: (v) =>
              isAdmin && byId.get(v.ea_id)?.input_type === 'text' && editing !== v.ea_id ? (
                <Button
                  size="xs"
                  variant="ghost"
                  icon={<Pencil />}
                  onClick={() => {
                    setEditing(v.ea_id)
                    setVal(v.value)
                  }}
                >
                  Edit
                </Button>
              ) : null,
          },
        ]}
      />
    </>
  )
}

export function LocalUsersTab({ c }: { c: ComputerDetail }) {
  return (
    <>
      <CardHeader title="Local user accounts" />
      <Table<LocalUser>
        rows={c.local_users ?? []}
        rowKey={(u) => u.username}
        dense
        emptyTitle="No local users reported"
        columns={[
          { key: 'username', header: 'Username', render: (u) => <span className="font-medium text-ink font-mono text-xs">{u.username}</span> },
          { key: 'real', header: 'Full name', render: (u) => u.real_name || '—' },
          { key: 'uid', header: 'UID', render: (u) => <span className="tabular-nums text-ink-muted">{u.uid ?? '—'}</span> },
          { key: 'home', header: 'Home', render: (u) => <span className="font-mono text-xs text-ink-muted">{u.home}</span> },
          { key: 'admin', header: 'Admin', render: (u) => (u.is_admin ? <Badge tone="warning">Admin</Badge> : <Badge>Standard</Badge>) },
          { key: 'token', header: 'Secure token', render: (u) => <BoolBadge value={u.secure_token} /> },
        ]}
      />
    </>
  )
}

export function GroupsTab({ c }: { c: ComputerDetail }) {
  return (
    <>
      <CardHeader title="Group membership" description="Smart groups are evaluated from current inventory; static groups are assigned manually." />
      <Table
        rows={c.groups}
        rowKey={(g) => g.id}
        dense
        emptyTitle="This computer is not a member of any group"
        columns={[
          {
            key: 'name',
            header: 'Group',
            render: (g) => (
              <Link to={`/groups/${g.id}`} className="font-medium text-ink hover:underline">
                {g.name}
              </Link>
            ),
          },
          { key: 'type', header: 'Type', render: (g) => <Badge tone={g.is_smart ? 'accent' : 'neutral'}>{g.is_smart ? 'Smart' : 'Static'}</Badge> },
          { key: 'desc', header: 'Description', render: (g) => <span className="text-ink-muted">{g.description || '—'}</span> },
          { key: 'count', header: 'Members', align: 'right', render: (g) => <span className="tabular-nums">{g.member_count}</span> },
        ]}
      />
    </>
  )
}
