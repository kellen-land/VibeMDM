import { useState, type FormEvent } from 'react'
import { Building2, Briefcase, Network, Pencil, Plus, Trash2 } from 'lucide-react'
import { qk, useBuildings, useDepartments, useInvalidate, useNetworkSegments } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { Building, Department, NetworkSegment } from '../../api/types'
import { Card, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Table } from '../../components/ui/Table'
import { Modal } from '../../components/ui/Modal'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select } from '../../components/ui/Input'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { Alert } from '../../components/ui/Alert'
import { useToast } from '../../components/ui/Toast'

export function NetworkOrgSettings() {
  return (
    <div className="flex flex-col gap-6">
      <Alert tone="info">
        Buildings, departments and network segments are the vocabulary used by scope Targets, Limitations and Exclusions. A network segment can also <span className="font-medium">auto-assign</span> its building and department to any computer seen inside its IP range.
      </Alert>
      <div className="grid gap-6 lg:grid-cols-2">
        <NamedList kind="building" title="Buildings" description="Physical locations. Assigned per computer on its General tab or by a network segment." path="/api/v1/buildings" queryKey={qk.buildings} useList={useBuildings} icon={<Building2 />} />
        <NamedList kind="department" title="Departments" description="Organizational units. Assigned per computer on its General tab or by a network segment." path="/api/v1/departments" queryKey={qk.departments} useList={useDepartments} icon={<Briefcase />} />
      </div>
      <SegmentsList />
    </div>
  )
}

type Named = Building | Department

