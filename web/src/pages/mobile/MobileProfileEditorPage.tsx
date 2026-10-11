import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Download, Plus, RefreshCw, Save, Trash2, Upload, X } from 'lucide-react'
import { qk, useInvalidate, useMobilePayloads, useMobileProfile, useMobileProfileDeployments } from '../../api/queries'
import { api, ApiError, errorMessage, saveText } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import {
  emptyMobileScope,
  normalizeMobileScope,
  type MobileConfigProfile,
  type MobilePayloadDef,
  type MobilePayloadErrors,
  type MobilePayloadValues,
  type MobileProfileDeployment,
  type MobileProfilePreview,
  type MobileScope,
  type SchemaValues,
} from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge, StatusBadge } from '../../components/ui/Badge'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Textarea } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { Tabs } from '../../components/ui/Tabs'
import { Modal } from '../../components/ui/Modal'
import { Table } from '../../components/ui/Table'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { LoadingBlock } from '../../components/ui/Spinner'
import { CodeBlock } from '../../components/ui/CopyButton'
import { useToast } from '../../components/ui/Toast'
import { CodeEditor } from '../../components/CodeEditor'
import { SchemaForm } from '../../components/schema/SchemaForm'
import { pruneValues } from '../../components/schema/schema'
import { MobileScopeEditor } from '../../components/pickers/MobileScopeEditor'
import { cn } from '../../lib/cn'

type Tab = 'options' | 'scope' | 'deployments'
/** 'general', or the index of a payload in the profile. */
type Selected = 'general' | number

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
  const defs = useMemo(() => new Map((catalog.data?.items ?? []).map((d) => [d.type, d])), [catalog.data])
  const ro = !isAdmin

  const [tab, setTab] = useState<Tab>('options')
  const [selected, setSelected] = useState<Selected>('general')
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
  const [adding, setAdding] = useState(false)
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

  const errorsFor = (i: number) => errors.find((e) => e.index === i)?.errors ?? []

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
        : { name: name.trim(), description, category: category.trim(), scope, payloads: payloads.map((p) => ({ type: p.type, values: pruneValues(p.values) })) }
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

  const addPayload = (d: MobilePayloadDef) => {
    setPayloads([...payloads, { type: d.type, values: {} }])
    setSelected(payloads.length)
    setAdding(false)
  }
  const removePayload = (i: number) => {
    setPayloads(payloads.filter((_, j) => j !== i))
    setErrors([])
    setSelected('general')
  }
  const onFile = async (f: File | undefined) => {
    if (!f) return
    setXml(await f.text())
  }

  if (!isNew && profile.isPending) return <LoadingBlock />
  if (!isNew && (profile.error || !profile.data)) return <Alert tone="critical">{profile.error ? errorMessage(profile.error) : 'Profile not found'}</Alert>

  const title = isNew ? (uploaded ? 'Upload a profile' : 'New mobile profile') : profile.data!.name
  const sel = typeof selected === 'number' ? payloads[selected] : undefined
  const selDef = sel ? defs.get(sel.type) : undefined

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
        <div className="grid gap-6 lg:grid-cols-[15rem_1fr]">
          <nav aria-label="Payloads" className="flex flex-col gap-1">
            <SideItem active={selected === 'general'} onClick={() => setSelected('general')} label="General" />
            {uploaded ? (
              <SideItem active={selected === 0} onClick={() => setSelected(0)} label="Profile XML" />
            ) : (
              payloads.map((p, i) => (
                <SideItem key={i} active={selected === i} onClick={() => setSelected(i)} label={defs.get(p.type)?.title ?? p.type} hint={payloadHint(p)} error={errorsFor(i).length > 0} />
              ))
            )}
            {!uploaded && !ro && (
              <Button size="sm" variant="ghost" icon={<Plus />} className="mt-2 justify-start" onClick={() => setAdding(true)}>
                Add payload
              </Button>
            )}
          </nav>

          <div className="flex min-w-0 flex-col gap-6">
            {selected === 'general' && (
              <Card>
                <CardHeader title="General" />
                <CardBody>
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
                  {!uploaded && payloads.length === 0 && !ro && (
                    <div className="mt-5 flex items-center justify-between rounded-lg border border-dashed border-line px-4 py-3">
                      <p className="text-sm text-ink-muted">Next, add the settings this profile carries.</p>
                      <Button size="sm" icon={<Plus />} onClick={() => setAdding(true)}>
                        Add payload
                      </Button>
                    </div>
                  )}
                </CardBody>
              </Card>
            )}

            {uploaded && selected === 0 && (
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
            )}

            {sel && (
              <Card>
                <CardHeader
                  title={selDef?.title ?? sel.type}
                  description={selDef?.description}
                  actions={
                    !ro && (
                      <Button size="sm" variant="ghost" icon={<X />} onClick={() => removePayload(selected as number)}>
                        Remove
                      </Button>
                    )
                  }
                />
                <CardBody>
                  {selDef ? (
                    <SchemaForm
                      key={selected}
                      schema={selDef.schema}
                      value={sel.values}
                      onChange={(v: SchemaValues) => setPayloads(payloads.map((p, i) => (i === selected ? { ...p, values: v } : p)))}
                      errors={errorsFor(selected as number)}
                      disabled={ro}
                    />
                  ) : (
                    <Alert tone="warning">This payload type has no editor in this version of VibeMDM.</Alert>
                  )}
                </CardBody>
              </Card>
            )}

            {!uploaded && payloads.length > 0 && <XmlPreview name={name} description={description} payloads={payloads} />}
          </div>
        </div>
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

      <Modal open={adding} onClose={() => setAdding(false)} title="Add a payload" size="lg">
        <PayloadCatalog defs={catalog.data?.items ?? []} used={payloads.map((p) => p.type)} onPick={addPayload} />
      </Modal>
      <ConfirmDialog open={del} onClose={() => setDel(false)} onConfirm={doDelete} destructive title={`Delete "${name}"?`} description="The profile is removed from every device that has it, then deleted." confirmLabel="Delete profile" />
    </>
  )
}

