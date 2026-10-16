import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ChevronRight, ExternalLink, FileKey2, Globe, ShieldCheck, X } from 'lucide-react'
import { qk, useCustomSchema, useInvalidate, useXCreds } from '../../api/queries'
import { api, errorMessage, fieldErrors } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { emptyScope, normalizeScope, type SchemaFieldError, type SchemaValues, type Scope, type XCredsSaveRequest, type XCredsView } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge, StatusBadge } from '../../components/ui/Badge'
import { Checkbox, Toggle } from '../../components/ui/Checkbox'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { useToast } from '../../components/ui/Toast'
import { ScopeEditor } from '../../components/pickers/ScopeEditor'
import { SchemaForm } from '../../components/schema/SchemaForm'
import { ProfileXmlPanel, useLivePreview } from '../../components/schema/ProfileXmlPanel'
import { errorsAt, fileToBase64, pruneValues } from '../../components/schema/schema'
import { cn } from '../../lib/cn'
import { useEditMode } from '../../components/editor/useEditMode'
import { ObjectBar } from '../../components/editor/ObjectBar'

const XCREDS_SCHEMA = 'builtin:com.twocanoes.xcreds'

type Provider = 'entra' | 'google' | 'okta' | 'other'

const providers: { key: Provider; title: string; text: string }[] = [
  { key: 'entra', title: 'Microsoft Entra ID', text: 'Azure AD / Microsoft 365 tenants.' },
  { key: 'google', title: 'Google Workspace', text: 'Google accounts in your Workspace domain.' },
  { key: 'okta', title: 'Okta', text: 'An Okta org or custom domain.' },
  { key: 'other', title: 'Other OpenID Connect', text: 'Enter the discovery URL yourself.' },
]

const entraURL = (tenant: string) => `https://login.microsoftonline.com/${tenant}/v2.0/.well-known/openid-configuration`
const GOOGLE_URL = 'https://accounts.google.com/.well-known/openid-configuration'
const oktaURL = (domain: string) => `https://${domain.replace(/^https?:\/\//, '').replace(/\/+$/, '')}/.well-known/openid-configuration`

/** Work out the preset (and its tenant/domain) from an existing discovery URL. */
function detectProvider(url: string): { provider: Provider; detail: string } {
  const entra = /^https:\/\/login\.microsoftonline\.com\/([^/]+)\/v2\.0\/\.well-known\/openid-configuration$/.exec(url)
  if (entra) return { provider: 'entra', detail: entra[1] }
  if (url === GOOGLE_URL) return { provider: 'google', detail: '' }
  const okta = /^https:\/\/([^/]*okta[^/]*)\/\.well-known\/openid-configuration$/i.exec(url)
  if (okta) return { provider: 'okta', detail: okta[1] }
  return { provider: url ? 'other' : 'entra', detail: '' }
}

interface Form {
  enabled: boolean
  install_app: boolean
  scope: Scope
  values: SchemaValues
  /** New license (base64), or "" to keep the stored one. */
  license: string
  licenseName: string
}

function toForm(v: XCredsView): Form {
  const values = { ...(v.values ?? {}) }
  delete values.LicenseFile
  return {
    enabled: v.enabled,
    install_app: v.app ? v.app.enabled || !v.enabled : true,
    scope: normalizeScope(v.scope),
    values,
    license: '',
    licenseName: '',
  }
}

