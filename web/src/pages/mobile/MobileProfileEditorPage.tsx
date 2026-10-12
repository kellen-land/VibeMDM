import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Download, RefreshCw, Save, Trash2, Upload } from 'lucide-react'
import { qk, useInvalidate, useMobilePayloads, useMobileProfile, useMobileProfileDeployments } from '../../api/queries'
import { api, ApiError, errorMessage, saveText } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import {
  emptyMobileScope,
  normalizeMobileScope,
  type MobileConfigProfile,
  type MobilePayloadErrors,
  type MobilePayloadValues,
  type MobileProfileDeployment,
  type MobileScope,
} from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { StatusBadge } from '../../components/ui/Badge'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Textarea } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { Tabs } from '../../components/ui/Tabs'
import { Table } from '../../components/ui/Table'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { LoadingBlock } from '../../components/ui/Spinner'
import { useToast } from '../../components/ui/Toast'
import { CodeEditor } from '../../components/CodeEditor'
import { PayloadEditor, VariablesHint, type PayloadSelection } from '../../components/profiles/PayloadEditor'
import { pruneValues } from '../../components/schema/schema'
import { MobileScopeEditor } from '../../components/pickers/MobileScopeEditor'

type Tab = 'options' | 'scope' | 'deployments'

const crumbs = [{ label: 'Mobile Devices', to: '/mobile-devices' }, { label: 'Configuration Profiles', to: '/mobile-profiles' }]

