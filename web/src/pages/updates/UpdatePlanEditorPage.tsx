import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { CalendarClock, ShieldAlert, SlidersHorizontal } from 'lucide-react'
import { qk, useInvalidate, useUpdatePlan, useUpdatesCatalog } from '../../api/queries'
import { api, ApiError, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { emptyScope, normalizeScope, type CatalogVersion, type Scope, type UpdateAction, type UpdatePlan, type UpdatePlanKind, type UpdateSettings } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Toggle } from '../../components/ui/Checkbox'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock, Spinner } from '../../components/ui/Spinner'
import { useToast } from '../../components/ui/Toast'
import { ScopeEditor, ScopeSummary } from '../../components/pickers/ScopeEditor'
import { formatLocalDeadline, SupervisionCallout } from '../../components/updates/updateState'
import { ErrorList, PlanKindBadge } from './SoftwareUpdatesPage'
import { relativeTime, shortDate } from '../../lib/format'
import { cn } from '../../lib/cn'
import { useCloneSource, useEditMode } from '../../components/editor/useEditMode'
import { ObjectBar } from '../../components/editor/ObjectBar'
import { DetailsLayout, SummaryCard } from '../../components/templates/DetailsLayout'
import { RelativeTime } from '../../components/ui/RelativeTime'

type Tri = '' | 'true' | 'false'
type Act = '' | UpdateAction

interface SettingsForm {
  notifications: Tri
  allow_standard_user_os_updates: Tri
  major_deferral_days: string
  minor_deferral_days: string
  system_deferral_days: string
  auto_download: Act
  auto_install_os_updates: Act
  auto_install_security_updates: Act
  rapid_security_response: Tri
  rapid_security_response_rollback: Tri
}

interface Form {
  name: string
  enabled: boolean
  kind: UpdatePlanKind
  target_os_version: string
  target_build_version: string
  date: string
  time: string
  details_url: string
  settings: SettingsForm
  scope: Scope
}

const tri = (v: boolean | undefined): Tri => (v === undefined ? '' : v ? 'true' : 'false')
const num = (v: number | undefined) => (v ? String(v) : '')

function settingsToForm(s: UpdateSettings | null | undefined): SettingsForm {
  const v = s ?? {}
  return {
    notifications: tri(v.notifications),
    allow_standard_user_os_updates: tri(v.allow_standard_user_os_updates),
    major_deferral_days: num(v.major_deferral_days),
    minor_deferral_days: num(v.minor_deferral_days),
    system_deferral_days: num(v.system_deferral_days),
    auto_download: v.auto_download ?? '',
    auto_install_os_updates: v.auto_install_os_updates ?? '',
    auto_install_security_updates: v.auto_install_security_updates ?? '',
    rapid_security_response: tri(v.rapid_security_response),
    rapid_security_response_rollback: tri(v.rapid_security_response_rollback),
  }
}

/** Only set fields are sent; blank means "leave the macOS default". */
function formToSettings(f: SettingsForm): UpdateSettings {
  const out: UpdateSettings = {}
  const b = (v: Tri) => (v === '' ? undefined : v === 'true')
  const n = (v: string) => (v.trim() === '' ? undefined : Number(v))
  const set = <K extends keyof UpdateSettings>(k: K, v: UpdateSettings[K] | undefined) => {
    if (v !== undefined && v !== ('' as unknown)) out[k] = v
  }
  set('notifications', b(f.notifications))
  set('allow_standard_user_os_updates', b(f.allow_standard_user_os_updates))
  set('major_deferral_days', n(f.major_deferral_days))
  set('minor_deferral_days', n(f.minor_deferral_days))
  set('system_deferral_days', n(f.system_deferral_days))
  set('auto_download', f.auto_download || undefined)
  set('auto_install_os_updates', f.auto_install_os_updates || undefined)
  set('auto_install_security_updates', f.auto_install_security_updates || undefined)
  set('rapid_security_response', b(f.rapid_security_response))
  set('rapid_security_response_rollback', b(f.rapid_security_response_rollback))
  return out
}