export function XCredsPage() {
  const { isAdmin } = useAuth()
  const q = useXCreds()
  const schema = useCustomSchema(XCREDS_SCHEMA)
  const toast = useToast()
  const invalidate = useInvalidate()
  const [f, setF] = useState<Form>({ enabled: false, install_app: true, scope: emptyScope(), values: {}, license: '', licenseName: '' })
  const [provider, setProvider] = useState<Provider>('entra')
  const [detail, setDetail] = useState('')
  const [advanced, setAdvanced] = useState(false)
  const [saveErrors, setSaveErrors] = useState<SchemaFieldError[]>([])
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  // The GET polls for app status; it only refreshes the form while viewing, so edits aren't overwritten.
  const load = useCallback(() => {
    if (!q.data) return
    setF(toForm(q.data))
    const d = detectProvider(String(q.data.values?.discoveryURL ?? ''))
    setProvider(d.provider)
    setDetail(d.detail)
    setSaveError(null)
    setSaveErrors([])
  }, [q.data])
  const ed = useEditMode({ isNew: false, canEdit: isAdmin, snapshot: f, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])
  const ro = ed.readOnly

  const previewValues = f.license ? { ...f.values, LicenseFile: f.license } : f.values
  const preview = useLivePreview(XCREDS_SCHEMA, 'XCreds cloud login', previewValues, !!schema.data && advanced)
  const errors = saveErrors.length ? saveErrors : (preview.data?.errors ?? [])
  const set = (p: Partial<Form>) => setF((cur) => ({ ...cur, ...p }))
  const setValue = (k: string, v: unknown) =>
    setF((cur) => {
      const values = { ...cur.values }
      if (v === undefined || v === '') delete values[k]
      else values[k] = v
      return { ...cur, values }
    })
  const str = (k: string) => (typeof f.values[k] === 'string' ? (f.values[k] as string) : '')

  const pickProvider = (p: Provider) => {
    setProvider(p)
    setDetail('')
    if (p === 'google') setValue('discoveryURL', GOOGLE_URL)
  }
  const fillFromDetail = (val: string) => {
    setDetail(val)
    const t = val.trim()
    if (!t) return
    if (provider === 'entra') setValue('discoveryURL', entraURL(t))
    if (provider === 'okta') setValue('discoveryURL', oktaURL(t))
  }

  const save = async () => {
    setSaving(true)
    setSaveError(null)
    const values = pruneValues(f.values)
    if (f.license) values.LicenseFile = f.license
    const body: XCredsSaveRequest = { enabled: f.enabled, scope: f.scope, values, install_app: f.install_app }
    try {
      const out = await api.put<XCredsView>('/api/v1/identity/xcreds', body)
      setF(toForm(out))
      setSaveErrors([])
      await invalidate(qk.xcreds, qk.profiles, qk.apps)
      toast.success('XCreds settings saved', out.enabled ? 'Macs in scope get the settings profile and the app at their next check-in.' : 'XCreds is off; its profile is removed from every Mac.')
      return true
    } catch (e) {
      const fe = fieldErrors(e)
      setSaveErrors(fe)
      setSaveError(errorMessage(e))
      if (fe.some((x) => !['discoveryURL', 'clientID', 'clientSecret'].includes(x.path))) setAdvanced(true)
      toast.error('Save failed', errorMessage(e))
      return false
    } finally {
      setSaving(false)
    }
  }

  if (q.isPending) return <LoadingBlock />
  if (q.error || !q.data) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'XCreds settings unavailable'}</Alert>
  const v = q.data
  const fieldErr = (k: string) => errorsAt(errors, k)[0]

  return (
    <>
      <PageHeader
        title="Identity"
        description="Sign in to Macs with your cloud identity provider using XCreds."
        meta={
          <>
            <Badge tone={v.enabled ? 'good' : 'neutral'} dot>
              {v.enabled ? 'XCreds enabled' : 'XCreds off'}
            </Badge>
            {v.has_license ? (
              <Badge tone="good" icon={<ShieldCheck />}>
                License stored
              </Badge>
            ) : (
              <Badge tone="warning">No license</Badge>
            )}
          </>
        }
      />
      <div className="flex flex-col gap-6">
        <Alert tone="warning" title="Test on a lab Mac first — XCreds replaces the login window.">
          Scope XCreds to one test Mac, confirm you can sign in with a cloud account and with a local account, then widen the scope.
        </Alert>
        {saveError && (
          <Alert tone="critical" title="The settings couldn’t be saved">
            {saveError}
          </Alert>
        )}

        <Card>
          <CardHeader title="About XCreds" />
          <CardBody className="flex flex-col gap-2 text-[13px] leading-relaxed text-ink-muted">
            <p>
              XCreds replaces the macOS login window with a sign-in page from your identity provider (Microsoft Entra ID, Google Workspace, Okta or any OpenID Connect provider) and keeps
              the Mac&apos;s local account password in sync with the cloud password.
            </p>
            <p>
              VibeMDM installs XCreds with Installomator and configures it with a managed-preferences profile, &ldquo;XCreds cloud login&rdquo;. XCreds needs a Twocanoes license after a
              14-day trial.
            </p>
            <p className="text-xs">
              <a href={v.project} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-1 text-accent-700 hover:underline dark:text-accent-300">
                XCreds by Twocanoes — {v.license || 'BSD-3-Clause'}
                <ExternalLink className="size-3" aria-hidden="true" />
              </a>
            </p>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Quick setup" description="Register an app with your identity provider, then enter its details here." />
          <CardBody className="flex flex-col gap-5">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" role="radiogroup" aria-label="Identity provider">
              {providers.map((p) => {
                const active = provider === p.key
                return (
                  <button
                    key={p.key}
                    type="button"
                    role="radio"
                    aria-checked={active}
                    disabled={ro}
                    onClick={() => pickProvider(p.key)}
                    className={cn('flex items-start gap-3 rounded-lg border p-3.5 text-left transition-colors focus-ring disabled:cursor-not-allowed', active ? 'border-accent-500/60 bg-accent-500/8' : 'border-line hover:border-line-strong')}
                  >
                    <Globe className={cn('mt-0.5 size-4 shrink-0', active ? 'text-accent-400' : 'text-ink-faint')} aria-hidden="true" />
                    <span className="min-w-0">
                      <span className="block text-sm font-medium text-ink">{p.title}</span>
                      <span className="mt-1 block text-xs text-ink-muted">{p.text}</span>
                    </span>
                  </button>
                )
              })}
            </div>

            {provider === 'entra' && (
              <Field label="Tenant ID" hint="Your directory (tenant) ID or primary domain. Fills the discovery URL below.">
                <Input mono value={detail} onChange={(e) => fillFromDetail(e.target.value)} disabled={ro} placeholder="00000000-0000-0000-0000-000000000000" spellCheck={false} className="max-w-md" />
              </Field>
            )}
            {provider === 'okta' && (
              <Field label="Okta domain" hint="Fills the discovery URL below.">
                <Input mono value={detail} onChange={(e) => fillFromDetail(e.target.value)} disabled={ro} placeholder="example.okta.com" spellCheck={false} className="max-w-md" />
              </Field>
            )}
            {provider === 'google' && (
              <Alert tone="info" title="Google Workspace needs a client secret">
                <p>Create an OAuth client of type &ldquo;Web application&rdquo; and enter both its client ID and secret.</p>
                <p className="mt-1">
                  Also turn on <code className="font-mono">shouldSetGoogleAccessTypeToOffline</code> so XCreds gets a refresh token and can keep checking the password.{' '}
                  {f.values.shouldSetGoogleAccessTypeToOffline === true ? (
                    <span className="font-medium">It is on.</span>
                  ) : (
                    !ro && (
                      <button type="button" onClick={() => setValue('shouldSetGoogleAccessTypeToOffline', true)} className="font-medium underline focus-ring rounded">
                        Turn it on
                      </button>
                    )
                  )}
                </p>
              </Alert>
            )}

            <FormGrid cols={1}>
              <Field label="Discovery URL" required error={fieldErr('discoveryURL')} hint="The provider's OpenID Connect discovery document (discoveryURL).">
                <Input mono value={str('discoveryURL')} onChange={(e) => setValue('discoveryURL', e.target.value)} disabled={ro} placeholder="https://…/.well-known/openid-configuration" spellCheck={false} />
              </Field>
            </FormGrid>
            <FormGrid cols={2}>
              <Field label="Client ID" required error={fieldErr('clientID')} hint="clientID from your app registration.">
                <Input mono value={str('clientID')} onChange={(e) => setValue('clientID', e.target.value)} disabled={ro} autoComplete="off" spellCheck={false} />
              </Field>
              <Field label="Client secret" required={provider === 'google'} error={fieldErr('clientSecret')} hint={provider === 'google' ? 'Required for Google.' : 'clientSecret; only needed if your provider requires one.'}>
                <Input type="password" mono value={str('clientSecret')} onChange={(e) => setValue('clientSecret', e.target.value)} disabled={ro} autoComplete="new-password" spellCheck={false} />
              </Field>
            </FormGrid>
          </CardBody>
        </Card>

        <LicenseCard v={v} f={f} set={set} ro={ro} error={fieldErr('LicenseFile')} />

        <Card>
          <CardHeader title="Deployment" description="The XCreds app and its settings profile always share this scope." />
          <CardBody className="flex flex-col gap-5">
            <div className="flex flex-col gap-3">
              <Toggle checked={f.enabled} onChange={(enabled) => set({ enabled })} disabled={ro} label="Enable XCreds" description="When off, the settings profile is removed from every Mac and the app is no longer installed." />
              <Checkbox checked={f.install_app} onChange={(e) => set({ install_app: e.target.checked })} disabled={ro} label="Install XCreds with Installomator" description="Uncheck if you deploy the XCreds app another way; VibeMDM then only delivers the settings." />
            </div>
            <div>
              <p className="mb-2 text-sm font-medium text-ink">Scope</p>
              <ScopeEditor value={f.scope} onChange={(scope) => set({ scope })} disabled={ro} />
            </div>
            <AppStatus v={v} />
          </CardBody>
        </Card>

        <Card>
          <button type="button" onClick={() => setAdvanced((o) => !o)} aria-expanded={advanced} className={cn('flex w-full items-center gap-2 px-4 py-3 text-left focus-ring rounded-xl', advanced && 'border-b border-line rounded-b-none')}>
            <ChevronRight className={cn('size-4 shrink-0 text-ink-faint transition-transform', advanced && 'rotate-90')} aria-hidden="true" />
            <span className="min-w-0 flex-1">
              <span className="block text-[13px] font-semibold leading-5 text-ink">Advanced settings</span>
              <span className="block text-xs text-ink-muted">Every XCreds preference: login window behaviour, claim mapping, admin rights, password sync and more.</span>
            </span>
            {errors.length > 0 && <span className="text-xs text-critical">{errors.length} {errors.length === 1 ? 'problem' : 'problems'}</span>}
          </button>
          {advanced && (
            <CardBody>
              {schema.isPending ? (
                <LoadingBlock />
              ) : schema.error || !schema.data ? (
                <Alert tone="critical">{schema.error ? errorMessage(schema.error) : 'The XCreds schema is unavailable.'}</Alert>
              ) : (
                <SchemaForm schema={schema.data.schema} value={f.values} onChange={(values) => set({ values })} errors={errors} disabled={ro} exclude={['LicenseFile']} />
              )}
            </CardBody>
          )}
        </Card>

        {advanced && schema.data && <ProfileXmlPanel preview={preview} />}
      </div>
      <ObjectBar ed={ed} isNew={false} listPath="/identity/xcreds" noun="XCreds settings" saving={saving} onSave={save} historyId="xcreds" noDone />
    </>
  )
}

