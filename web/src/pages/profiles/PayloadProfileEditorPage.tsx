import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { qk, useInvalidate, usePayloadEditors, useProfile } from '../../api/queries'
import { api, ApiError, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { emptyScope, normalizeScope, type ConfigProfile, type PayloadErrors, type PayloadValues, type Scope } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Badge } from '../../components/ui/Badge'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select, Textarea } from '../../components/ui/Input'
import { Tabs } from '../../components/ui/Tabs'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { useToast } from '../../components/ui/Toast'
import { ScopeEditor } from '../../components/pickers/ScopeEditor'
import { PayloadEditor, VariablesHint, type PayloadSelection } from '../../components/profiles/PayloadEditor'
import { pruneValues } from '../../components/schema/schema'
import { DeploymentCounts } from '../ProfilesPage'
import { DeploymentsTab } from '../ProfileEditorPage'
import { useCloneSource, useEditMode } from '../../components/editor/useEditMode'
import { ObjectBar } from '../../components/editor/ObjectBar'

type Tab = 'options' | 'scope' | 'deployments'

/** What Clone carries over to the new profile (scope is left empty so nothing deploys by surprise). */
export interface ProfileClone {
  name: string
  category: string
  description: string
  level?: 'device' | 'user'
  payloads: PayloadValues[]
}

/**
 * Copies of payloads for a cloned profile: each gets a fresh PayloadUUID, and values that
 * pointed at another payload of the profile (a certificate reference, say) follow it.
 */
export function clonePayloads(payloads: PayloadValues[]): PayloadValues[] {
  const map = new Map<string, string>()
  for (const p of payloads) if (p.uuid) map.set(p.uuid, crypto.randomUUID().toUpperCase())
  const remap = (v: unknown): unknown => {
    if (typeof v === 'string') return map.get(v) ?? v
    if (Array.isArray(v)) return v.map(remap)
    if (v && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, remap(x)]))
    return v
  }
  return payloads.map((p) => ({ ...p, uuid: p.uuid ? map.get(p.uuid) : undefined, values: remap(p.values) as PayloadValues['values'] }))
}