const pad = (n: number) => String(n).padStart(2, '0')
const isoDate = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`

function emptyForm(): Form {
  const d = new Date()
  d.setDate(d.getDate() + 14)
  return {
    name: '',
    enabled: true,
    kind: 'enforcement',
    target_os_version: '',
    target_build_version: '',
    date: isoDate(d),
    time: '17:00',
    details_url: '',
    settings: settingsToForm(null),
    scope: emptyScope(),
  }
}

function planToForm(p: UpdatePlan): Form {
  const [date = '', t = ''] = (p.target_local_datetime || '').split('T')
  const base = emptyForm()
  return {
    name: p.name,
    enabled: p.enabled,
    kind: p.kind,
    target_os_version: p.target_os_version,
    target_build_version: p.target_build_version,
    date: date || base.date,
    time: t ? t.slice(0, 5) : base.time,
    details_url: p.details_url,
    settings: settingsToForm(p.settings),
    scope: normalizeScope(p.scope),
  }
}

/** Client-side checks mirroring ddm.Enforcement.Validate / ddm.Settings.Validate, so most mistakes never round-trip. */
function validate(f: Form): string[] {
  const e: string[] = []
  if (!f.name.trim()) e.push('Name is required')
  if (f.kind === 'enforcement') {
    if (!/^\d+(\.\d+){0,2}$/.test(f.target_os_version.trim())) e.push('Target OS version must look like 15.6 or 15.6.1')
    if (f.target_build_version.trim() && !/^[0-9]{2}[A-Z][0-9]{1,5}[a-z]?$/.test(f.target_build_version.trim())) e.push('Target build version must look like 24G84')
    if (!/^\d{4}-\d{2}-\d{2}$/.test(f.date) || !/^\d{2}:\d{2}$/.test(f.time)) e.push('Deadline needs a date and a time')
    if (f.details_url.trim() && !f.details_url.trim().startsWith('https://')) e.push('Details URL must start with https://')
  } else {
    for (const [k, label] of [
      ['major_deferral_days', 'Major deferral'],
      ['minor_deferral_days', 'Minor deferral'],
      ['system_deferral_days', 'System deferral'],
    ] as const) {
      const v = f.settings[k].trim()
      if (v && !(Number.isInteger(Number(v)) && Number(v) >= 1 && Number(v) <= 90)) e.push(`${label} must be a whole number of days between 1 and 90`)
    }
  }
  return e
}

export function UpdatePlanEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const plan = useUpdatePlan(isNew ? undefined : id)
  const clone = useCloneSource<Form>()
  const [f, setF] = useState<Form>(() => (isNew && clone ? clone : emptyForm()))
  const [errors, setErrors] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const load = useCallback(() => {
    if (plan.data) setF(planToForm(plan.data))
  }, [plan.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: f, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])
  const readOnly = ed.readOnly

  const set = (patch: Partial<Form>) => setF((cur) => ({ ...cur, ...patch }))
  const setS = (patch: Partial<SettingsForm>) => setF((cur) => ({ ...cur, settings: { ...cur.settings, ...patch } }))

  const save = async () => {
    const local = validate(f)
    if (local.length) {
      setErrors(local.join('\n'))
      return false
    }
    setErrors(null)
    setSaving(true)
    const body = {
      ...(plan.data ?? {}),
      name: f.name.trim(),
      enabled: f.enabled,
      kind: f.kind,
      target_os_version: f.kind === 'enforcement' ? f.target_os_version.trim() : '',
      target_build_version: f.kind === 'enforcement' ? f.target_build_version.trim() : '',
      target_local_datetime: f.kind === 'enforcement' ? `${f.date}T${f.time}:00` : '',
      details_url: f.kind === 'enforcement' ? f.details_url.trim() : '',
      settings: f.kind === 'settings' ? formToSettings(f.settings) : {},
      scope: f.scope,
    }
    try {
      const out = isNew ? await api.post<UpdatePlan>('/api/v1/updates/plans', body) : await api.put<UpdatePlan>(`/api/v1/updates/plans/${id}`, body)
      await invalidate(qk.updatePlans, qk.updatesOverview, qk.updatePlan(out.id))
      toast.success(isNew ? 'Plan created' : 'Plan saved', 'Affected Macs will be asked to sync within a minute.')
      if (isNew) navigate(`/updates/plans/${out.id}`, { replace: true })
      return true
    } catch (e) {
      const msg = errorMessage(e)
      setErrors(msg)
      toast.error('Save failed', e instanceof ApiError && e.status === 400 ? 'See the problems listed above the form.' : msg)
      return false
    } finally {
      setSaving(false)
    }
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/updates/plans/${id}`)
      await invalidate(qk.updatePlans, qk.updatesOverview)
      toast.success('Plan deleted', 'Affected Macs will be asked to sync within a minute.')
      navigate('/updates/plans')
      return true
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
      return false
    }
  }

  if (!isNew && plan.isPending) return <LoadingBlock />
  if (!isNew && (plan.error || !plan.data)) return <Alert tone="critical">{plan.error ? errorMessage(plan.error) : 'Plan not found'}</Alert>

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Software updates', to: '/updates/plans' }, { label: isNew ? 'New plan' : plan.data?.name ?? '' }]}
        title={isNew ? 'New update plan' : plan.data?.name ?? 'Update plan'}
        description="Delivered to Macs as Declarative Device Management declarations."
      />
      <DetailsLayout
        aside={
          !isNew && (
            <SummaryCard
              rows={[
                { label: 'Status', value: <Badge tone={f.enabled ? 'good' : 'neutral'} dot>{f.enabled ? 'Enabled' : 'Disabled'}</Badge> },
                { label: 'Kind', value: <PlanKindBadge kind={f.kind} /> },
                { label: 'Target', value: f.kind === 'enforcement' && f.target_os_version && <span className="tabular-nums">macOS {f.target_os_version}{f.target_build_version && ` (${f.target_build_version})`}</span> },
                { label: 'Deadline', value: f.kind === 'enforcement' && f.date && f.time && formatLocalDeadline(`${f.date}T${f.time}:00`) },
                { label: 'Scope', value: <ScopeSummary scope={f.scope} /> },
                { label: 'Last changed', value: plan.data?.updated_at ? <RelativeTime value={plan.data.updated_at} /> : '' },
              ]}
            />
          )
        }
      >
      <div className="flex flex-col gap-6">
        {errors && (
          <Alert tone="critical" title="The plan couldn't be saved">
            <ErrorList message={errors} />
          </Alert>
        )}

        <Card>
          <CardHeader title="General" />
          <CardBody className="flex flex-col gap-5">
            <FormGrid cols={2}>
              <Field label="Name" required>
                <Input value={f.name} onChange={(e) => set({ name: e.target.value })} placeholder="e.g. macOS 15.6 by November" disabled={readOnly} />
              </Field>
              <div className="flex items-end pb-1.5">
                <Toggle checked={f.enabled} onChange={(v) => set({ enabled: v })} disabled={readOnly} label="Enabled" description="Disabled plans are removed from every Mac's declaration set." />
              </div>
            </FormGrid>
            <div>
              <p className="mb-1.5 text-sm font-medium text-ink">Kind</p>
              <div className="grid gap-3 md:grid-cols-2" role="radiogroup">
                <KindOption
                  active={f.kind === 'enforcement'}
                  disabled={readOnly}
                  onClick={() => set({ kind: 'enforcement' })}
                  icon={<CalendarClock />}
                  title="Enforcement"
                  minOS="macOS 14+"
                  text="Require a target version (and optionally build) by a local date and time. macOS shows escalating reminders, then installs and restarts at the deadline."
                />
                <KindOption
                  active={f.kind === 'settings'}
                  disabled={readOnly}
                  onClick={() => set({ kind: 'settings' })}
                  icon={<SlidersHorizontal />}
                  title="Settings"
                  minOS="macOS 15+"
                  text="Notifications, deferrals, automatic download and install behavior, Rapid Security Response, and whether standard users can update."
                />
              </div>
            </div>
          </CardBody>
        </Card>

        {f.kind === 'enforcement' ? <EnforcementCard f={f} set={set} readOnly={readOnly} /> : <SettingsCard s={f.settings} setS={setS} readOnly={readOnly} />}

        <Card>
          <CardHeader
            title="Scope"
            description="When several plans reach the same Mac, the enforcement plan with the highest target version wins (ties: earliest deadline), and the most recently updated settings plan wins."
          />
          <CardBody>{readOnly ? <ScopeSummary scope={f.scope} /> : <ScopeEditor value={f.scope} onChange={(scope) => set({ scope })} />}</CardBody>
        </Card>

        <SupervisionCallout />
      </div>
      </DetailsLayout>
      <ObjectBar
        ed={ed}
        isNew={isNew}
        listPath="/updates/plans"
        noun="plan"
        name={plan.data?.name}
        saving={saving}
        onSave={save}
        onDelete={doDelete}
        deleteDescription="The plan's declaration is removed from every Mac in scope at their next sync. Updates already installed are not affected."
        onClone={() => navigate('/updates/plans/new', { state: { clone: { ...f, name: `Copy of ${f.name}`, enabled: false } } })}
        historyId={id}
      />
    </>
  )
}