function LicenseCard({ v, f, set, ro, error }: { v: XCredsView; f: Form; set: (p: Partial<Form>) => void; ro: boolean; error?: string }) {
  const ref = useRef<HTMLInputElement>(null)
  const [readErr, setReadErr] = useState('')
  const onFile = async (file: File | undefined) => {
    if (!file) return
    setReadErr('')
    try {
      set({ license: await fileToBase64(file), licenseName: file.name })
    } catch (e) {
      setReadErr(e instanceof Error ? e.message : String(e))
    } finally {
      if (ref.current) ref.current.value = ''
    }
  }
  let status: ReactNode
  if (f.license)
    status = (
      <span className="text-xs text-ink">
        New license selected: <span className="font-medium">{f.licenseName}</span> · saved with the settings
      </span>
    )
  else if (v.has_license)
    status = (
      <Badge tone="good" icon={<ShieldCheck />}>
        License stored
      </Badge>
    )
  else status = <span className="text-xs text-ink-muted">No license stored. XCreds runs as a 14-day trial without one.</span>
  return (
    <Card>
      <CardHeader title="License" description="The Twocanoes license file, delivered in the settings profile (LicenseFile). It is never shown again after upload." />
      <CardBody className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-3">
          <input ref={ref} type="file" className="hidden" onChange={(e) => onFile(e.target.files?.[0])} aria-label="License file" />
          {!ro && (
            <Button size="sm" icon={<FileKey2 />} onClick={() => ref.current?.click()}>
              {v.has_license || f.license ? 'Replace license…' : 'Upload license…'}
            </Button>
          )}
          {status}
          {!ro && f.license && (
            <Button size="xs" variant="ghost" icon={<X />} onClick={() => set({ license: '', licenseName: '' })}>
              {v.has_license ? 'Keep stored license' : 'Clear'}
            </Button>
          )}
        </div>
        {(error || readErr) && <p className="text-xs text-critical">{error || readErr}</p>}
      </CardBody>
    </Card>
  )
}