function NamedList({
  kind,
  title,
  description,
  path,
  queryKey,
  useList,
  icon,
}: {
  kind: string
  title: string
  description: string
  path: string
  queryKey: readonly unknown[]
  useList: typeof useBuildings | typeof useDepartments
  icon: React.ReactNode
}) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const list = useList()
  const [adding, setAdding] = useState('')
  const [busy, setBusy] = useState(false)
  const [edit, setEdit] = useState<Named | null>(null)
  const [editName, setEditName] = useState('')
  const [del, setDel] = useState<Named | null>(null)
  const refresh = () => invalidate(queryKey, ['computers'])

  const add = async (e: FormEvent) => {
    e.preventDefault()
    if (!adding.trim()) return
    setBusy(true)
    try {
      await api.post(path, { name: adding.trim() })
      setAdding('')
      await refresh()
      toast.success(`${title.slice(0, -1)} added`)
    } catch (err) {
      toast.error('Add failed', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  const rename = async (e: FormEvent) => {
    e.preventDefault()
    if (!edit || !editName.trim()) return
    setBusy(true)
    try {
      await api.put(`${path}/${edit.id}`, { name: editName.trim() })
      await refresh()
      toast.success('Renamed')
      setEdit(null)
    } catch (err) {
      toast.error('Rename failed', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`${path}/${del.id}`)
      await refresh()
      toast.success('Deleted')
      setDel(null)
    } catch (err) {
      toast.error('Delete failed', errorMessage(err))
    }
  }

  return (
    <Card>
      <CardHeader title={title} description={description} />
      {isAdmin && (
        <form onSubmit={add} className="flex items-center gap-2 border-b border-line px-4 py-3">
          <Input value={adding} onChange={(e) => setAdding(e.target.value)} placeholder={`New ${kind} name`} className="flex-1" />
          <Button type="submit" variant="primary" size="sm" icon={<Plus />} loading={busy} disabled={!adding.trim()}>
            Add
          </Button>
        </form>
      )}
      <Table<Named>
        rows={list.data?.items}
        loading={list.isPending}
        error={list.error ? errorMessage(list.error) : null}
        rowKey={(r) => r.id}
        dense
        emptyTitle={`No ${title.toLowerCase()} yet`}
        columns={[
          {
            key: 'name',
            header: 'Name',
            render: (r) => (
              <span className="flex items-center gap-2.5">
                <span className="text-ink-faint [&>svg]:size-4">{icon}</span>
                <span className="font-medium text-ink">{r.name}</span>
              </span>
            ),
          },
          { key: 'computers', header: 'Computers', align: 'right', render: (r) => <span className="tabular-nums">{r.computers.toLocaleString()}</span> },
          { key: 'updated', header: 'Updated', render: (r) => <RelativeTime value={r.updated_at} className="text-ink-muted" /> },
          {
            key: 'actions',
            header: '',
            align: 'right',
            render: (r) =>
              isAdmin ? (
                <span className="inline-flex gap-1">
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<Pencil />}
                    aria-label="Rename"
                    onClick={() => {
                      setEdit(r)
                      setEditName(r.name)
                    }}
                  />
                  <Button size="xs" variant="ghost" icon={<Trash2 />} aria-label="Delete" onClick={() => setDel(r)} />
                </span>
              ) : null,
          },
        ]}
      />
      <Modal open={!!edit} onClose={() => setEdit(null)} title={`Rename ${kind}`} size="sm">
        <form onSubmit={rename} className="flex flex-col gap-4">
          <Field label="Name" required hint="Computers already assigned keep the old value; scopes referencing it by name should be updated.">
            <Input value={editName} onChange={(e) => setEditName(e.target.value)} autoFocus required />
          </Field>
          <div className="flex justify-end gap-2 pt-1">
            <Button variant="ghost" onClick={() => setEdit(null)} data-shortcut="cancel">
              Cancel
            </Button>
            <Button type="submit" variant="primary" loading={busy} data-shortcut="save">
              Save
            </Button>
          </div>
        </form>
      </Modal>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete ${kind} "${del?.name}"?`} description={`Computers keep their current ${kind} value; scopes that reference it by name will no longer match anything.`} confirmLabel="Delete" />
    </Card>
  )
}

const blankSegment = (): NetworkSegment => ({ id: '', name: '', starting_address: '', ending_address: '', assign_building: '', assign_department: '', computers: 0, created_at: '', updated_at: '' })

function SegmentsList() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const segments = useNetworkSegments()
  const [edit, setEdit] = useState<NetworkSegment | null>(null)
  const [del, setDel] = useState<NetworkSegment | null>(null)

  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/network-segments/${del.id}`)
      await invalidate(qk.networkSegments)
      toast.success('Network segment deleted')
      setDel(null)
    } catch (err) {
      toast.error('Delete failed', errorMessage(err))
    }
  }

  return (
    <Card>
      <CardHeader
        title="Network segments"
        description="Inclusive IP ranges matched against each computer's last reported address. Use them in scope targets, limitations and exclusions, or to auto-assign a building and department."
        actions={
          isAdmin && (
            <Button variant="primary" size="sm" icon={<Plus />} onClick={() => setEdit(blankSegment())}>
              New segment
            </Button>
          )
        }
      />
      <Table<NetworkSegment>
        rows={segments.data?.items}
        loading={segments.isPending}
        error={segments.error ? errorMessage(segments.error) : null}
        rowKey={(r) => r.id}
        onRowClick={isAdmin ? (r) => setEdit(r) : undefined}
        emptyTitle="No network segments yet"
        emptyDescription="Define a range like 10.0.1.1 – 10.0.1.254 to scope by office or VLAN."
        columns={[
          {
            key: 'name',
            header: 'Name',
            render: (r) => (
              <span className="flex items-center gap-2.5">
                <Network className="size-4 text-ink-faint" />
                <span className="font-medium text-ink">{r.name}</span>
              </span>
            ),
          },
          { key: 'range', header: 'Range', render: (r) => <span className="font-mono text-xs">{r.starting_address} – {r.ending_address}</span> },
          {
            key: 'assign',
            header: 'Auto-assigns',
            render: (r) =>
              r.assign_building || r.assign_department ? (
                <span className="text-ink-muted text-xs">
                  {[r.assign_building && `Building: ${r.assign_building}`, r.assign_department && `Department: ${r.assign_department}`].filter(Boolean).join(' · ')}
                </span>
              ) : (
                <span className="text-ink-faint text-xs">—</span>
              ),
          },
          { key: 'computers', header: 'Computers', align: 'right', render: (r) => <span className="tabular-nums">{r.computers.toLocaleString()}</span> },
          { key: 'updated', header: 'Updated', render: (r) => <RelativeTime value={r.updated_at} className="text-ink-muted" /> },
          {
            key: 'actions',
            header: '',
            align: 'right',
            render: (r) =>
              isAdmin ? (
                <Button
                  size="xs"
                  variant="ghost"
                  icon={<Trash2 />}
                  aria-label="Delete segment"
                  onClick={(e) => {
                    e.stopPropagation()
                    setDel(r)
                  }}
                />
              ) : null,
          },
        ]}
      />
      {edit && <SegmentDialog seg={edit} onClose={() => setEdit(null)} />}
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete segment "${del?.name}"?`} description="Scopes that reference this segment will no longer match it." confirmLabel="Delete segment" />
    </Card>
  )
}

function SegmentDialog({ seg, onClose }: { seg: NetworkSegment; onClose: () => void }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const buildings = useBuildings()
  const departments = useDepartments()
  const [form, setForm] = useState<NetworkSegment>(seg)
  const [busy, setBusy] = useState(false)
  const isNew = !seg.id
  const set = (patch: Partial<NetworkSegment>) => setForm({ ...form, ...patch })
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const body = { name: form.name.trim(), starting_address: form.starting_address.trim(), ending_address: form.ending_address.trim(), assign_building: form.assign_building, assign_department: form.assign_department }
      if (isNew) await api.post('/api/v1/network-segments', body)
      else await api.put(`/api/v1/network-segments/${seg.id}`, body)
      await invalidate(qk.networkSegments, ['computers'])
      toast.success(isNew ? 'Network segment created' : 'Network segment saved')
      onClose()
    } catch (err) {
      toast.error('Save failed', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  const nameOptions = (items: { name: string }[] | undefined, current: string) => {
    const names = (items ?? []).map((i) => i.name)
    if (current && !names.includes(current)) names.unshift(current)
    return [{ value: '', label: 'Do not assign' }, ...names.map((n) => ({ value: n, label: n }))]
  }
  return (
    <Modal open onClose={onClose} title={isNew ? 'New network segment' : `Edit ${seg.name}`} size="md">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field label="Name" required>
          <Input value={form.name} onChange={(e) => set({ name: e.target.value })} required autoFocus placeholder="e.g. HQ Wi-Fi" />
        </Field>
        <FormGrid cols={2}>
          <Field label="Starting address" required>
            <Input value={form.starting_address} onChange={(e) => set({ starting_address: e.target.value })} required mono placeholder="10.0.1.1" />
          </Field>
          <Field label="Ending address" required hint="Inclusive. IPv4 or IPv6.">
            <Input value={form.ending_address} onChange={(e) => set({ ending_address: e.target.value })} required mono placeholder="10.0.1.254" />
          </Field>
          <Field label="Assign building" hint="Set on computers seen in this range.">
            <Select value={form.assign_building} onChange={(e) => set({ assign_building: e.target.value })} options={nameOptions(buildings.data?.items, form.assign_building)} />
          </Field>
          <Field label="Assign department">
            <Select value={form.assign_department} onChange={(e) => set({ assign_department: e.target.value })} options={nameOptions(departments.data?.items, form.assign_department)} />
          </Field>
        </FormGrid>
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={onClose} data-shortcut="cancel">
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={busy} data-shortcut="save">
            {isNew ? 'Create segment' : 'Save'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}
