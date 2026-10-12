import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Eye } from 'lucide-react'
import { qk, useCriteriaFields, useGroup, useGroupMembers, useInvalidate } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Computer, ComputerGroup, Criterion, GroupPreview } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Field, FormGrid } from '../components/ui/Field'
import { Input } from '../components/ui/Input'
import { Toggle } from '../components/ui/Checkbox'
import { Tabs } from '../components/ui/Tabs'
import { Table } from '../components/ui/Table'
import { Alert } from '../components/ui/Alert'
import { Badge } from '../components/ui/Badge'
import { LoadingBlock } from '../components/ui/Spinner'
import { RelativeTime } from '../components/ui/RelativeTime'
import { useToast } from '../components/ui/Toast'
import { useCloneSource, useEditMode } from '../components/editor/useEditMode'
import { ObjectBar } from '../components/editor/ObjectBar'
import { DetailsLayout, SummaryCard } from '../components/templates/DetailsLayout'
import { CriteriaBuilder, describeCriteria } from '../components/pickers/CriteriaBuilder'
import { ComputerPicker } from '../components/pickers/ComputerPicker'
import { ComputerStatusBadges } from './computers/ComputersPage'
import { latest } from './DashboardPage'

type Tab = 'settings' | 'members'

/** What Clone hands the new group: the group plus a static group's members. */
type GroupClone = Pick<ComputerGroup, 'name' | 'description' | 'is_smart' | 'criteria'> & { member_ids?: string[] }