function KindOption({ active, disabled, onClick, icon, title, minOS, text }: { active: boolean; disabled?: boolean; onClick: () => void; icon: React.ReactNode; title: string; minOS: string; text: string }) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={active}
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'flex items-start gap-3 rounded-lg border p-3.5 text-left transition-colors focus-ring disabled:cursor-not-allowed',
        active ? 'border-accent-500/60 bg-accent-500/8' : 'border-line hover:border-line-strong',
      )}
    >
      <span className={cn('mt-0.5 [&>svg]:size-4', active ? 'text-accent-400' : 'text-ink-faint')}>{icon}</span>
      <span className="min-w-0">
        <span className="flex items-center gap-2">
          <span className="text-sm font-medium text-ink">{title}</span>
          <Badge>{minOS}</Badge>
        </span>
        <span className="mt-1 block text-xs text-ink-muted">{text}</span>
      </span>
    </button>
  )
}

function EnforcementCard({ f, set, readOnly }: { f: Form; set: (p: Partial<Form>) => void; readOnly: boolean }) {
  const preset = (days: number) => {
    const d = new Date()
    d.setDate(d.getDate() + days)
    set({ date: isoDate(d), time: '17:00' })
  }
  const preview = /^\d{4}-\d{2}-\d{2}$/.test(f.date) && /^\d{2}:\d{2}$/.test(f.time) ? `${f.date}T${f.time}:00` : ''
  return (
    <Card>
      <CardHeader title="Enforcement" description="com.apple.configuration.softwareupdate.enforcement.specific · requires macOS 14 or later" />
      <CardBody className="flex flex-col gap-5">
        {!readOnly && <CatalogPicker onPick={(version, build) => set({ target_os_version: version, target_build_version: build })} selected={f.target_os_version} />}
        <FormGrid cols={2}>
          <Field label="Target OS version" required hint="Macs below this version must update. Example: 15.6 or 15.6.1">
            <Input mono value={f.target_os_version} onChange={(e) => set({ target_os_version: e.target.value })} placeholder="15.6" disabled={readOnly} />
          </Field>
          <Field label="Target build" hint="Optional. Pins an exact build, e.g. 24G84 (useful for betas or re-released builds).">
            <Input mono value={f.target_build_version} onChange={(e) => set({ target_build_version: e.target.value })} placeholder="24G84" disabled={readOnly} />
          </Field>
        </FormGrid>
        <Field
          label="Deadline"
          required
          hint={
            <>
              This is the <strong className="text-ink">Mac&apos;s own local time</strong>, with no time zone: a 17:00 deadline is 17:00 wherever each Mac is. Sent as{' '}
              <span className="font-mono">{preview || 'YYYY-MM-DDTHH:MM:SS'}</span>.
            </>
          }
        >
          <div className="flex flex-wrap items-center gap-2">
            <Input type="date" value={f.date} onChange={(e) => set({ date: e.target.value })} className="w-44" disabled={readOnly} aria-label="Deadline date" />
            <Input type="time" step={60} value={f.time} onChange={(e) => set({ time: e.target.value.slice(0, 5) })} className="w-32" disabled={readOnly} aria-label="Deadline time" />
            {!readOnly && (
              <span className="flex gap-1">
                {[7, 14, 30].map((d) => (
                  <Button key={d} size="xs" variant="ghost" onClick={() => preset(d)}>
                    +{d} days
                  </Button>
                ))}
              </span>
            )}
            {preview && <span className="text-xs text-ink-muted">{formatLocalDeadline(preview)}</span>}
          </div>
        </Field>
        <Field label="Details URL" hint="Optional https:// link shown in macOS's update notification, e.g. your internal update policy page.">
          <Input value={f.details_url} onChange={(e) => set({ details_url: e.target.value })} placeholder="https://intranet.example.com/macos-updates" disabled={readOnly} />
        </Field>
      </CardBody>
    </Card>
  )
}

