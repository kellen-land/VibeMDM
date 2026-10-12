import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ArrowUpCircle, CheckCircle2, CircleDashed, Download, ExternalLink, ShieldCheck, Target, UserCog, UserRound, Users, type LucideIcon } from 'lucide-react'
import { qk, useCustomSchema, useInvalidate, useSuper, useSuperStatus } from '../../api/queries'
import { api, errorMessage, fieldErrors } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { emptyScope, normalizeScope, type SchemaFieldError, type SchemaValues, type Scope, type SuperAuthMode, type SuperInstallResult, type SuperSaveRequest, type SuperState, type SuperStatusRow, type SuperView } from '../../api/types'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table } from '../../components/ui/Table'
import { Button } from '../../components/ui/Button'
import { Badge, type BadgeTone } from '../../components/ui/Badge'
import { Tabs } from '../../components/ui/Tabs'
import { Toggle } from '../../components/ui/Checkbox'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select } from '../../components/ui/Input'
import { SearchInput } from '../../components/ui/SearchInput'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { useToast } from '../../components/ui/Toast'
import { ScopeEditor } from '../../components/pickers/ScopeEditor'
import { SchemaForm } from '../../components/schema/SchemaForm'
import { ProfileXmlPanel, useLivePreview } from '../../components/schema/ProfileXmlPanel'
import { pruneValues } from '../../components/schema/schema'
import { compactNumber, shortHash } from '../../lib/format'
import { cn } from '../../lib/cn'
import { useEditMode, type EditMode } from '../../components/editor/useEditMode'
import { ObjectBar } from '../../components/editor/ObjectBar'

const SUPER_SCHEMA = 'builtin:com.macjutsu.super'
const SUPER_PROFILE_NAME = 'super (S.U.P.E.R.M.A.N.) settings'

type View = 'status' | 'settings'