/** A Computer Configuration Profile built with Jamf-style payload editors. */
export function PayloadProfileEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const profile = useProfile(isNew ? undefined : id)
  const catalog = usePayloadEditors('macos')
  const clone = useCloneSource<ProfileClone>()
  const fromClone = isNew ? clone : undefined

  const tab = (sp.get('tab') as Tab) || 'options'
  const setTab = (t: Tab) => setSp(t === 'options' ? {} : { tab: t }, { replace: true })
  const [selected, setSelected] = useState<PayloadSelection>('general')
  const [name, setName] = useState(fromClone?.name ?? '')
  const [category, setCategory] = useState(fromClone?.category ?? '')
  const [description, setDescription] = useState(fromClone?.description ?? '')
  const [level, setLevel] = useState<'device' | 'user'>(fromClone?.level ?? 'device')
  const [scope, setScope] = useState<Scope>(emptyScope())
  const [payloads, setPayloads] = useState<PayloadValues[]>(fromClone?.payloads ?? [])
  const [errors, setErrors] = useState<PayloadErrors[]>([])
  const [saving, setSaving] = useState(false)

  const load = useCallback(() => {
    const p = profile.data
    if (!p) return
    setName(p.name)
    setCategory(p.category)
    setDescription(p.description)
    setLevel(p.level === 'user' ? 'user' : 'device')
    setScope(normalizeScope(p.scope))
    setPayloads(p.payloads ?? [])
    setErrors([])
  }, [profile.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: { name, category, description, level, scope, payloads }, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])
  const ro = ed.readOnly

  const save = async () => {
    if (!name.trim()) {
      setSelected('general')
      setTab('options')
      toast.error('Enter a profile name')
      return false
    }
    if (payloads.length === 0) {
      setTab('options')
      toast.error('Add at least one payload')
      return false
    }
    setSaving(true)
    setErrors([])
    try {
      const body = { name: name.trim(), description, category: category.trim(), level, scope, payloads: payloads.map((p) => ({ type: p.type, uuid: p.uuid, values: pruneValues(p.values) })) }
      const out = isNew ? await api.post<ConfigProfile>('/api/v1/profiles', body) : await api.put<ConfigProfile>(`/api/v1/profiles/${id}`, body)
      await invalidate(qk.profiles, qk.profile(out.id), qk.profileDeployments(out.id), qk.dashboard)
      toast.success(isNew ? 'Profile created' : 'Profile saved', 'Computers in scope receive it at their next check-in.')
      if (isNew) navigate(`/profiles/${out.id}`, { replace: true })
      return true
    } catch (e) {
      const pe = e instanceof ApiError ? ((e.body as { payload_errors?: PayloadErrors[] } | undefined)?.payload_errors ?? []) : []
      setErrors(pe)
      if (pe.length) {
        setTab('options')
        setSelected(pe[0].index)
      }
      toast.error('Save failed', pe.length ? 'Some values are invalid; they are marked in the form.' : errorMessage(e))
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
  if (!isNew && (profile.error || !profile.data)) return <Alert tone="critical">{profile.error ? errorMessage(profile.error) : 'Profile not found'}</Alert>
  const p = profile.data

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Configuration Profiles', to: '/profiles' }, { label: isNew ? 'New profile' : p?.name ?? '' }]}
        title={isNew ? 'New configuration profile' : p?.name}
        meta={
          p && (
            <>
              <Badge>{p.level === 'user' ? 'User level' : 'Computer level'}</Badge>
              <span className="font-mono text-xs text-ink-muted">{p.identifier}</span>
              <DeploymentCounts status={p.status} />
            </>
          )
        }
      />
      <Tabs className="mb-5" value={tab} onChange={setTab} items={[{ key: 'options', label: 'Options' }, { key: 'scope', label: 'Scope' }, ...(isNew ? [] : [{ key: 'deployments' as Tab, label: 'Deployments' }])]} />

      {tab === 'options' && (
        <PayloadEditor
          catalog={catalog.data?.items ?? []}
          payloads={payloads}
          onChange={setPayloads}
          errors={errors}
          selected={selected}
          onSelect={setSelected}
          readOnly={ro}
          level={level}
          preview={{ path: '/api/v1/profiles/preview', body: { name, description, level } }}
          general={
            <Card>
              <CardHeader title="General" />
              <CardBody className="flex flex-col gap-4">
                <FormGrid cols={2}>
                  <Field label="Name" required>
                    <Input value={name} onChange={(e) => setName(e.target.value)} disabled={ro} autoFocus={isNew} />
                  </Field>
                  <Field label="Category">
                    <Input value={category} onChange={(e) => setCategory(e.target.value)} disabled={ro} placeholder="e.g. Security" />
                  </Field>
                  <Field label="Level" hint={level === 'user' ? 'Installed for the user who logs in (needs a directory or MDM-enabled account).' : 'Applies to the whole computer.'}>
                    <Select
                      value={level}
                      onChange={(e) => setLevel(e.target.value as 'device' | 'user')}
                      disabled={ro}
                      options={[
                        { value: 'device', label: 'Computer level' },
                        { value: 'user', label: 'User level' },
                      ]}
                    />
                  </Field>
                  <Field label="Description" className="md:col-span-2" hint="Shown in System Settings → Device Management.">
                    <Textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} disabled={ro} />
                  </Field>
                </FormGrid>
                <VariablesHint />
              </CardBody>
            </Card>
          }
        />
      )}

      {tab === 'scope' && (
        <Card>
          <CardHeader title="Scope" description="Computers in scope receive the profile when it is saved; computers that leave the scope have it removed." />
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
        onClone={() => navigate('/profiles/new', { state: { clone: { name: `Copy of ${name}`, category, description, level, payloads: clonePayloads(payloads) } satisfies ProfileClone } })}
        historyId={id}
      />
    </>
  )
}