function CatalogPicker({ onPick, selected }: { onPick: (version: string, build: string) => void; selected: string }) {
  const q = useUpdatesCatalog()
  if (q.isPending)
    return (
      <div className="flex items-center gap-2 text-xs text-ink-muted">
        <Spinner className="size-3.5" /> Loading the macOS version catalog…
      </div>
    )
  if (q.error || !q.data)
    return (
      <Alert tone="warning" title="Version catalog unavailable">
        {q.error instanceof ApiError && q.error.status === 502 ? 'The SOFA feed could not be reached from the server.' : errorMessage(q.error)} Enter the target version manually below.
      </Alert>
    )
  const versions = q.data.versions ?? []
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p className="micro-label">Latest macOS releases</p>
        <p className="text-[11px] text-ink-faint">
          <a href="https://sofa.macadmins.io" target="_blank" rel="noreferrer" className="hover:text-ink hover:underline">
            SOFA feed by Mac Admins Open Source
          </a>{' '}
          · fetched {relativeTime(q.data.fetched_at)}
        </p>
      </div>
      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
        {versions.map((v) => (
          <CatalogCard key={v.name} v={v} selected={selected} onPick={onPick} />
        ))}
      </div>
    </div>
  )
}

function CatalogCard({ v, selected, onPick }: { v: CatalogVersion; selected: string; onPick: (version: string, build: string) => void }) {
  const cves = v.actively_exploited_cves?.length ?? 0
  const older = (v.recent_releases ?? []).filter((r) => r.version !== v.latest_version)
  const isSel = selected === v.latest_version
  return (
    <div className={cn('flex flex-col gap-2 rounded-lg border p-3', isSel ? 'border-accent-500/60 bg-accent-500/6' : 'border-line')}>
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-ink">{v.name}</p>
          <p className="text-xs text-ink-muted">
            <span className="tabular-nums text-ink">{v.latest_version}</span> <span className="font-mono">({v.latest_build})</span> · {shortDate(v.release_date)}
          </p>
        </div>
        <Button size="xs" variant={isSel ? 'primary' : 'secondary'} onClick={() => onPick(v.latest_version, v.latest_build)}>
          {isSel ? 'Selected' : 'Use'}
        </Button>
      </div>
      {cves > 0 && (
        <Badge tone="critical" icon={<ShieldAlert />} className="w-fit" title={v.actively_exploited_cves?.join(', ')}>
          Fixes {cves} actively exploited CVE{cves === 1 ? '' : 's'}
        </Badge>
      )}
      {older.length > 0 && (
        <Select
          value=""
          onChange={(e) => e.target.value && onPick(e.target.value, '')}
          className="h-7 text-xs"
          aria-label={`Earlier ${v.name} releases`}
          options={[{ value: '', label: 'Earlier releases…' }, ...older.map((r) => ({ value: r.version, label: `${r.version} · ${shortDate(r.release_date)}` }))]}
        />
      )}
    </div>
  )
}