function AppStatus({ v }: { v: XCredsView }) {
  if (!v.app)
    return (
      <div className="rounded-lg border border-line px-3.5 py-3 text-xs text-ink-muted">
        <span className="font-medium text-ink">XCreds app:</span> not set up yet. It is added to Mac apps when you enable XCreds with Installomator.
      </div>
    )
  const entries = Object.entries(v.app.status ?? {}).filter(([, n]) => n > 0)
  const order = ['failed', 'running', 'pending', 'completed']
  entries.sort(([a], [b]) => (order.indexOf(a) === -1 ? 99 : order.indexOf(a)) - (order.indexOf(b) === -1 ? 99 : order.indexOf(b)))
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line px-3.5 py-3 text-xs text-ink-muted">
      <span className="font-medium text-ink">XCreds app</span>
      <Badge tone={v.app.enabled ? 'good' : 'neutral'}>{v.app.enabled ? 'Installing on scope' : 'Not installing'}</Badge>
      {entries.length === 0 ? (
        <span>No installs reported yet.</span>
      ) : (
        entries.map(([k, n]) => (
          <span key={k} className="inline-flex items-center gap-1 tabular-nums">
            <StatusBadge status={k} />
            <span>{n}</span>
          </span>
        ))
      )}
      <Link to={`/apps/${v.app.id}`} className="ml-auto text-accent-700 hover:underline dark:text-accent-300">
        Open in Mac apps
      </Link>
    </div>
  )
}
