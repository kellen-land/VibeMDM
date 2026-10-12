import { useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Save, Trash2 } from 'lucide-react'
import { qk, useInvalidate, usePayloadEditors, useProfile } from '../../api/queries'
import { api, ApiError, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { emptyScope, normalizeScope, type ConfigProfile, type PayloadErrors, type PayloadValues, type Scope } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select, Textarea } from '../../components/ui/Input'
import { Tabs } from '../../components/ui/Tabs'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { useToast } from '../../components/ui/Toast'
import { ScopeEditor } from '../../components/pickers/ScopeEditor'
import { PayloadEditor, VariablesHint, type PayloadSelection } from '../../components/profiles/PayloadEditor'
import { pruneValues } from '../../components/schema/schema'
import { DeploymentCounts } from '../ProfilesPage'
import { DeploymentsTab } from '../ProfileEditorPage'

type Tab = 'options' | 'scope' | 'deployments'

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
  const ro = !isAdmin

  const tab = (sp.get('tab') as Tab) || 'options'
  const setTab = (t: Tab) => setSp(t === 'options' ? {} : { tab: t }, { replace: true })
  const [selected, setSelected] = useState<PayloadSelection>('general')
  const [name, setName] = useState('')
  const [category, setCategory] = useState('')
  const [description, setDescription] = useState('')
  const [level, setLevel] = useState<'device' | 'user'>('device')
  const [scope, setScope] = useState<Scope>(emptyScope())
  const [payloads, setPayloads] = useState<PayloadValues[]>([])
  const [errors, setErrors] = useState<PayloadErrors[]>([])
  const [saving, setSaving] = useState(false)
  const [del, setDel] = useState(false)

  const seed = profile.data ? `${profile.data.id}|${profile.data.updated_at}` : ''
  useEffect(() => {
    const p = profile.data
    if (!p) return
    setName(p.name)
    setCategory(p.category)
    setDescription(p.description)
    setLevel(p.level === 'user' ? 'user' : 'device')
    setScope(normalizeScope(p.scope))
    setPayloads(p.payloads ?? [])
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seed])

  const save = async () => {
    if (!name.trim()) {
      setSelected('general')
      setTab('options')
      toast.error('Enter a profile name')
      return
    }
    if (payloads.length === 0) {
      toast.error('Add at least one payload')
      return
    }
    setSaving(true)
    setErrors([])
    try {
      const body = { name: name.trim(), description, category: category.trim(), level, scope, payloads: payloads.map((p) => ({ type: p.type, uuid: p.uuid, values: pruneValues(p.values) })) }
      const out = isNew ? await api.post<ConfigProfile>('/api/v1/profiles', body) : await api.put<ConfigProfile>(`/api/v1/profiles/${id}`, body)
      await invalidate(qk.profiles, qk.profile(out.id), qk.profileDeployments(out.id), qk.dashboard)
      toast.success(isNew ? 'Profile created' : 'Profile saved', 'Computers in scope receive it at their next check-in.')
      if (isNew) navigate(`/profiles/${out.id}`, { replace: true })
    } catch (e) {
      const pe = e instanceof ApiError ? ((e.body as { payload_errors?: PayloadErrors[] } | undefined)?.payload_errors ?? []) : []
      setErrors(pe)
      if (pe.length) {
        setTab('options')
        setSelected(pe[0].index)
      }
      toast.error('Save failed', pe.length ? 'Some values are invalid; they are marked in the form.' : errorMessage(e))
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
        actions={
          isAdmin && (
            <>
              {!isNew && (
                <Button variant="ghost" icon={<Trash2 />} onClick={() => setDel(true)}>
                  Delete
                </Button>
              )}
              <Button variant="primary" icon={<Save />} loading={saving} onClick={save}>
                {isNew ? 'Create profile' : 'Save changes'}
              </Button>
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

      <ConfirmDialog open={del} onClose={() => setDel(false)} onConfirm={doDelete} destructive title={`Delete "${name}"?`} description="The profile is removed from every computer that has it installed, then deleted." confirmLabel="Delete profile" />
    </>
  )
}