function SideItem({ active, onClick, label, hint, error }: { active: boolean; onClick: () => void; label: string; hint?: string; error?: boolean }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? 'page' : undefined}
      className={cn('rounded-md px-2 py-1.5 text-left text-[13px] focus-ring', active ? 'bg-accent-500/12 font-medium text-ink' : 'text-ink-muted hover:bg-line/40 hover:text-ink')}
    >
      <span className="flex items-center gap-2">
        <span className="truncate">{label}</span>
        {error && <span className="size-1.5 shrink-0 rounded-full bg-critical" aria-label="has errors" />}
      </span>
      {hint && <span className="block truncate text-xs text-ink-faint">{hint}</span>}
    </button>
  )
}

/** A short hint so several payloads of one type can be told apart. */
function payloadHint(p: MobilePayloadValues): string | undefined {
  const v = p.values as Record<string, unknown>
  for (const k of ['PayloadDisplayName', 'SSID_STR', 'UserDefinedName', 'Label', 'PayloadCertificateFileName']) {
    if (typeof v[k] === 'string' && v[k]) return v[k] as string
  }
  return undefined
}

function PayloadCatalog({ defs, used, onPick }: { defs: MobilePayloadDef[]; used: string[]; onPick: (d: MobilePayloadDef) => void }) {
  const categories = [...new Set(defs.map((d) => d.category))]
  return (
    <div className="flex flex-col gap-5">
      {categories.map((c) => (
        <section key={c}>
          <h3 className="mb-1.5 text-xs font-medium uppercase tracking-wide text-ink-faint">{c}</h3>
          <div className="divide-y divide-line rounded-lg border border-line">
            {defs
              .filter((d) => d.category === c)
              .map((d) => {
                const taken = !d.multiple && used.includes(d.type)
                return (
                  <button key={d.type} type="button" disabled={taken} onClick={() => onPick(d)} className="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-surface-sunken disabled:cursor-default disabled:opacity-50 disabled:hover:bg-transparent">
                    <span className="min-w-0 flex-1">
                      <span className="block text-sm font-medium text-ink">{d.title}</span>
                      <span className="block text-xs text-ink-muted">{d.description}</span>
                    </span>
                    {taken && <Badge>Added</Badge>}
                  </button>
                )
              })}
          </div>
        </section>
      ))}
    </div>
  )
}

/** Folded-away preview of the generated .mobileconfig, built by the server on demand. */
function XmlPreview({ name, description, payloads }: { name: string; description: string; payloads: MobilePayloadValues[] }) {
  const [open, setOpen] = useState(false)
  const [preview, setPreview] = useState<MobileProfilePreview | null>(null)
  const body = JSON.stringify({ name, description, payloads: payloads.map((p) => ({ type: p.type, values: pruneValues(p.values) })) })
  useEffect(() => {
    if (!open) return
    const t = setTimeout(() => {
      api
        .post<MobileProfilePreview>('/api/v1/mobile-device-profiles/preview', JSON.parse(body))
        .then(setPreview)
        .catch((e) => setPreview({ payload: '', error: errorMessage(e), payload_errors: [] }))
    }, 400)
    return () => clearTimeout(t)
  }, [open, body])
  return (
    <details className="rounded-lg border border-line" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}>
      <summary className="cursor-pointer select-none px-4 py-2.5 text-sm font-medium text-ink-muted hover:text-ink">Show profile XML</summary>
      {open && (
        <div className="px-4 pb-4">
          {preview?.error && <Alert tone="warning">{preview.error}</Alert>}
          {preview && preview.payload_errors.length > 0 && <Alert tone="warning">Some values are invalid. Fix them to see the XML.</Alert>}
          {preview?.payload && <CodeBlock text={preview.payload} filename="profile.mobileconfig" maxHeight="28rem" />}
        </div>
      )}
    </details>
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
