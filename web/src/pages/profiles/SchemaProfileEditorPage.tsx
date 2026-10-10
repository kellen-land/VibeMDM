import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { FileCode2, Save } from 'lucide-react'
import { qk, schemaPath, useCustomSchema, useInvalidate, useProfile } from '../../api/queries'
import { api, ApiError, errorMessage, fieldErrors } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { emptyScope, normalizeScope, type ConfigProfile, type SchemaFieldError, type SchemaValues, type Scope } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Textarea } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { useToast } from '../../components/ui/Toast'
import { ScopeEditor } from '../../components/pickers/ScopeEditor'
import { SchemaForm } from '../../components/schema/SchemaForm'
import { ProfileXmlPanel, useLivePreview } from '../../components/schema/ProfileXmlPanel'
import { managedProfiles, pruneValues, schemaIdFromSource } from '../../components/schema/schema'
import { customSettingsCrumb } from './CustomSettingsPage'

/**
 * New profile from a schema (/profiles/custom-settings/:schemaId/new) or edit a
 * schema-built profile with the form (/profiles/:id/schema).
 */
export function SchemaProfileEditorPage() {
  const { schemaId: schemaParam, id: profileId } = useParams()
  const isNew = !profileId
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const profile = useProfile(profileId)
  const schemaId = isNew ? (schemaParam ?? null) : schemaIdFromSource(profile.data?.schema_source)
  const schema = useCustomSchema(schemaId)

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState('Applications')
  const [scope, setScope] = useState<Scope>(emptyScope())
  const [values, setValues] = useState<SchemaValues>({})
  const [saveErrors, setSaveErrors] = useState<SchemaFieldError[]>([])
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const ro = !isAdmin

  // Seed from the profile once per server version, not on every refetch.
  const seed = profile.data ? `${profile.data.id}|${profile.data.updated_at}` : ''
  useEffect(() => {
    const p = profile.data
    if (!p) return
    setName(p.name)
    setDescription(p.description)
    setCategory(p.category || 'Applications')
    setScope(normalizeScope(p.scope))
    setValues((p.schema_values as SchemaValues | null) ?? {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seed])

  const preview = useLivePreview(schemaId, name || schema.data?.name || '', values, !!schema.data)
  const errors = preview.data?.errors?.length ? preview.data.errors : saveErrors

  const onValues = (v: SchemaValues) => {
    setValues(v)
    if (saveErrors.length) setSaveErrors([])
  }

  const save = async () => {
    if (!schemaId) return
    if (!name.trim()) {
      setSaveError('Enter a profile name.')
      return
    }
    setSaving(true)
    setSaveError(null)
    try {
      const body = { profile_id: profileId, name: name.trim(), description, category: category.trim() || 'Applications', scope, values: pruneValues(values) }
      const out = await api.post<ConfigProfile>(`${schemaPath(schemaId)}/profiles`, body)
      await invalidate(qk.profiles, qk.profile(out.id), qk.profileDeployments(out.id), qk.customSchemas, qk.dashboard)
      toast.success(isNew ? 'Profile created' : 'Profile saved', 'Deployment to scoped computers has been queued.')
      navigate(`/profiles/${out.id}`)
    } catch (e) {
      const fe = fieldErrors(e)
      setSaveErrors(fe)
      setSaveError(fe.length ? null : errorMessage(e))
      toast.error('Save failed', fe.length ? 'Some values are invalid; they are marked in the form.' : errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  if (!isNew && profile.isPending) return <LoadingBlock />
  if (!isNew && (profile.error || !profile.data)) return <Alert tone="critical">{profile.error ? errorMessage(profile.error) : 'Profile not found'}</Alert>

  const p = profile.data
  const managed = p ? managedProfiles[p.identifier] : undefined
  const crumbs = [
    { label: 'Configuration Profiles', to: '/profiles' },
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
  const saveButton = (
    <Button variant="primary" icon={<Save />} loading={saving} onClick={save}>
      {isNew ? 'Create profile' : 'Save changes'}
    </Button>
  )

  return (
    <>
      <PageHeader
        crumbs={crumbs}
        title={isNew ? 'New configuration profile' : p?.name}
        description={sd.description || undefined}
        meta={
          <>
            <Badge icon={<FileCode2 />}>{sd.name}</Badge>
            {sd.builtin && <Badge tone="accent">Built-in</Badge>}
            <span className="font-mono text-xs text-ink-muted">{sd.preference_domain}</span>
            {p && <span className="font-mono text-xs text-ink-faint">{p.identifier}</span>}
          </>
        }
        actions={isAdmin && saveButton}
      />
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
          {isAdmin && <CardFooter>{saveButton}</CardFooter>}
        </Card>

        <ProfileXmlPanel preview={preview} />
      </div>
    </>
  )
}