export function GroupEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const group = useGroup(isNew ? undefined : id)
  const members = useGroupMembers(isNew ? undefined : id)
  const fields = useCriteriaFields()

  const clone = useCloneSource<GroupClone>()
  const [name, setName] = useState(() => (isNew && clone ? clone.name : ''))
  const [description, setDescription] = useState(() => (isNew && clone ? clone.description : ''))
  const [isSmart, setIsSmart] = useState(() => (isNew && clone ? clone.is_smart : true))
  const [criteria, setCriteria] = useState<Criterion[]>(() => (isNew && clone ? clone.criteria ?? [] : []))
  const [memberIds, setMemberIds] = useState<string[]>(() => (isNew && clone ? clone.member_ids ?? [] : []))
  const [dirtyMembers, setDirtyMembers] = useState(false)
  const [saving, setSaving] = useState(false)
  const [preview, setPreview] = useState<GroupPreview | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const tab = (sp.get('tab') as Tab) || 'settings'
  const setTab = (t: Tab) => setSp(t === 'settings' ? {} : { tab: t }, { replace: true })

  const load = useCallback(() => {
    if (group.data) {
      setName(group.data.name)
      setDescription(group.data.description)
      setIsSmart(group.data.is_smart)
      setCriteria(group.data.criteria ?? [])
    }
    if (members.data) setMemberIds(members.data.items.map((m) => m.id))
    setDirtyMembers(false)
  }, [group.data, members.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: { name, description, isSmart, criteria, memberIds: isSmart ? [] : memberIds }, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])
  // Clone onto the same route (/x/:id → /x/new keeps this page mounted): take the copy.
  useEffect(() => {
    if (isNew && clone) {
      setName(clone.name)
      setDescription(clone.description)
      setIsSmart(clone.is_smart)
      setCriteria(clone.criteria ?? [])
      setMemberIds(clone.member_ids ?? [])
      setPreview(null)
    }
  }, [isNew, clone])

  const save = async () => {
    if (!name.trim()) {
      setTab('settings')
      toast.error('Name is required')
      return false
    }
    setSaving(true)
    try {
      const body: Partial<ComputerGroup> = { name: name.trim(), description, is_smart: isSmart, criteria: isSmart ? criteria : [] }
      let out: ComputerGroup
      if (isNew) out = await api.post<ComputerGroup>('/api/v1/groups', body)
      else out = await api.put<ComputerGroup>(`/api/v1/groups/${id}`, body)
      if (!isSmart && (isNew || dirtyMembers)) {
        await api.put(`/api/v1/groups/${out.id}/members`, { computer_ids: memberIds })
      }
      await invalidate(qk.groups, qk.group(out.id), qk.groupMembers(out.id), qk.dashboard, ['computers'])
      setDirtyMembers(false)
      toast.success(isNew ? 'Group created' : 'Group saved')
      if (isNew) navigate(`/groups/${out.id}`, { replace: true })
      return true
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
      return false
    } finally {
      setSaving(false)
    }
  }

  const runPreview = async () => {
    setPreviewing(true)
    try {
      const r = await api.post<GroupPreview>('/api/v1/groups/preview', { criteria })
      setPreview(r)
    } catch (e) {
      toast.error('Preview failed', errorMessage(e))
    } finally {
      setPreviewing(false)
    }
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/groups/${id}`)
      toast.success('Group deleted')
      await invalidate(qk.groups, qk.dashboard)
      navigate('/groups')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (!isNew && group.isPending) return <LoadingBlock />
  if (!isNew && group.error) return <Alert tone="critical">{errorMessage(group.error)}</Alert>
  const ro = ed.readOnly

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Computer groups', to: '/groups' }, { label: isNew ? 'New group' : group.data?.name ?? '' }]}
        title={isNew ? 'New computer group' : group.data?.name}
      />
      {!isNew && <Tabs className="mb-5" value={tab} onChange={setTab} items={[{ key: 'settings', label: 'Settings' }, { key: 'members', label: 'Members', count: members.data?.total, shortcut: 'view' }]} />}

      {tab === 'settings' && (
        <DetailsLayout
          aside={
            !isNew &&
            group.data && (
              <SummaryCard
                rows={[
                  { label: 'Type', value: <Badge tone={group.data.is_smart ? 'accent' : 'neutral'}>{group.data.is_smart ? 'Smart group' : 'Static group'}</Badge> },
                  { label: 'Members', value: (members.data?.total ?? group.data.member_count).toLocaleString() },
                  { label: 'Criteria', value: group.data.is_smart ? `${criteria.length} rule${criteria.length === 1 ? '' : 's'}` : '' },
                  { label: 'Last changed', value: <RelativeTime value={group.data.updated_at} /> },
                ]}
              />
            )
          }
        >
          <Card>
            <CardHeader title="General" />
            <CardBody>
              <FormGrid cols={2}>
                <Field label="Name" required>
                  <Input value={name} onChange={(e) => setName(e.target.value)} disabled={ro} placeholder="e.g. Engineering laptops" />
                </Field>
                <Field label="Description">
                  <Input value={description} onChange={(e) => setDescription(e.target.value)} disabled={ro} />
                </Field>
              </FormGrid>
              <div className="mt-5">
                <Toggle
                  checked={isSmart}
                  onChange={setIsSmart}
                  disabled={ro || !isNew}
                  label="Smart group"
                  description={isNew ? 'Membership is computed from criteria. Turn off to pick members manually.' : 'The group type cannot be changed after creation.'}
                />
              </div>
            </CardBody>
          </Card>

          {isSmart ? (
            <Card>
              <CardHeader
                title="Criteria"
                description="Computers matching these rules are members. Rules are evaluated top to bottom; use parentheses to group and/or conditions."
                actions={
                  <Button icon={<Eye />} loading={previewing} onClick={runPreview} disabled={criteria.length === 0}>
                    Preview
                  </Button>
                }
              />
              <CardBody className="flex flex-col gap-4">
                <CriteriaBuilder value={criteria} onChange={setCriteria} disabled={ro} />
                {criteria.length > 0 && <p className="text-xs text-ink-muted font-mono rounded-lg bg-surface-sunken px-3 py-2">{describeCriteria(criteria, fields.data)}</p>}
                {preview && (
                  <div className="rounded-xl border border-line">
                    <div className="flex items-center justify-between px-4 py-2.5 border-b border-line">
                      <p className="text-sm font-medium text-ink">
                        Preview: {preview.count.toLocaleString()} matching {preview.count === 1 ? 'computer' : 'computers'}
                      </p>
                      <Button size="xs" variant="ghost" onClick={() => setPreview(null)}>
                        Dismiss
                      </Button>
                    </div>
                    <MemberTable rows={preview.computers} emptyTitle="No computers match these criteria" />
                  </div>
                )}
              </CardBody>
            </Card>
          ) : (
            <Card>
              <CardHeader title="Members" description="Search for computers and add them to this static group." />
              <CardBody>
                <ComputerPicker
                  value={memberIds}
                  onChange={(ids) => {
                    setMemberIds(ids)
                    setDirtyMembers(true)
                  }}
                  disabled={ro}
                />
                {dirtyMembers && <p className="mt-2 text-xs text-ink-muted">Membership changes are saved when you click Save.</p>}
              </CardBody>
            </Card>
          )}
        </DetailsLayout>
      )}

      {tab === 'members' && !isNew && (
        <Card>
          <CardHeader title={`Members (${members.data?.total ?? 0})`} description={group.data?.is_smart ? 'Computed from the current criteria.' : 'Edit membership on the Settings tab.'} />
          <MemberTable rows={members.data?.items} loading={members.isPending} emptyTitle="This group has no members" />
        </Card>
      )}

      <ObjectBar
        ed={ed}
        isNew={isNew}
        listPath="/groups"
        noun="group"
        name={group.data?.name}
        saving={saving}
        onSave={save}
        onDelete={doDelete}
        deleteDescription="Policies and profiles scoped to this group will no longer target its members."
        onClone={() => navigate('/groups/new', { state: { clone: { name: `Copy of ${name}`, description, is_smart: isSmart, criteria: isSmart ? criteria : [], member_ids: isSmart ? [] : memberIds } satisfies GroupClone } })}
        historyId={id}
      />
    </>
  )
}

export function MemberTable({ rows, loading, emptyTitle }: { rows: Computer[] | undefined; loading?: boolean; emptyTitle: string }) {
  const navigate = useNavigate()
  return (
    <Table<Computer>
      rows={rows}
      loading={loading}
      rowKey={(c) => c.id}
      dense
      onRowClick={(c) => navigate(`/computers/${c.id}`)}
      emptyTitle={emptyTitle}
      columns={[
        { key: 'name', header: 'Computer', render: (c) => <span className="font-medium text-ink">{c.name || c.hostname || c.serial_number}</span> },
        { key: 'serial', header: 'Serial', render: (c) => <span className="font-mono text-xs">{c.serial_number}</span> },
        { key: 'model', header: 'Model', render: (c) => <span className="text-ink-muted">{c.model_name || c.model || '—'}</span> },
        { key: 'os', header: 'macOS', render: (c) => <span className="tabular-nums">{c.os_version || '—'}</span> },
        { key: 'user', header: 'Last user', render: (c) => <span className="text-ink-muted">{c.last_user || '—'}</span> },
        { key: 'checkin', header: 'Last check-in', render: (c) => <RelativeTime value={latest(c.agent_last_checkin, c.mdm_last_checkin)} className="text-ink-muted" /> },
        { key: 'status', header: 'Status', render: (c) => <ComputerStatusBadges c={c} /> },
      ]}
    />
  )
}