export function MobileProfileEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const profile = useMobileProfile(isNew ? undefined : id)
  const catalog = useMobilePayloads()
  const ro = !isAdmin

  const [tab, setTab] = useState<Tab>('options')
  const [selected, setSelected] = useState<PayloadSelection>('general')
  const [name, setName] = useState('')
  const [category, setCategory] = useState('')
  const [description, setDescription] = useState('')
  const [scope, setScope] = useState<MobileScope>(emptyMobileScope())
  const [payloads, setPayloads] = useState<MobilePayloadValues[]>([])
  /** Uploaded-profile mode: the XML is edited directly instead of payload editors. */
  const [uploaded, setUploaded] = useState(isNew && params.get('upload') === '1')
  const [xml, setXml] = useState('')
  const [errors, setErrors] = useState<MobilePayloadErrors[]>([])
  const [saving, setSaving] = useState(false)
  const [del, setDel] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  const seed = profile.data ? `${profile.data.id}|${profile.data.updated_at}` : ''
  useEffect(() => {
    const p = profile.data
    if (!p) return
    setName(p.name)
    setCategory(p.category)
    setDescription(p.description)
    setScope(normalizeMobileScope(p.scope))
    setPayloads(p.payloads ?? [])
    setUploaded(p.payloads === null)
    setXml(p.payload ?? '')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seed])

  const save = async () => {
    if (!uploaded && !name.trim()) {
      setSelected('general')
      toast.error('Enter a profile name')
      return
    }
    setSaving(true)
    setErrors([])
    try {
      const body = uploaded
        ? { name: name.trim(), description, category: category.trim(), scope, payload: xml }
        : { name: name.trim(), description, category: category.trim(), scope, payloads: payloads.map((p) => ({ type: p.type, uuid: p.uuid, values: pruneValues(p.values) })) }
      const out = isNew ? await api.post<MobileConfigProfile>('/api/v1/mobile-device-profiles', body) : await api.put<MobileConfigProfile>(`/api/v1/mobile-device-profiles/${id}`, body)
      await invalidate(qk.mobileProfiles, qk.mobileProfile(out.id), qk.mobileProfileDeployments(out.id))
      toast.success(isNew ? 'Profile created' : 'Profile saved', 'Devices in scope receive it at their next check-in.')
      if (isNew) navigate(`/mobile-profiles/${out.id}`, { replace: true })
    } catch (e) {
      const pe = e instanceof ApiError ? ((e.body as { payload_errors?: MobilePayloadErrors[] } | undefined)?.payload_errors ?? []) : []
      setErrors(pe)
      if (pe.length) setSelected(pe[0].index)
      toast.error('Save failed', pe.length ? 'Some values are invalid; they are marked in the form.' : errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  const doDelete = async () => {
    try {
      const r = await api.del<{ removed_from: number }>(`/api/v1/mobile-device-profiles/${id}`)
      toast.success('Profile deleted', r.removed_from ? `Removal queued on ${r.removed_from} device${r.removed_from === 1 ? '' : 's'}.` : undefined)
      await invalidate(qk.mobileProfiles)
      navigate('/mobile-profiles')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  const onFile = async (f: File | undefined) => {
    if (!f) return
    setXml(await f.text())
  }

  if (!isNew && profile.isPending) return <LoadingBlock />
  if (!isNew && (profile.error || !profile.data)) return <Alert tone="critical">{profile.error ? errorMessage(profile.error) : 'Profile not found'}</Alert>

  const title = isNew ? (uploaded ? 'Upload a profile' : 'New mobile profile') : profile.data!.name

  return (
    <>
      <PageHeader
        crumbs={[...crumbs, { label: title }]}
        title={title}
        meta={!isNew && <span className="font-mono text-xs text-ink-faint">{profile.data!.identifier}</span>}
        actions={
          isAdmin && (
            <>
              {!isNew && (
                <Button variant="ghost" icon={<Trash2 />} onClick={() => setDel(true)}>
                  Delete
                </Button>
              )}
              <Button variant="primary" icon={<Save />} loading={saving} onClick={save}>
                {isNew ? 'Create profile' : 'Save'}
              </Button>
            </>
          )
        }
      />
      <Tabs
        className="mb-5"
        value={tab}
        onChange={setTab}
        items={[{ key: 'options', label: 'Options' }, { key: 'scope', label: 'Scope' }, ...(isNew ? [] : [{ key: 'deployments' as Tab, label: 'Deployments' }])]}
      />

      {tab === 'options' && (
        <PayloadEditor
          catalog={catalog.data?.items ?? []}
          payloads={payloads}
          onChange={setPayloads}
          errors={errors}
          selected={selected}
          onSelect={setSelected}
          readOnly={ro}
          preview={{ path: '/api/v1/mobile-device-profiles/preview', body: { name, description } }}
          general={
            <Card>
              <CardHeader title="General" />
              <CardBody className="flex flex-col gap-4">
                <FormGrid cols={2}>
                  <Field label="Name" required={!uploaded} hint={uploaded ? 'Defaults to the profile’s PayloadDisplayName.' : undefined}>
                    <Input value={name} onChange={(e) => setName(e.target.value)} disabled={ro} autoFocus={isNew} />
                  </Field>
                  <Field label="Category">
                    <Input value={category} onChange={(e) => setCategory(e.target.value)} disabled={ro} placeholder="e.g. Network" />
                  </Field>
                  <Field label="Description" className="md:col-span-2" hint="Shown to the user in Settings → General → VPN & Device Management.">
                    <Textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} disabled={ro} />
                  </Field>
                </FormGrid>
                {!uploaded && <VariablesHint />}
              </CardBody>
            </Card>
          }
          xml={
            uploaded ? (
              <Card>
                <CardHeader
                  title="Profile XML"
                  description="A .mobileconfig made elsewhere (for example in Apple Configurator). VibeMDM keeps it as is, apart from the identifier and UUID."
                  actions={
                    <>
                      {xml && (
                        <Button size="sm" icon={<Download />} onClick={() => saveText(xml, `${(name || 'profile').replace(/[^\w.-]+/g, '_')}.mobileconfig`, 'application/x-apple-aspen-config')}>
                          Download
                        </Button>
                      )}
                      {!ro && (
                        <>
                          <input ref={fileRef} type="file" accept=".mobileconfig,.xml,.plist" className="hidden" onChange={(e) => onFile(e.target.files?.[0])} />
                          <Button size="sm" icon={<Upload />} onClick={() => fileRef.current?.click()}>
                            Choose file
                          </Button>
                        </>
                      )}
                    </>
                  }
                />
                <CardBody>
                  <CodeEditor language="xml" value={xml} onChange={setXml} height="440px" readOnly={ro} placeholder="Paste .mobileconfig XML here or choose a file" />
                </CardBody>
              </Card>
            ) : undefined
          }
        />
      )}

      {tab === 'scope' && (
        <Card>
          <CardHeader title="Scope" description="Devices in scope receive the profile when it is saved; devices that leave the scope have it removed." />
          <CardBody>
            <MobileScopeEditor value={scope} onChange={setScope} disabled={ro} />
          </CardBody>
        </Card>
      )}

      {tab === 'deployments' && !isNew && id && <Deployments profileId={id} />}

      <ConfirmDialog open={del} onClose={() => setDel(false)} onConfirm={doDelete} destructive title={`Delete "${name}"?`} description="The profile is removed from every device that has it, then deleted." confirmLabel="Delete profile" />
    </>
  )
}

function Deployments({ profileId }: { profileId: string }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const deps = useMobileProfileDeployments(profileId)
  const [busy, setBusy] = useState<string | null>(null)
  const run = async (reinstall: boolean) => {
    setBusy(reinstall ? 'redeploy' : 'sync')
    try {
      const r = await api.post<{ installed: number; removed: number }>(`/api/v1/mobile-device-profiles/${profileId}/deploy`, { reinstall })
      toast.success(reinstall ? 'Redeploy queued' : 'Scope synced', `${r.installed} install${r.installed === 1 ? '' : 's'}, ${r.removed} removal${r.removed === 1 ? '' : 's'}`)
      await invalidate(qk.mobileProfileDeployments(profileId), qk.mobileProfile(profileId), qk.mobileProfiles)
    } catch (e) {
      toast.error('Not queued', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }
  return (
    <Card>
      <CardHeader
        title="Deployments"
        description="Install status on each device."
        actions={
          isAdmin && (
            <>
              <Button size="sm" variant="ghost" loading={busy === 'redeploy'} onClick={() => run(true)} title="Send the profile again to every device in scope">
                Redeploy
              </Button>
              <Button size="sm" icon={<RefreshCw />} loading={busy === 'sync'} onClick={() => run(false)} title="Install on devices newly in scope and remove from devices out of scope">
                Sync scope
              </Button>
            </>
          )
        }
      />
      <Table<MobileProfileDeployment>
        rows={deps.data?.items ?? []}
        loading={deps.isPending}
        rowKey={(d) => d.mobile_device_id}
        dense
        emptyTitle="Not deployed to any device yet"
        emptyDescription="Add devices or groups on the Scope tab and save."
        columns={[
          {
            key: 'device',
            header: 'Device',
            render: (d) => (
              <Link to={`/mobile-devices/${d.mobile_device_id}/profiles`} className="font-medium text-ink hover:underline">
                {d.mobile_device_name || d.mobile_device_id}
              </Link>
            ),
          },
          { key: 'status', header: 'Status', render: (d) => <StatusBadge status={d.status} /> },
          { key: 'error', header: 'Error', render: (d) => <span className="break-words text-xs text-critical">{d.error}</span> },
          { key: 'updated', header: 'Updated', render: (d) => <RelativeTime value={d.updated_at} className="text-ink-muted" /> },
        ]}
      />
    </Card>
  )
}