export function SuperTab() {
  const [sp, setSp] = useSearchParams()
  const view: View = sp.get('view') === 'settings' ? 'settings' : 'status'
  const setView = (v: View) => setSp(v === 'status' ? {} : { view: v }, { replace: true })
  const { isAdmin } = useAuth()
  const cfg = useSuper()
  const toast = useToast()
  const invalidate = useInvalidate()

  // The settings form lives here so that one edit mode (and one Save) covers the whole tab,
  // whichever view is showing.
  const [f, setF] = useState<Form>({ enabled: false, scope: emptyScope(), auth_mode: 'none', auth_account: '', password: '', clearPassword: false, values: {} })
  const [saveErrors, setSaveErrors] = useState<SchemaFieldError[]>([])
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const load = useCallback(() => {
    if (cfg.data) setF(toForm(cfg.data))
    setSaveError(null)
    setSaveErrors([])
  }, [cfg.data])
  const ed = useEditMode({ isNew: false, canEdit: isAdmin, snapshot: f, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])
  // Edit opens the settings, which is where the form is.
  useEffect(() => {
    if (ed.editing && view !== 'settings') setView('settings')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ed.editing])

  const save = async () => {
    if (f.auth_mode !== 'none' && !f.auth_account.trim()) {
      setView('settings')
      setSaveError(`Enter the account name for ${f.auth_mode === 'local' ? 'the local account' : 'the service account'}.`)
      return false
    }
    setSaving(true)
    setSaveError(null)
    const body: SuperSaveRequest = { enabled: f.enabled, scope: f.scope, auth_mode: f.auth_mode, auth_account: f.auth_mode === 'none' ? '' : f.auth_account.trim(), values: pruneValues(f.values) }
    if (f.auth_mode !== 'none') {
      if (f.clearPassword) body.auth_password = ''
      else if (f.password) body.auth_password = f.password
    }
    try {
      const out = await api.put<SuperView>('/api/v1/updates/super', body)
      setF(toForm(out))
      setSaveErrors([])
      await invalidate(qk.superAll, qk.profiles)
      toast.success('super settings saved', out.config.enabled ? 'Macs in scope get super and its settings profile at their next check-in.' : 'super is off; its settings profile is removed from every Mac.')
      return true
    } catch (e) {
      const fe = fieldErrors(e)
      setSaveErrors(fe)
      setSaveError(fe.length ? 'Some preference values are invalid; they are marked in the form below.' : errorMessage(e))
      setView('settings')
      toast.error('Save failed', fe.length ? 'Some values are invalid.' : errorMessage(e))
      return false
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <Alert tone="info" title="Use DDM enforcement or super on each Mac, not both">
        A DDM enforcement plan sets a hard deadline that macOS enforces itself. super adds a friendlier layer on top: deferrals, reminders and notifications before it installs. Running both on the
        same Mac gives users competing prompts, so scope each Mac to one or the other.
      </Alert>
      <Tabs<View>
        variant="pills"
        items={[
          { key: 'status', label: 'Status' },
          { key: 'settings', label: 'Settings' },
        ]}
        value={view}
        onChange={setView}
      />
      {view === 'status' ? <StatusSection /> : <SettingsSection f={f} setF={setF} ed={ed} saveErrors={saveErrors} saveError={saveError} />}
      {cfg.data && <AboutCard v={cfg.data} />}
      <ObjectBar ed={ed} isNew={false} listPath="/updates/super" noun="super settings" saving={saving} onSave={save} noDone />
    </div>
  )
}

// ---- Status ----

interface StateMeta {
  label: string
  tone: BadgeTone
  icon: LucideIcon
  ink: string
  fill: string
}

const superStateMeta: Record<SuperState, StateMeta> = {
  current: { label: 'Current', tone: 'good', icon: CheckCircle2, ink: 'text-good', fill: 'bg-good' },
  outdated: { label: 'Outdated', tone: 'warning', icon: ArrowUpCircle, ink: 'text-warning', fill: 'bg-warning' },
  missing: { label: 'Not installed', tone: 'neutral', icon: CircleDashed, ink: 'text-ink-faint', fill: 'bg-line-strong' },
}

function SuperStateBadge({ state }: { state: SuperState }) {
  const m = superStateMeta[state] ?? superStateMeta.missing
  return (
    <Badge tone={m.tone} icon={<m.icon />}>
      {m.label}
    </Badge>
  )
}

function Tile({ label, value, icon: Icon, ink, fill, active, onClick, hint }: { label: string; value: number; icon: LucideIcon; ink: string; fill: string; active: boolean; onClick: () => void; hint?: string }) {
  const accent = value > 0
  return (
    <button type="button" onClick={onClick} aria-pressed={active} className="block h-full rounded-xl text-left focus-ring" title={hint}>
      <Card className={cn('group relative h-full overflow-hidden px-4 pt-3.5 pb-4 transition-colors hover:border-line-strong', active && 'border-accent-500/60')}>
        <div className="flex items-start justify-between gap-2">
          <p className="micro-label">{label}</p>
          <Icon className={cn('size-4 shrink-0', accent ? ink : 'text-ink-faint')} aria-hidden="true" />
        </div>
        <p className="mt-2 text-[30px] font-semibold leading-9 tracking-tight text-ink tabular-nums" title={value.toLocaleString()}>
          {compactNumber(value)}
        </p>
        <span className={cn('absolute inset-x-0 bottom-0 h-[2px]', accent ? fill : 'bg-transparent group-hover:bg-accent-500/70')} aria-hidden="true" />
      </Card>
    </button>
  )
}

function str(v: unknown): string {
  if (v === undefined || v === null) return ''
  return typeof v === 'string' ? v : String(v)
}

function StatusSection() {
  const { isAdmin } = useAuth()
  const q = useSuperStatus()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [filter, setFilter] = useState<SuperState | ''>('')
  const [search, setSearch] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const [confirmAll, setConfirmAll] = useState(false)

  const install = async (ids?: string[]) => {
    setBusy(ids ? ids[0] : 'all')
    try {
      const r = await api.post<SuperInstallResult>('/api/v1/updates/super/install', ids ? { computer_ids: ids } : {})
      const skipped = Object.entries(r.skipped ?? {})
      if (r.queued === 0 && skipped.length) toast.warning('Nothing queued', skipped.map(([k, v]) => `${k}: ${v}`).join(' · '))
      else if (skipped.length) toast.warning(`super install queued on ${r.queued} ${r.queued === 1 ? 'Mac' : 'Macs'}`, `Skipped ${skipped.map(([k, v]) => `${k}: ${v}`).join(' · ')}`)
      else toast.success(r.queued ? `super install queued on ${r.queued} ${r.queued === 1 ? 'Mac' : 'Macs'}` : 'Nothing to install', r.queued ? 'Each Mac installs it at its next agent check-in.' : 'Every Mac in scope already runs the pinned version.')
      await invalidate(qk.superStatus)
      setConfirmAll(false)
    } catch (e) {
      toast.error('Install failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  if (q.isPending) return <LoadingBlock />
  if (q.error || !q.data) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'super status unavailable'}</Alert>
  const d = q.data
  const s = search.trim().toLowerCase()
  const rows = (d.computers ?? []).filter((r) => (!filter || r.state === filter) && (!s || r.computer_name.toLowerCase().includes(s) || r.os_version.includes(s)))
  const toInstall = d.summary.missing + d.summary.outdated
  const toggle = (st: SuperState) => setFilter((cur) => (cur === st ? '' : st))

  return (
    <div className="flex flex-col gap-5">
      {!d.enabled && (
        <Alert tone="info" title="super is turned off">
          Turn it on under Settings to install super {d.version} on the Macs in scope and deliver its settings profile.
        </Alert>
      )}
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <Tile label="Targeted" value={d.summary.targeted} icon={Target} ink="text-accent-400" fill="bg-accent-500/70" active={filter === ''} onClick={() => setFilter('')} hint="Macs in super's scope" />
        {(['current', 'outdated', 'missing'] as SuperState[]).map((st) => {
          const m = superStateMeta[st]
          return <Tile key={st} label={st === 'missing' ? 'Missing' : m.label} value={d.summary[st]} icon={m.icon} ink={m.ink} fill={m.fill} active={filter === st} onClick={() => toggle(st)} />
        })}
      </div>
      <Card>
        <CardHeader
          title="Computers"
          description={`${rows.length.toLocaleString()} of ${(d.computers ?? []).length.toLocaleString()} targeted Macs · pinned version ${d.version}`}
          actions={
            <div className="flex flex-wrap items-center gap-2">
              <Select value={filter} onChange={(e) => setFilter(e.target.value as SuperState | '')} className="w-36" aria-label="Filter by state">
                <option value="">All states</option>
                <option value="current">Current</option>
                <option value="outdated">Outdated</option>
                <option value="missing">Not installed</option>
              </Select>
              <SearchInput value={search} onChange={setSearch} placeholder="Filter by name or version…" className="w-52" />
              {isAdmin && (
                <Button icon={<Download />} onClick={() => setConfirmAll(true)} disabled={!d.enabled || toInstall === 0} loading={busy === 'all'} title={!d.enabled ? 'Turn super on first' : undefined}>
                  Install on all missing
                </Button>
              )}
            </div>
          }
        />
        <Table<SuperStatusRow>
          rows={rows}
          rowKey={(r) => r.computer_id}
          emptyTitle={filter || s ? 'No computers match' : 'No Macs in scope'}
          emptyDescription={filter || s ? undefined : 'Add computers or groups to super’s scope under Settings.'}
          columns={[
            {
              key: 'name',
              header: 'Computer',
              render: (r) => (
                <span className="block">
                  <Link to={`/computers/${r.computer_id}`} className="font-medium text-ink hover:underline">
                    {r.computer_name || r.computer_id}
                  </Link>
                  <span className="block text-xs text-ink-muted">
                    macOS <span className="tabular-nums">{r.os_version || '?'}</span>
                    {!r.agent_recent && <span className="text-ink-faint"> · agent not seen in the last hour</span>}
                  </span>
                </span>
              ),
            },
            { key: 'state', header: 'State', render: (r) => <SuperStateBadge state={r.state} /> },
            { key: 'version', header: 'super', render: (r) => <span className="font-mono text-xs text-ink-muted">{str(r.super?.SuperVersion) || '—'}</span> },
            {
              key: 'status',
              header: 'Status',
              render: (r) => {
                const t = str(r.super?.SuperStatus)
                return t ? (
                  <span className="block max-w-72 truncate text-xs text-ink-muted" title={t}>
                    {t}
                  </span>
                ) : (
                  <span className="text-ink-faint">—</span>
                )
              },
            },
            { key: 'next', header: 'Next launch', render: (r) => <span className="whitespace-nowrap text-xs text-ink-muted">{str(r.super?.NextAutoLaunch) || '—'}</span> },
            {
              key: 'deadlines',
              header: 'Deferrals used',
              render: (r) => {
                const soft = str(r.super?.DeadlineCounterSoft)
                const hard = str(r.super?.DeadlineCounterHard)
                if (!soft && !hard) return <span className="text-ink-faint">—</span>
                return (
                  <span className="whitespace-nowrap text-xs text-ink-muted tabular-nums" title="Deadline counters reported by super">
                    {soft && <>Soft {soft}</>}
                    {soft && hard && ' · '}
                    {hard && <>Hard {hard}</>}
                  </span>
                )
              },
            },
            { key: 'inv', header: 'Last inventory', render: (r) => <RelativeTime value={r.last_inventory_at} className="text-ink-muted" /> },
            ...(isAdmin
              ? [
                  {
                    key: 'actions',
                    header: '',
                    align: 'right' as const,
                    render: (r: SuperStatusRow) =>
                      r.state !== 'current' ? (
                        <Button size="xs" icon={<Download />} loading={busy === r.computer_id} disabled={!!busy || !d.enabled} onClick={() => install([r.computer_id])}>
                          Install now
                        </Button>
                      ) : null,
                  },
                ]
              : []),
          ]}
        />
      </Card>
      <ConfirmDialog
        open={confirmAll}
        onClose={() => setConfirmAll(false)}
        onConfirm={() => install()}
        title={`Install super ${d.version} on ${toInstall} ${toInstall === 1 ? 'Mac' : 'Macs'}?`}
        description="Every Mac in scope that is missing super or runs another version gets an install task at its next agent check-in."
        confirmLabel="Install"
        loading={busy === 'all'}
      />
    </div>
  )
}

// ---- Settings ----

interface Form {
  enabled: boolean
  scope: Scope
  auth_mode: SuperAuthMode
  auth_account: string
  password: string
  clearPassword: boolean
  values: SchemaValues
}

function toForm(v: SuperView): Form {
  return {
    enabled: v.config.enabled,
    scope: normalizeScope(v.config.scope),
    auth_mode: v.config.auth_mode || 'none',
    auth_account: v.config.auth_account ?? '',
    password: '',
    clearPassword: false,
    values: v.values ?? {},
  }
}

const authOptions: { key: SuperAuthMode; title: string; text: string; icon: ReactNode }[] = [
  { key: 'none', title: 'None', text: 'super asks the user to authenticate when an update needs it.', icon: <Users /> },
  { key: 'local', title: 'Local account', text: 'An existing local administrator account on each Mac.', icon: <UserRound /> },
  { key: 'service', title: 'Service account', text: 'A dedicated account that super uses only to install updates.', icon: <UserCog /> },
]

interface SettingsProps {
  f: Form
  setF: React.Dispatch<React.SetStateAction<Form>>
  ed: EditMode
  saveErrors: SchemaFieldError[]
  saveError: string | null
}

function SettingsSection({ f, setF, ed, saveErrors, saveError }: SettingsProps) {
  const q = useSuper()
  const schema = useCustomSchema(SUPER_SCHEMA)
  const ro = ed.readOnly

  const preview = useLivePreview(SUPER_SCHEMA, SUPER_PROFILE_NAME, f.values, !!schema.data)
  const errors = preview.data?.errors?.length ? preview.data.errors : saveErrors
  const set = (p: Partial<Form>) => setF((cur) => ({ ...cur, ...p }))

  if (q.isPending) return <LoadingBlock />
  if (q.error || !q.data) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'super settings unavailable'}</Alert>
  const v = q.data

  return (
    <div className="flex flex-col gap-6">
      {saveError && (
        <Alert tone="critical" title="The settings couldn’t be saved">
          {saveError}
        </Alert>
      )}
      <Card>
        <CardHeader title="Deployment" description={`Install super ${v.version} with the VibeMDM agent on the Macs in scope.`} />
        <CardBody className="flex flex-col gap-5">
          <Toggle checked={f.enabled} onChange={(enabled) => set({ enabled })} disabled={ro} label="Enable super" description="When off, VibeMDM stops installing super and removes its settings profile from every Mac." />
          <div>
            <p className="mb-2 text-sm font-medium text-ink">Scope</p>
            <ScopeEditor value={f.scope} onChange={(scope) => set({ scope })} disabled={ro} />
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardHeader title="Authentication" description="How super gets permission to install updates." />
        <CardBody className="flex flex-col gap-5">
          <p className="text-xs leading-relaxed text-ink-muted">
            Apple silicon needs a local admin or service account to install updates without asking the user. The password is sealed at rest and only sent to Macs when super is installed.
          </p>
          <div className="grid gap-3 md:grid-cols-3" role="radiogroup" aria-label="Authentication">
            {authOptions.map((o) => {
              const active = f.auth_mode === o.key
              return (
                <button
                  key={o.key}
                  type="button"
                  role="radio"
                  aria-checked={active}
                  disabled={ro}
                  onClick={() => set({ auth_mode: o.key })}
                  className={cn('flex items-start gap-3 rounded-lg border p-3.5 text-left transition-colors focus-ring disabled:cursor-not-allowed', active ? 'border-accent-500/60 bg-accent-500/8' : 'border-line hover:border-line-strong')}
                >
                  <span className={cn('mt-0.5 [&>svg]:size-4', active ? 'text-accent-400' : 'text-ink-faint')}>{o.icon}</span>
                  <span className="min-w-0">
                    <span className="block text-sm font-medium text-ink">{o.title}</span>
                    <span className="mt-1 block text-xs text-ink-muted">{o.text}</span>
                  </span>
                </button>
              )
            })}
          </div>
          {f.auth_mode !== 'none' && (
            <FormGrid cols={2}>
              <Field label="Account name" required hint={f.auth_mode === 'local' ? 'The short name of a local administrator on each Mac.' : 'The short name of the service account.'}>
                <Input mono value={f.auth_account} onChange={(e) => set({ auth_account: e.target.value })} disabled={ro} autoComplete="off" spellCheck={false} placeholder={f.auth_mode === 'local' ? 'admin' : 'super-service'} />
              </Field>
              <Field
                label={
                  <span className="inline-flex items-center gap-2">
                    Password
                    {v.config.has_password && !f.clearPassword && !f.password && (
                      <Badge tone="good" icon={<ShieldCheck />}>
                        Stored
                      </Badge>
                    )}
                    {f.clearPassword && <Badge tone="warning">Will be cleared</Badge>}
                  </span>
                }
                hint={v.config.has_password ? 'Leave blank to keep the stored password.' : 'Required for super to install without asking the user.'}
              >
                <div className="flex items-center gap-2">
                  <Input
                    type="password"
                    value={f.password}
                    onChange={(e) => set({ password: e.target.value, clearPassword: false })}
                    disabled={ro || f.clearPassword}
                    autoComplete="new-password"
                    placeholder={v.config.has_password ? '••••••••' : ''}
                    className="flex-1"
                  />
                  {!ro && v.config.has_password && (
                    <Button size="sm" variant="ghost" onClick={() => set({ clearPassword: !f.clearPassword, password: '' })}>
                      {f.clearPassword ? 'Keep stored' : 'Clear stored'}
                    </Button>
                  )}
                </div>
              </Field>
            </FormGrid>
          )}
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title="super preferences"
          description={
            <>
              Delivered as the configuration profile &ldquo;{SUPER_PROFILE_NAME}&rdquo; (<code className="font-mono">com.macjutsu.super</code>). Leave a setting unset to use super&apos;s default.
            </>
          }
        />
        <CardBody>
          {schema.isPending ? (
            <LoadingBlock />
          ) : schema.error || !schema.data ? (
            <Alert tone="critical">{schema.error ? errorMessage(schema.error) : 'The super schema is unavailable.'}</Alert>
          ) : (
            <SchemaForm schema={schema.data.schema} value={f.values} onChange={(values) => set({ values })} errors={errors} disabled={ro} />
          )}
        </CardBody>
      </Card>

      {schema.data && <ProfileXmlPanel preview={preview} />}
    </div>
  )
}

function AboutCard({ v }: { v: SuperView }) {
  return (
    <Card>
      <CardHeader title="About super" />
      <CardBody>
        <ul className="flex flex-col gap-2 text-xs leading-relaxed text-ink-muted">
          <li>
            VibeMDM pins super <span className="font-medium text-ink tabular-nums">{v.version}</span> and verifies its SHA-256 before running it
            {v.sha256 && (
              <>
                {' '}
                (<code className="font-mono text-ink" title={v.sha256}>
                  {shortHash(v.sha256, 16)}
                </code>
                )
              </>
            )}
            .{' '}
            {v.script_url && (
              <a href={v.script_url} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-0.5 text-accent-700 hover:underline dark:text-accent-300">
                Pinned script
                <ExternalLink className="size-3" aria-hidden="true" />
              </a>
            )}
          </li>
          <li>super 6 (beta) is not used yet.</li>
          <li>
            Settings are delivered as a configuration profile, &ldquo;{SUPER_PROFILE_NAME}&rdquo;
            {v.config.profile_id && (
              <>
                {' '}
                (
                <Link to={`/profiles/${v.config.profile_id}`} className="text-accent-700 hover:underline dark:text-accent-300">
                  view profile
                </Link>
                )
              </>
            )}
            .
          </li>
          <li>
            <a href={v.project} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-0.5 text-accent-700 hover:underline dark:text-accent-300">
              super by Kevin M. White — {v.license || 'Apache-2.0'}
              <ExternalLink className="size-3" aria-hidden="true" />
            </a>
          </li>
        </ul>
      </CardBody>
    </Card>
  )
}
