import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { FileCode2 } from 'lucide-react'
import { qk, schemaPath, useCustomSchema, useInvalidate, useProfile } from '../../api/queries'
import { api, ApiError, errorMessage, fieldErrors } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { emptyScope, normalizeScope, type ConfigProfile, type SchemaFieldError, type SchemaValues, type Scope } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Textarea } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { useToast } from '../../components/ui/Toast'
import { ScopeEditor, ScopeSummary } from '../../components/pickers/ScopeEditor'
import { SchemaForm } from '../../components/schema/SchemaForm'
import { ProfileXmlPanel, useLivePreview } from '../../components/schema/ProfileXmlPanel'
import { managedProfiles, pruneValues, schemaIdFromSource } from '../../components/schema/schema'
import { customSettingsCrumb } from './CustomSettingsPage'
import { useCloneSource, useEditMode } from '../../components/editor/useEditMode'
import { ObjectBar } from '../../components/editor/ObjectBar'
import { DetailsLayout, SummaryCard } from '../../components/templates/DetailsLayout'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { DeploymentCounts } from '../ProfilesPage'

/** What Clone carries over to a new profile from the same schema (scope is left empty so nothing deploys by surprise). */
export interface SchemaProfileClone {
  name: string
  description: string
  category: string
  values: SchemaValues
}

/**
 * New profile from a schema (/profiles/custom-settings/:schemaId/new) or edit a
 * schema-built profile with the form (/profiles/:id/schema).
 */
export function SchemaProfileEditorPage() {
  const { schemaId, id } = useParams()
  // Keyed so going to another profile (or a clone) starts with fresh state.
  return <SchemaProfileEditor key={id ? `p:${id}` : `s:${schemaId}`} />
}

