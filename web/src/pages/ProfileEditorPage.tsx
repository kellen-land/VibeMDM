import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Download, FileCode2, RefreshCw, Trash2, Upload, X } from 'lucide-react'
import { qk, useCustomSchema, useInvalidate, useProfile, useProfileDeployments } from '../api/queries'
import { api, errorMessage, saveText } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { emptyScope, normalizeScope, type ConfigProfile, type ProfileDeployment, type SchemaValues, type Scope } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Field, FormGrid } from '../components/ui/Field'
import { Input, Textarea } from '../components/ui/Input'
import { Tabs } from '../components/ui/Tabs'
import { Alert } from '../components/ui/Alert'
import { Badge, StatusBadge } from '../components/ui/Badge'
import { Table } from '../components/ui/Table'
import { LoadingBlock } from '../components/ui/Spinner'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { RelativeTime } from '../components/ui/RelativeTime'
import { Checkbox } from '../components/ui/Checkbox'
import { useToast } from '../components/ui/Toast'
import { CodeEditor } from '../components/CodeEditor'
import { ScopeEditor, ScopeSummary } from '../components/pickers/ScopeEditor'
import { DeploymentCounts } from './ProfilesPage'
import { managedProfiles, schemaIdFromSource } from '../components/schema/schema'
import { Link } from 'react-router-dom'
import { PayloadProfileEditorPage } from './profiles/PayloadProfileEditorPage'
import type { SchemaProfileClone } from './profiles/SchemaProfileEditorPage'
import { useEditMode } from '../components/editor/useEditMode'
import { ObjectBar } from '../components/editor/ObjectBar'
import { DetailsLayout, SummaryCard } from '../components/templates/DetailsLayout'

type Tab = 'general' | 'scope' | 'deployments'

/** Best-effort client-side parse of a mobileconfig's top-level keys for the read-only preview. */
function parsePayloadMeta(xml: string): { identifier?: string; uuid?: string; displayName?: string; scope?: string; type?: string; description?: string } {
  try {
    const doc = new DOMParser().parseFromString(xml, 'application/xml')
    if (doc.querySelector('parsererror')) return {}
    const root = doc.querySelector('plist > dict')
    if (!root) return {}
    const out: Record<string, string> = {}
    const children = Array.from(root.children)
    for (let i = 0; i < children.length - 1; i++) {
      const k = children[i]
      if (k.tagName === 'key') {
        const v = children[i + 1]
        if (v.tagName === 'string') out[k.textContent ?? ''] = v.textContent ?? ''
      }
    }
    return { identifier: out.PayloadIdentifier, uuid: out.PayloadUUID, displayName: out.PayloadDisplayName, scope: out.PayloadScope, type: out.PayloadType, description: out.PayloadDescription }
  } catch {
    return {}
  }
}

/** Profiles built with the payload editors open in the editor; uploaded ones in the XML view. */
export function ProfileEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const [sp] = useSearchParams()
  const profile = useProfile(isNew ? undefined : id)
  if (!isNew && profile.isPending) return <LoadingBlock />
  const editor = isNew ? sp.get('upload') !== '1' : Array.isArray(profile.data?.payloads)
  // Keyed so going from one profile to another (or to a clone) starts with fresh state.
  return editor ? <PayloadProfileEditorPage key={id ?? 'new'} /> : <UploadedProfileEditorPage key={id ?? 'new'} />
}

function UploadedProfileEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const profile = useProfile(isNew ? undefined : id)
  const fileRef = useRef<HTMLInputElement>(null)

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState('')
  const [payload, setPayload] = useState('')
  const [scope, setScope] = useState<Scope>(emptyScope())
  const [saving, setSaving] = useState(false)
  const tab = (sp.get('tab') as Tab) || 'general'
  const setTab = (t: Tab) => setSp(t === 'general' ? {} : { tab: t }, { replace: true })

  const load = useCallback(() => {
    if (profile.data) {
      setName(profile.data.name)
      setDescription(profile.data.description)
      setCategory(profile.data.category)
      setPayload(profile.data.payload ?? '')
      setScope(normalizeScope(profile.data.scope))
    }
  }, [profile.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: { name, description, category, payload, scope }, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])

  const schemaId = schemaIdFromSource(profile.data?.schema_source)
  const schemaInfo = useCustomSchema(schemaId)
  const meta = useMemo(() => parsePayloadMeta(payload), [payload])
  const payloadLooksValid = payload.trim() === '' || (meta.type === 'Configuration' && !!meta.identifier)

  const onFile = async (f: File | undefined) => {
    if (!f) return
    const text = await f.text()
    setPayload(text)
    const m = parsePayloadMeta(text)
    if (!name && m.displayName) setName(m.displayName)
    if (!description && m.description) setDescription(m.description)
    if (fileRef.current) fileRef.current.value = ''
  }

  const save = async () => {
    if (isNew && !payload.trim()) {
      setTab('general')
      toast.error('A profile payload (.mobileconfig) is required')
      return false
    }
    if (payload.trim() !== '' && !payloadLooksValid) {
      setTab('general')
      toast.error('This does not look like a Configuration profile', 'The top-level PayloadType must be Configuration and a PayloadIdentifier is required.')
      return false
    }
    setSaving(true)
    try {
      const body = { name: name.trim(), description, category, payload, scope }
      const out = isNew ? await api.post<ConfigProfile>('/api/v1/profiles', body) : await api.put<ConfigProfile>(`/api/v1/profiles/${id}`, body)
      await invalidate(qk.profiles, qk.profile(out.id), qk.profileDeployments(out.id), qk.dashboard)
      toast.success(isNew ? 'Profile created' : 'Profile saved', 'Deployment to scoped computers has been queued.')
      if (isNew) navigate(`/profiles/${out.id}`, { replace: true })
      return true
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
      return false
    } finally {
      setSaving(false)
    }
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/profiles/${id}`)
      toast.success('Profile deleted')
      await invalidate(qk.profiles, qk.dashboard)
      navigate('/profiles')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (!isNew && profile.isPending) return <LoadingBlock />
  if (!isNew && profile.error) return <Alert tone="critical">{errorMessage(profile.error)}</Alert>
  const ro = ed.readOnly
  const p = profile.data
  const managed = p ? managedProfiles[p.identifier] : undefined
  const fromSchema = !!p?.schema_source
  const detaches = fromSchema && payload !== (p?.payload ?? '') && payload.trim() !== ''
  const schemaName = schemaInfo.data?.name ?? (schemaInfo.isError ? 'a deleted schema' : schemaId ?? '')

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Configuration profiles', to: '/profiles' }, { label: isNew ? 'New profile' : p?.name ?? '' }]}
        title={isNew ? 'Upload a profile' : p?.name}
        actions={
          fromSchema &&
          p && (
            <Link to={managed ? managed.to : `/profiles/${p.id}/schema`}>
              <Button icon={<FileCode2 />}>{managed ? `Open ${managed.label.split(' → ')[1]}` : 'Open form'}</Button>
            </Link>
          )
        }
      />
      <Tabs className="mb-5" value={tab} onChange={setTab} items={[{ key: 'general', label: 'General' }, { key: 'scope', label: 'Scope' }, ...(isNew ? [] : [{ key: 'deployments' as Tab, label: 'Deployments' }])]} />

      {tab === 'general' && (
        <DetailsLayout
          aside={
            p && (
              <SummaryCard
                rows={[
                  { label: 'Level', value: <Badge>{p.level === 'user' ? 'User level' : 'Device level'}</Badge> },
                  { label: 'Scope', value: <ScopeSummary scope={p.scope} /> },
                  { label: 'Deployment', value: <DeploymentCounts status={p.status} /> },
                  { label: 'Last changed', value: p.updated_at ? <RelativeTime value={p.updated_at} /> : '' },
                ]}
              />
            )
          }
        >
          <div className="flex flex-col gap-6">
            {fromSchema && p && (
              <Alert tone="info" title={managed ? `Managed by ${managed.label}` : `Built from schema: ${schemaName}`}>
                {managed
                  ? 'This profile is generated from its own settings page. Change it there; editing the XML here would detach it from that page.'
                  : 'Use “Open form” to change its settings. You can still edit the XML below, but saving XML changes detaches the profile from the form.'}
              </Alert>
            )}
            <Card>
              <CardHeader title="General" />
              <CardBody>
                <FormGrid cols={2}>
                  <Field label="Name" hint="Defaults to the profile's PayloadDisplayName.">
                    <Input value={name} onChange={(e) => setName(e.target.value)} disabled={ro} placeholder={meta.displayName || 'Profile name'} />
                  </Field>
                  <Field label="Category">
                    <Input value={category} onChange={(e) => setCategory(e.target.value)} disabled={ro} />
                  </Field>
                  <Field label="Description" className="md:col-span-2">
                    <Textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} disabled={ro} placeholder={meta.description} />
                  </Field>
                </FormGrid>
              </CardBody>
            </Card>
            <Card>
              <CardHeader
                title="Payload"
                description="Upload a .mobileconfig or paste its XML. The server validates it, fills the identifier and UUID, and forces device scope unless PayloadScope is User."
                actions={
                  <>
                    {payload && (
                      <Button size="sm" icon={<Download />} onClick={() => saveText(payload, `${(name || meta.identifier || 'profile').replace(/[^\w.-]+/g, '_')}.mobileconfig`, 'application/x-apple-aspen-config')}>
                        Download
                      </Button>
                    )}
                    {!ro && (
                      <>
                        <input ref={fileRef} type="file" accept=".mobileconfig,.xml,.plist" className="hidden" onChange={(e) => onFile(e.target.files?.[0])} />
                        <Button size="sm" icon={<Upload />} onClick={() => fileRef.current?.click()}>
                          Upload .mobileconfig
                        </Button>
                        {payload && (
                          <Button size="sm" variant="ghost" icon={<X />} onClick={() => setPayload('')}>
                            Clear
                          </Button>
                        )}
                      </>
                    )}
                  </>
                }
              />
              <CardBody className="flex flex-col gap-4">
                {payload.trim() !== '' && !payloadLooksValid && (
                  <Alert tone="warning" title="This does not look like a Configuration profile">
                    The top-level PayloadType must be <code>Configuration</code> and a PayloadIdentifier is required. The server will reject invalid payloads.
                  </Alert>
                )}
                {!isNew && payload.trim() === '' && <Alert tone="info">Leave the payload empty to keep the existing one on save.</Alert>}
                {detaches && (
                  <Alert tone="warning" title="Saving will detach the form">
                    You changed the XML of a profile built from {managed ? managed.label : `the ${schemaName} schema`}. After saving, it can no longer be edited with the form.
                  </Alert>
                )}
                <FormGrid cols={3}>
                  <Field label="Identifier">
                    <Input readOnly mono value={meta.identifier ?? p?.identifier ?? ''} placeholder="Parsed from payload" />
                  </Field>
                  <Field label="UUID">
                    <Input readOnly mono value={meta.uuid ?? p?.uuid ?? ''} placeholder="Generated on save" />
                  </Field>
                  <Field label="Level">
                    <Input readOnly value={meta.scope ? (meta.scope.toLowerCase() === 'user' ? 'User' : 'Device') : p?.level ? (p.level === 'user' ? 'User' : 'Device') : ''} placeholder="Device" />
                  </Field>
                </FormGrid>
                <div
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={(e) => {
                    e.preventDefault()
                    if (!ro) onFile(e.dataTransfer.files?.[0])
                  }}
                >
                  <CodeEditor language="xml" value={payload} onChange={setPayload} height="440px" readOnly={ro} placeholder="Paste mobileconfig XML here or drop a file" />
                </div>
              </CardBody>
            </Card>
          </div>
        </DetailsLayout>
      )}

      {tab === 'scope' && (
        <Card>
          <CardHeader title="Scope" description="Scoped computers receive the profile when it is saved or deployed; computers that fall out of scope have it removed." />
          <CardBody>
            <ScopeEditor value={scope} onChange={setScope} disabled={ro} />
          </CardBody>
        </Card>
      )}

      {tab === 'deployments' && !isNew && id && <DeploymentsTab profileId={id} />}

      <ObjectBar
        ed={ed}
        isNew={isNew}
        listPath="/profiles"
        noun="profile"
        name={p?.name}
        saving={saving}
        onSave={save}
        onDelete={doDelete}
        deleteDescription="It is removed from every computer that has it installed, then deleted."
        onClone={
          // Uploaded XML can't be cloned; a profile built from a schema clones through its form.
          fromSchema && !managed && schemaId && p
            ? () =>
                navigate(`/profiles/custom-settings/${encodeURIComponent(schemaId)}/new`, {
                  state: { clone: { name: `Copy of ${p.name}`, description: p.description, category: p.category, values: (p.schema_values as SchemaValues | null) ?? {} } satisfies SchemaProfileClone },
                })
            : undefined
        }
        historyId={id}
      />
    </>
  )
}

export function DeploymentsTab({ profileId }: { profileId: string }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const deps = useProfileDeployments(profileId)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [busy, setBusy] = useState<string | null>(null)
  const [confirmRemove, setConfirmRemove] = useState(false)
  const rows = deps.data?.items ?? []

  const redeploy = async () => {
    setBusy('redeploy')
    try {
      const r = await api.post<{ installed: number; removed: number }>(`/api/v1/profiles/${profileId}/deploy`, { reinstall: true })
      toast.success('Redeploy queued', `${r.installed} install${r.installed === 1 ? '' : 's'}, ${r.removed} removal${r.removed === 1 ? '' : 's'}`)
      await invalidate(qk.profileDeployments(profileId), qk.profile(profileId), qk.profiles)
    } catch (e) {
      toast.error('Redeploy failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }
  const sync = async () => {
    setBusy('sync')
    try {
      const r = await api.post<{ installed: number; removed: number }>(`/api/v1/profiles/${profileId}/deploy`, { reinstall: false })
      toast.success('Scope synced', `${r.installed} install${r.installed === 1 ? '' : 's'}, ${r.removed} removal${r.removed === 1 ? '' : 's'}`)
      await invalidate(qk.profileDeployments(profileId), qk.profile(profileId), qk.profiles)
    } catch (e) {
      toast.error('Sync failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }
  const remove = async () => {
    setBusy('remove')
    try {
      const r = await api.post<{ removed: number }>(`/api/v1/profiles/${profileId}/remove`, { computer_ids: [...selected] })
      toast.success(`Removal queued on ${r.removed} computer${r.removed === 1 ? '' : 's'}`)
      setSelected(new Set())
      setConfirmRemove(false)
      await invalidate(qk.profileDeployments(profileId), qk.profile(profileId), qk.profiles)
    } catch (e) {
      toast.error('Remove failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }
  const toggleAll = (v: boolean) => setSelected(v ? new Set(rows.map((r) => r.computer_id)) : new Set())

  return (
    <Card>
      <CardHeader
        title="Deployments"
        description="Per-computer install status for this profile."
        actions={
          isAdmin && (
            <>
              {selected.size > 0 && (
                <Button /* ui-allow: delete-header-button (removes the profile from selected computers) */ variant="danger-outline" icon={<Trash2 />} onClick={() => setConfirmRemove(true)}>
                  Remove from {selected.size}
                </Button>
              )}
              <Button icon={<RefreshCw />} loading={busy === 'sync'} onClick={sync} title="Install on newly scoped computers and remove from unscoped ones">
                Sync scope
              </Button>
              <Button variant="primary" icon={<Upload />} loading={busy === 'redeploy'} onClick={redeploy} title="Reinstall on every scoped computer">
                Redeploy
              </Button>
            </>
          )
        }
      />
      <Table<ProfileDeployment>
        rows={rows}
        loading={deps.isPending}
        rowKey={(d) => d.computer_id}
        dense
        emptyTitle="Not deployed to any computer yet"
        emptyDescription="Add computers or groups on the Scope tab and save."
        selected={(d) => selected.has(d.computer_id)}
        columns={[
          ...(isAdmin
            ? [
                {
                  key: 'sel',
                  header: <Checkbox checked={rows.length > 0 && selected.size === rows.length} onChange={(e) => toggleAll(e.target.checked)} aria-label="Select all" />,
                  width: '40px',
                  render: (d: ProfileDeployment) => (
                    <Checkbox
                      checked={selected.has(d.computer_id)}
                      onChange={(e) => {
                        const n = new Set(selected)
                        if (e.target.checked) n.add(d.computer_id)
                        else n.delete(d.computer_id)
                        setSelected(n)
                      }}
                      aria-label="Select"
                    />
                  ),
                },
              ]
            : []),
          {
            key: 'computer',
            header: 'Computer',
            render: (d) => (
              <Link to={`/computers/${d.computer_id}/profiles`} className="font-medium text-ink hover:underline">
                {d.computer_name || d.computer_id}
              </Link>
            ),
          },
          { key: 'status', header: 'Status', render: (d) => <StatusBadge status={d.status} /> },
          { key: 'error', header: 'Error', render: (d) => <span className="text-xs text-critical break-words">{d.error}</span> },
          { key: 'cmd', header: 'Command', render: (d) => <span className="font-mono text-xs text-ink-muted">{d.command_uuid ? d.command_uuid.slice(0, 8) : '—'}</span> },
          { key: 'updated', header: 'Updated', render: (d) => <RelativeTime value={d.updated_at} className="text-ink-muted" /> },
        ]}
      />
      <ConfirmDialog
        open={confirmRemove}
        onClose={() => setConfirmRemove(false)}
        onConfirm={remove}
        destructive
        loading={busy === 'remove'}
        title={`Remove profile from ${selected.size} computer${selected.size === 1 ? '' : 's'}?`}
        description="A RemoveProfile command is sent to each selected computer. If they remain in scope, the next sync will reinstall it."
        confirmLabel="Remove profile"
      />
    </Card>
  )
}