const actionOptions = [
  { value: '', label: 'Not set (macOS default)' },
  { value: 'Allowed', label: 'Allowed (user chooses)' },
  { value: 'AlwaysOn', label: 'Always on' },
  { value: 'AlwaysOff', label: 'Always off' },
]

function TriSelect({ value, onChange, on = 'On', off = 'Off', disabled }: { value: Tri; onChange: (v: Tri) => void; on?: string; off?: string; disabled?: boolean }) {
  return (
    <Select
      value={value}
      onChange={(e) => onChange(e.target.value as Tri)}
      disabled={disabled}
      options={[
        { value: '', label: 'Not set (macOS default)' },
        { value: 'true', label: on },
        { value: 'false', label: off },
      ]}
    />
  )
}

function SettingsCard({ s, setS, readOnly }: { s: SettingsForm; setS: (p: Partial<SettingsForm>) => void; readOnly: boolean }) {
  const days = (k: 'major_deferral_days' | 'minor_deferral_days' | 'system_deferral_days', label: string, hint: string) => (
    <Field label={label} hint={hint}>
      <Input type="number" min={1} max={90} step={1} value={s[k]} onChange={(e) => setS({ [k]: e.target.value } as Partial<SettingsForm>)} placeholder="Not set" disabled={readOnly} className="max-w-32" />
    </Field>
  )
  return (
    <Card>
      <CardHeader title="Settings" description="com.apple.configuration.softwareupdate.settings · requires macOS 15 or later. Leave a field unset to keep the macOS default; only set fields are sent." />
      <CardBody className="flex flex-col gap-6">
        <FormGrid cols={2}>
          <Field label="Notifications" hint="Show software update notifications to the user.">
            <TriSelect value={s.notifications} onChange={(v) => setS({ notifications: v })} disabled={readOnly} />
          </Field>
          <Field label="Standard users can update" hint="Allow non-admin users to install OS updates.">
            <TriSelect value={s.allow_standard_user_os_updates} onChange={(v) => setS({ allow_standard_user_os_updates: v })} on="Allowed" off="Not allowed" disabled={readOnly} />
          </Field>
        </FormGrid>
        <div>
          <p className="micro-label mb-3">Deferrals (1–90 days, blank = unset)</p>
          <FormGrid cols={3}>
            {days('major_deferral_days', 'Major upgrades', 'Hide new major macOS versions this many days after release.')}
            {days('minor_deferral_days', 'Minor updates', 'Hide minor macOS updates this many days after release.')}
            {days('system_deferral_days', 'System files & security', 'Defer non-OS system and security data updates.')}
          </FormGrid>
        </div>
        <div>
          <p className="micro-label mb-3">Automatic actions</p>
          <FormGrid cols={3}>
            <Field label="Download updates">
              <Select value={s.auto_download} onChange={(e) => setS({ auto_download: e.target.value as Act })} options={actionOptions} disabled={readOnly} />
            </Field>
            <Field label="Install macOS updates">
              <Select value={s.auto_install_os_updates} onChange={(e) => setS({ auto_install_os_updates: e.target.value as Act })} options={actionOptions} disabled={readOnly} />
            </Field>
            <Field label="Install security updates">
              <Select value={s.auto_install_security_updates} onChange={(e) => setS({ auto_install_security_updates: e.target.value as Act })} options={actionOptions} disabled={readOnly} />
            </Field>
          </FormGrid>
        </div>
        <div>
          <p className="micro-label mb-3">Rapid Security Response</p>
          <FormGrid cols={2}>
            <Field label="Rapid Security Responses" hint="Allow background security responses between full updates.">
              <TriSelect value={s.rapid_security_response} onChange={(v) => setS({ rapid_security_response: v })} on="Enabled" off="Disabled" disabled={readOnly} />
            </Field>
            <Field label="Allow rollback" hint="Let users remove an installed Rapid Security Response.">
              <TriSelect value={s.rapid_security_response_rollback} onChange={(v) => setS({ rapid_security_response_rollback: v })} on="Allowed" off="Not allowed" disabled={readOnly} />
            </Field>
          </FormGrid>
        </div>
      </CardBody>
    </Card>
  )
}