function SchemaProfileEditor() {
  const { schemaId: schemaParam, id: profileId } = useParams()
  const isNew = !profileId
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const profile = useProfile(profileId)
  const schemaId = isNew ? (schemaParam ?? null) : schemaIdFromSource(profile.data?.schema_source)
  const schema = useCustomSchema(schemaId)

  const clone = useCloneSource<SchemaProfileClone>()
  const fromClone = isNew ? clone : undefined

  const [name, setName] = useState(fromClone?.name ?? '')
  const [description, setDescription] = useState(fromClone?.description ?? '')
  const [category, setCategory] = useState(fromClone?.category || 'Applications')
  const [scope, setScope] = useState<Scope>(emptyScope())
  const [values, setValues] = useState<SchemaValues>(fromClone?.values ?? {})
  const [saveErrors, setSaveErrors] = useState<SchemaFieldError[]>([])
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const load = useCallback(() => {
    const p = profile.data
    if (!p) return
    setName(p.name)
    setDescription(p.description)
    setCategory(p.category || 'Applications')
    setScope(normalizeScope(p.scope))
    setValues((p.schema_values as SchemaValues | null) ?? {})
    setSaveErrors([])
    setSaveError(null)
  }, [profile.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: { name, description, category, scope, values }, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])
  const ro = ed.readOnly

  const preview = useLivePreview(schemaId, name || schema.data?.name || '', values, !!schema.data)
  const errors = preview.data?.errors?.length ? preview.data.errors : saveErrors

  const onValues = (v: SchemaValues) => {
    setValues(v)
    if (saveErrors.length) setSaveErrors([])
  }

  const save = async () => {
    if (!schemaId) return false
    if (!name.trim()) {
      setSaveError('Enter a profile name.')
      return false
    }
    setSaving(true)
    setSaveError(null)
    try {
      const body = { profile_id: profileId, name: name.trim(), description, category: category.trim() || 'Applications', scope, values: pruneValues(values) }
      const out = await api.post<ConfigProfile>(`${schemaPath(schemaId)}/profiles`, body)
      await invalidate(qk.profiles, qk.profile(out.id), qk.profileDeployments(out.id), qk.customSchemas, qk.dashboard)
      toast.success(isNew ? 'Profile created' : 'Profile saved', 'Deployment to scoped computers has been queued.')
      if (isNew) navigate(`/profiles/${out.id}`, { replace: true })
      return true
    } catch (e) {
      const fe = fieldErrors(e)
      setSaveErrors(fe)
      setSaveError(fe.length ? null : errorMessage(e))
      toast.error('Save failed', fe.length ? 'Some values are invalid; they are marked in the form.' : errorMessage(e))
      return false
    } finally {
      setSaving(false)
    }
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/profiles/${profileId}`)
      toast.success('Profile deleted')
      await invalidate(qk.profiles, qk.customSchemas, qk.dashboard)
      navigate('/profiles')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
      return false
    }
  }

  if (!isNew && profile.isPending) return <LoadingBlock />
  if (!isNew && (profile.error || !profile.data)) return <Alert tone="critical">{profile.error ? errorMessage(profile.error) : 'Profile not found'}</Alert>

  const p = profile.data
  const managed = p ? managedProfiles[p.identifier] : undefined
  const crumbs = [
    { label: 'Configuration profiles', to: '/profiles' },
    ...(isNew ? [customSettingsCrumb] : p ? [{ label: p.name, to: `/profiles/${p.id}` }] : []),
    { label: isNew ? `New ${schema.data?.name ?? ''} profile`.replace('  ', ' ') : 'Edit with form' },
  ]

  if (!isNew && p && !schemaId)
    return (
      <>
        <PageHeader crumbs={crumbs} title={p.name} />
        <Alert tone="info" title="This profile wasn’t built from a schema" actions={<Link to={`/profiles/${p.id}`}><Button size="sm">Open profile</Button></Link>}>
          Edit its XML on the profile page instead.
        </Alert>
      </>
    )

  if (managed)
    return (
      <>
        <PageHeader crumbs={crumbs} title={p?.name} />
        <Alert tone="info" title={`Managed by ${managed.label}`} actions={<Link to={managed.to}><Button size="sm">Open {managed.label.split(' → ')[1]}</Button></Link>}>
          This profile is generated by its own page. Change its settings there so they stay in sync.
        </Alert>
      </>
    )

  if (schema.isPending) return <LoadingBlock />
  if (schema.error || !schema.data)
    return (
      <>
        <PageHeader crumbs={crumbs} title={isNew ? 'New profile' : p?.name} />
        <Alert
          tone="warning"
          title={schema.error instanceof ApiError && schema.error.status === 404 ? 'The schema no longer exists' : 'Schema unavailable'}
          actions={
            p ? (
              <Link to={`/profiles/${p.id}`}>
                <Button size="sm">Edit XML instead</Button>
              </Link>
            ) : (
              <Link to="/profiles/custom-settings">
                <Button size="sm">Back to schemas</Button>
              </Link>
            )
          }
        >
          {schema.error instanceof ApiError && schema.error.status === 404
            ? 'The schema this profile was built from was deleted. The profile keeps its payload, which you can still edit as XML.'
            : errorMessage(schema.error)}
        </Alert>
      </>
    )

  const sd = schema.data

  return (
    <>
      <PageHeader
        crumbs={crumbs}
        title={isNew ? 'New configuration profile' : p?.name}
        description={sd.description || undefined}
        meta={
          // Existing profiles show these in the summary instead.
          !p && (
            <>
              <Badge icon={<FileCode2 />}>{sd.name}</Badge>
              {sd.builtin && <Badge tone="accent">Built-in</Badge>}
              <span className="font-mono text-xs text-ink-muted">{sd.preference_domain}</span>
            </>
          )
        }
      />
      <DetailsLayout
        aside={
          p && (
            <SummaryCard
              rows={[
                {
                  label: 'Schema',
                  value: (
                    <span className="inline-flex flex-wrap justify-end gap-1">
                      <Badge icon={<FileCode2 />}>{sd.name}</Badge>
                      {sd.builtin && <Badge tone="accent">Built-in</Badge>}
                    </span>
                  ),
                },
                { label: 'Domain', value: <span className="font-mono text-xs break-all">{sd.preference_domain}</span> },
                { label: 'Identifier', value: <span className="font-mono text-xs break-all">{p.identifier}</span> },
                { label: 'Scope', value: <ScopeSummary scope={p.scope} /> },
                { label: 'Deployment', value: <DeploymentCounts status={p.status} /> },
                { label: 'Last changed', value: p.updated_at ? <RelativeTime value={p.updated_at} /> : '' },
              ]}
            />
          )
        }
      >
        <div className="flex flex-col gap-6">
          {saveError && (
            <Alert tone="critical" title="The profile couldn’t be saved">
              {saveError}
            </Alert>
          )}
          <Card>
            <CardHeader title="General" />
            <CardBody>
              <FormGrid cols={2}>
                <Field label="Name" required>
                  <Input value={name} onChange={(e) => setName(e.target.value)} disabled={ro} placeholder={`e.g. ${sd.name}`} />
                </Field>
                <Field label="Category">
                  <Input value={category} onChange={(e) => setCategory(e.target.value)} disabled={ro} placeholder="Applications" />
                </Field>
                <Field label="Description" className="md:col-span-2">
                  <Textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} disabled={ro} />
                </Field>
              </FormGrid>
            </CardBody>
          </Card>

          <Card>
            <CardHeader title="Scope" description="Scoped computers receive the profile when it is saved; computers that fall out of scope have it removed." />
            <CardBody>
              <ScopeEditor value={scope} onChange={setScope} disabled={ro} />
            </CardBody>
          </Card>

          <Card>
            <CardHeader
              title="Preferences"
              description={
                <>
                  Managed preferences for <code className="font-mono">{sd.preference_domain}</code>. Only settings you configure are included; everything else keeps the app&apos;s default.
                </>
              }
            />
            <CardBody>
              <SchemaForm schema={sd.schema} value={values} onChange={onValues} errors={errors} disabled={ro} />
            </CardBody>
          </Card>

          <ProfileXmlPanel preview={preview} />
        </div>
      </DetailsLayout>

      <ObjectBar
        ed={ed}
        isNew={isNew}
        listPath={isNew ? '/profiles/custom-settings' : `/profiles/${profileId}`}
        noun="profile"
        name={p?.name}
        saving={saving}
        onSave={save}
        onDelete={isNew ? undefined : doDelete}
        deleteDescription="It is removed from every computer that has it installed, then deleted."
        onClone={
          schemaId
            ? () =>
                navigate(`/profiles/custom-settings/${encodeURIComponent(schemaId)}/new`, {
                  state: { clone: { name: `Copy of ${name}`, description, category, values } satisfies SchemaProfileClone },
                })
            : undefined
        }
        historyId={profileId}
      />
    </>
  )
}
