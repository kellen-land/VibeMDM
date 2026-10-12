import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Check, ChevronDown, ChevronRight, FileCode2, ListPlus, MinusCircle, Plus, RotateCcw, ShieldCheck, Wrench, X } from 'lucide-react'
import { qk, useComplianceBaseline, useComplianceCatalog, useComplianceProfile, useComplianceRules, useInvalidate } from '../../api/queries'
import { api, ApiError, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import { emptyScope, normalizeScope, type ComplianceBaseline, type ComplianceCatalogBaseline, type ComplianceRule, type Scope } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { Button, IconButton } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Toggle } from '../../components/ui/Checkbox'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select, Textarea } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock, Spinner } from '../../components/ui/Spinner'
import { Modal } from '../../components/ui/Modal'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { SearchInput } from '../../components/ui/SearchInput'
import { Tabs } from '../../components/ui/Tabs'
import { CodeBlock } from '../../components/ui/CopyButton'
import { EmptyState } from '../../components/ui/EmptyState'
import { useToast } from '../../components/ui/Toast'
import { ScopeEditor, ScopeSummary } from '../../components/pickers/ScopeEditor'
import { ErrorList } from '../updates/SoftwareUpdatesPage'
import {
  ComplianceAttribution,
  Discussion,
  References,
  RuleCapabilityBadges,
  RuleScripts,
  RuleSeverityBadge,
  benchmarkBlurb,
  benchmarkName,
  benchmarkOrder,
  bySection,
  sectionLabel,
} from '../../components/compliance/compliance'
import { cn } from '../../lib/cn'
import { useCloneSource, useEditMode } from '../../components/editor/useEditMode'
import { ObjectBar } from '../../components/editor/ObjectBar'

const intervalPresets = [
  { hours: 6, label: 'Every 6 hours' },
  { hours: 12, label: 'Every 12 hours' },
  { hours: 24, label: 'Daily' },
  { hours: 48, label: 'Every 2 days' },
  { hours: 168, label: 'Weekly' },
]

interface Form {
  benchmark: string
  name: string
  description: string
  enabled: boolean
  interval: string // preset hours, or 'custom'
  customHours: string
  auto_remediate: boolean
  deploy_profile: boolean
  scope: Scope
  extra_rules: string[]
  exclusions: Record<string, string>
  /** Raw text from the inputs; converted per datatype on save. */
  odv: Record<string, string>
}

function emptyForm(): Form {
  return {
    benchmark: '',
    name: '',
    description: '',
    enabled: true,
    interval: '24',
    customHours: '',
    auto_remediate: false,
    deploy_profile: false,
    scope: emptyScope(),
    extra_rules: [],
    exclusions: {},
    odv: {},
  }
}

function toForm(b: ComplianceBaseline): Form {
  const preset = intervalPresets.some((p) => p.hours === b.scan_interval_hours)
  const odv: Record<string, string> = {}
  for (const [k, v] of Object.entries(b.odv_overrides ?? {})) if (v !== null && v !== undefined && v !== '') odv[k] = String(v)
  return {
    benchmark: b.benchmark,
    name: b.name,
    description: b.description ?? '',
    enabled: b.enabled,
    interval: preset ? String(b.scan_interval_hours) : 'custom',
    customHours: preset ? '' : String(b.scan_interval_hours || ''),
    auto_remediate: b.auto_remediate,
    deploy_profile: b.deploy_profile,
    scope: normalizeScope(b.scope),
    extra_rules: b.extra_rules ?? [],
    exclusions: { ...(b.exclusions ?? {}) },
    odv,
  }
}

const intervalHours = (f: Form) => (f.interval === 'custom' ? Number(f.customHours) : Number(f.interval))

const isNumeric = (r: ComplianceRule) => r.odv?.hint.datatype === 'number' || r.odv?.hint.datatype === 'integer'

/** "Allowed|ReadOnly|Disallowed" style enum hints become a select. */
function enumChoices(r: ComplianceRule): string[] | null {
  const h = r.odv?.hint
  if (!h || h.datatype !== 'enum') return null
  return /^[\w.-]+(\|[\w.-]+)+$/.test(h.description.trim()) ? h.description.trim().split('|') : null
}

function odvProblem(r: ComplianceRule, raw: string): string | null {
  if (!r.odv || raw.trim() === '') return null
  if (isNumeric(r)) {
    const n = Number(raw)
    if (raw.trim() === '' || Number.isNaN(n)) return 'Must be a number'
    if (r.odv.hint.min !== undefined && n < r.odv.hint.min) return `At least ${r.odv.hint.min}`
    if (r.odv.hint.max !== undefined && n > r.odv.hint.max) return `At most ${r.odv.hint.max}`
  } else if (/[\n\r]/.test(raw)) return 'Single line only'
  return null
}

function validate(f: Form, rules: Map<string, ComplianceRule>): string[] {
  const e: string[] = []
  if (!f.benchmark) e.push('Choose a benchmark')
  if (!f.name.trim()) e.push('Name is required')
  const h = intervalHours(f)
  if (!Number.isInteger(h) || h < 1 || h > 720) e.push('Scan interval must be a whole number of hours between 1 and 720')
  for (const [id, reason] of Object.entries(f.exclusions)) if (!reason.trim()) e.push(`Exemption for ${id} needs a reason`)
  for (const [id, raw] of Object.entries(f.odv)) {
    const r = rules.get(id)
    const p = r && odvProblem(r, raw)
    if (p) e.push(`${r?.title ?? id}: ${p.toLowerCase()}`)
  }
  return e
}

export function BaselineEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const baseline = useComplianceBaseline(isNew ? undefined : id)
  const catalog = useComplianceCatalog()
  const clone = useCloneSource<ComplianceBaseline>()
  const [f, setF] = useState<Form>(() => (isNew && clone ? toForm(clone) : emptyForm()))
  const [errors, setErrors] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [changingBenchmark, setChangingBenchmark] = useState(false)
  const [pendingBenchmark, setPendingBenchmark] = useState<string | null>(null)

  const seed = baseline.data ? `${baseline.data.id}|${baseline.data.updated_at}` : ''
  const [loaded, setLoaded] = useState('')
  const load = useCallback(() => {
    if (baseline.data) {
      setF(toForm(baseline.data))
      setLoaded(`${baseline.data.id}|${baseline.data.updated_at}`)
    }
  }, [baseline.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: f, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])
  // This page is the baseline's edit route (the report is its view), so an admin lands in edit
  // mode once the saved values are in the form.
  const autoEdit = useRef(!isNew)
  useEffect(() => {
    if (autoEdit.current && loaded) {
      autoEdit.current = false
      ed.startEdit()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loaded])
  const readOnly = ed.readOnly

  const set = (patch: Partial<Form>) => setF((cur) => ({ ...cur, ...patch }))
  const rulesQ = useComplianceRules(f.benchmark || undefined)
  const [pickerOpen, setPickerOpen] = useState(false)
  const allQ = useComplianceRules(f.benchmark && (pickerOpen || f.extra_rules.length > 0) ? f.benchmark : undefined, true)

  const ruleIndex = useMemo(() => {
    const m = new Map<string, ComplianceRule>()
    for (const r of allQ.data ?? []) m.set(r.id, r)
    for (const r of rulesQ.data ?? []) m.set(r.id, r)
    return m
  }, [rulesQ.data, allQ.data])

  const catBaselines = catalog.data?.baselines ?? []
  const current = catBaselines.find((b) => b.key === f.benchmark)
  const customized = Object.keys(f.exclusions).length + Object.keys(f.odv).length + f.extra_rules.length

  const pickBenchmark = (key: string) => {
    if (key === f.benchmark) {
      setChangingBenchmark(false)
      return
    }
    if (customized > 0) {
      setPendingBenchmark(key)
      return
    }
    applyBenchmark(key)
  }
  const applyBenchmark = (key: string) => {
    const b = catBaselines.find((x) => x.key === key)
    setF((cur) => {
      const oldDefault = cur.benchmark ? benchmarkName(cur.benchmark, catBaselines.find((x) => x.key === cur.benchmark)?.title) : ''
      const name = !cur.name.trim() || cur.name === oldDefault ? benchmarkName(key, b?.title) : cur.name
      return { ...cur, benchmark: key, name, exclusions: {}, odv: {}, extra_rules: [] }
    })
    setChangingBenchmark(false)
    setPendingBenchmark(null)
  }

  const save = async () => {
    const local = validate(f, ruleIndex)
    if (local.length) {
      setErrors(local.join('\n'))
      window.scrollTo({ top: 0, behavior: 'smooth' })
      return false
    }
    setErrors(null)
    setSaving(true)
    const odv_overrides: Record<string, unknown> = {}
    for (const [k, raw] of Object.entries(f.odv)) {
      if (raw.trim() === '') continue
      const r = ruleIndex.get(k)
      odv_overrides[k] = r && isNumeric(r) ? Number(raw) : raw
    }
    const exclusions: Record<string, string> = {}
    for (const [k, v] of Object.entries(f.exclusions)) exclusions[k] = v.trim()
    const body = {
      ...(baseline.data ?? {}),
      name: f.name.trim(),
      description: f.description.trim(),
      benchmark: f.benchmark,
      enabled: f.enabled,
      scope: f.scope,
      extra_rules: f.extra_rules,
      exclusions,
      odv_overrides,
      scan_interval_hours: intervalHours(f),
      auto_remediate: f.auto_remediate,
      deploy_profile: f.deploy_profile,
    }
    try {
      const out = isNew ? await api.post<ComplianceBaseline>('/api/v1/compliance/baselines', body) : await api.put<ComplianceBaseline>(`/api/v1/compliance/baselines/${id}`, body)
      await invalidate(qk.compliance, qk.profiles)
      toast.success(isNew ? 'Baseline created' : 'Baseline saved', f.enabled ? 'Macs in scope are scanned on their next check-in.' : undefined)
      navigate(`/compliance/${out.id}`, { replace: true })
      return true
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) {
        await invalidate(qk.compliance, qk.profiles)
        toast.warning('Saved, but the profile could not be generated', e.message)
        navigate(isNew ? '/compliance' : `/compliance/${id}`, { replace: true })
        return true
      }
      const msg = errorMessage(e)
      setErrors(msg)
      window.scrollTo({ top: 0, behavior: 'smooth' })
      toast.error('Save failed', e instanceof ApiError && e.status === 400 ? 'See the problems listed at the top.' : msg)
      return false
    } finally {
      setSaving(false)
    }
  }

  if (!isNew && baseline.isPending) return <LoadingBlock />
  if (!isNew && (baseline.error || !baseline.data)) return <Alert tone="critical">{baseline.error ? errorMessage(baseline.error) : 'Baseline not found'}</Alert>

  const showPicker = isNew ? !f.benchmark || changingBenchmark : changingBenchmark

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Compliance', to: '/compliance' }, ...(isNew ? [{ label: 'New baseline' }] : [{ label: baseline.data?.name ?? '', to: `/compliance/${id}` }, { label: 'Edit' }])]}
        title={isNew ? 'New compliance baseline' : `Edit ${baseline.data?.name ?? 'baseline'}`}
        description="Choose an mSCP benchmark, decide where it applies, then tailor its rules."
      />
      <div className="flex flex-col gap-6">
        {errors && (
          <Alert tone="critical" title="The baseline couldn't be saved">
            <ErrorList message={errors} />
          </Alert>
        )}

        <Card>
          <CardHeader title={<StepTitle n={1} title="Benchmark" />} description={showPicker ? 'Each benchmark is a curated set of mSCP rules. You can exempt or add rules afterwards.' : undefined} />
          <CardBody>
            {catalog.isPending ? (
              <LoadingBlock label="Loading the mSCP catalog…" />
            ) : catalog.error ? (
              <Alert tone="critical">{errorMessage(catalog.error)}</Alert>
            ) : showPicker ? (
              <BenchmarkPicker baselines={catBaselines} selected={f.benchmark} onPick={pickBenchmark} disabled={readOnly} onCancel={f.benchmark ? () => setChangingBenchmark(false) : undefined} />
            ) : (
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-ink">{benchmarkName(f.benchmark, current?.title)}</p>
                  <p className="text-xs text-ink-muted">
                    {current ? (
                      <>
                        {current.rules} rules · {current.scannable} scanned automatically · {current.with_profile} with profile settings
                      </>
                    ) : (
                      f.benchmark
                    )}
                    {catalog.data?.release && <> · mSCP {catalog.data.release}</>}
                  </p>
                </div>
                {!readOnly && (
                  <Button size="sm" variant="ghost" onClick={() => setChangingBenchmark(true)}>
                    Change
                  </Button>
                )}
              </div>
            )}
          </CardBody>
        </Card>

        {f.benchmark && (
          <>
            <SettingsCard f={f} set={set} readOnly={readOnly} />
            <TailorCard
              f={f}
              setF={setF}
              readOnly={readOnly}
              rules={rulesQ.data}
              rulesLoading={rulesQ.isPending}
              rulesError={rulesQ.error ? errorMessage(rulesQ.error) : null}
              ruleIndex={ruleIndex}
              allRules={allQ.data}
              allLoading={allQ.isFetching && !allQ.data}
              pickerOpen={pickerOpen}
              setPickerOpen={setPickerOpen}
            />
            {!isNew && id && <ProfilePreview id={id} dirty={seed !== '' && JSON.stringify(toForm(baseline.data!)) !== JSON.stringify(f)} />}
          </>
        )}
      </div>
      <ComplianceAttribution release={catalog.data?.release} />
      {/* The report page is this baseline's view: Cancel, Save and Done all go back there. */}
      <ObjectBar ed={ed} isNew listPath={isNew ? '/compliance' : `/compliance/${id}`} noun="baseline" name={baseline.data?.name} saving={saving} onSave={save} />
      <ConfirmDialog
        open={!!pendingBenchmark}
        onClose={() => setPendingBenchmark(null)}
        onConfirm={() => {
          if (pendingBenchmark) applyBenchmark(pendingBenchmark)
        }}
        title={`Switch to ${pendingBenchmark ? benchmarkName(pendingBenchmark, catBaselines.find((b) => b.key === pendingBenchmark)?.title) : ''}?`}
        description={`Your tailoring (${customized} ${customized === 1 ? 'change' : 'changes'}: exemptions, custom values and added rules) belongs to the current benchmark and will be cleared.`}
        confirmLabel="Switch benchmark"
      />
    </>
  )
}

function StepTitle({ n, title }: { n: number; title: string }) {
  return (
    <span className="inline-flex items-center gap-2">
      <span className="flex size-5 items-center justify-center rounded-full bg-accent-500/15 text-[11px] font-semibold text-accent-700 tabular-nums dark:text-accent-300">{n}</span>
      {title}
    </span>
  )
}

// ---- step 1 ----

function BenchmarkPicker({ baselines, selected, onPick, disabled, onCancel }: { baselines: ComplianceCatalogBaseline[]; selected: string; onPick: (k: string) => void; disabled?: boolean; onCancel?: () => void }) {
  const rank = (k: string) => {
    const i = benchmarkOrder.indexOf(k)
    return i < 0 ? 50 : i
  }
  const sorted = [...baselines].sort((a, b) => rank(a.key) - rank(b.key) || a.key.localeCompare(b.key))
  return (
    <div className="flex flex-col gap-3">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3" role="radiogroup" aria-label="Benchmark">
        {sorted.map((b) => {
          const active = b.key === selected
          return (
            <button
              key={b.key}
              type="button"
              role="radio"
              aria-checked={active}
              disabled={disabled}
              onClick={() => onPick(b.key)}
              className={cn(
                'flex flex-col gap-2 rounded-lg border p-3.5 text-left transition-colors focus-ring disabled:cursor-not-allowed',
                active ? 'border-accent-500/60 bg-accent-500/8' : 'border-line hover:border-line-strong',
              )}
            >
              <span className="flex items-start justify-between gap-2">
                <span className="text-sm font-medium text-ink">{benchmarkName(b.key, b.title)}</span>
                {active && <Check className="size-4 shrink-0 text-accent-400" aria-hidden="true" />}
              </span>
              <span className="text-xs text-ink-muted">{benchmarkBlurb(b.key, b.description)}</span>
              <span className="mt-auto flex flex-wrap gap-x-3 gap-y-0.5 pt-1 text-[11px] text-ink-muted tabular-nums">
                <span>
                  <span className="font-medium text-ink">{b.rules}</span> rules
                </span>
                <span>{b.scannable} scannable</span>
                <span>{b.with_profile} with profile settings</span>
              </span>
            </button>
          )
        })}
      </div>
      {onCancel && (
        <div>
          <Button size="sm" variant="ghost" onClick={onCancel}>
            Keep current benchmark
          </Button>
        </div>
      )}
    </div>
  )
}

// ---- step 2 ----

function SettingsCard({ f, set, readOnly }: { f: Form; set: (p: Partial<Form>) => void; readOnly: boolean }) {
  return (
    <Card>
      <CardHeader title={<StepTitle n={2} title="Settings and scope" />} />
      <CardBody className="flex flex-col gap-6">
        <FormGrid cols={2}>
          <Field label="Name" required>
            <Input value={f.name} onChange={(e) => set({ name: e.target.value })} placeholder="e.g. CIS Level 1 for staff Macs" disabled={readOnly} />
          </Field>
          <div className="flex items-end pb-1.5">
            <Toggle checked={f.enabled} onChange={(v) => set({ enabled: v })} disabled={readOnly} label="Enabled" description="Disabled baselines are not scanned and their profile is withdrawn." />
          </div>
        </FormGrid>
        <Field label="Description" hint="Optional. Shown on the report.">
          <Textarea rows={2} value={f.description} onChange={(e) => set({ description: e.target.value })} disabled={readOnly} />
        </Field>
        <Field label="Scan schedule" hint="The VibeMDM agent runs mSCP's own shell checks as root on each Mac at this interval.">
          <div className="flex flex-wrap items-center gap-2">
            <Select value={f.interval} onChange={(e) => set({ interval: e.target.value })} disabled={readOnly} className="w-48" aria-label="Scan interval">
              {intervalPresets.map((p) => (
                <option key={p.hours} value={String(p.hours)}>
                  {p.label}
                </option>
              ))}
              <option value="custom">Custom…</option>
            </Select>
            {f.interval === 'custom' && (
              <span className="inline-flex items-center gap-2 text-[13px] text-ink-muted">
                every
                <Input type="number" min={1} max={720} step={1} value={f.customHours} onChange={(e) => set({ customHours: e.target.value })} className="w-24" disabled={readOnly} aria-label="Custom interval in hours" />
                hours (1–720)
              </span>
            )}
          </div>
        </Field>

        <div className="grid gap-4 md:grid-cols-2">
          <div className="flex flex-col gap-3 rounded-lg border border-line p-3.5">
            <Toggle
              checked={f.auto_remediate}
              onChange={(v) => set({ auto_remediate: v })}
              disabled={readOnly}
              label="Auto-remediate"
              description="Run mSCP's fix script for each failing rule, then check again. Fixed rules show as remediated."
            />
            {f.auto_remediate && (
              <Alert tone="warning">
                Fix scripts run as root and change system settings on every Mac in scope without asking the user. Try the baseline on a small test group first. Some settings can
                only be enforced with a configuration profile.
              </Alert>
            )}
          </div>
          <div className="flex flex-col gap-3 rounded-lg border border-line p-3.5">
            <Toggle
              checked={f.deploy_profile}
              onChange={(v) => set({ deploy_profile: v })}
              disabled={readOnly}
              label="Deploy configuration profile"
              description={
                <>
                  Generate one profile from the benchmark&apos;s settings, with your custom values applied and exempted rules left out, and install it on this scope over MDM. It
                  appears in Profiles as &ldquo;Compliance: {f.name.trim() || 'name'}&rdquo; and is regenerated whenever you save.
                </>
              }
            />
          </div>
        </div>

        <div>
          <p className="mb-1.5 text-sm font-medium text-ink">Scope</p>
          <p className="mb-3 text-xs text-ink-muted">Macs in scope are scanned on schedule and receive the profile when it is deployed.</p>
          {readOnly ? <ScopeSummary scope={f.scope} /> : <ScopeEditor value={f.scope} onChange={(scope) => set({ scope })} />}
        </div>
      </CardBody>
    </Card>
  )
}

// ---- step 3 ----

type RuleFilter = 'all' | 'exempt' | 'custom' | 'fix' | 'manual'

interface TailorProps {
  f: Form
  setF: React.Dispatch<React.SetStateAction<Form>>
  readOnly: boolean
  rules: ComplianceRule[] | undefined
  rulesLoading: boolean
  rulesError: string | null
  ruleIndex: Map<string, ComplianceRule>
  allRules: ComplianceRule[] | undefined
  allLoading: boolean
  pickerOpen: boolean
  setPickerOpen: (v: boolean) => void
}

function TailorCard({ f, setF, readOnly, rules, rulesLoading, rulesError, ruleIndex, allRules, allLoading, pickerOpen, setPickerOpen }: TailorProps) {
  const [filter, setFilter] = useState<RuleFilter>('all')
  const [search, setSearch] = useState('')
  const [exempting, setExempting] = useState<ComplianceRule | null>(null)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())

  const extra = f.extra_rules.map((id) => ruleIndex.get(id) ?? ({ id, title: id, section: 'supplemental', discussion: '', tags: [], severity: '', manual_reason: '', has_fix: false, has_profile: false, platforms: [], in_benchmark: false } as ComplianceRule))
  const list = [...(rules ?? []), ...extra.filter((r) => !(rules ?? []).some((x) => x.id === r.id))]
  const added = new Set(f.extra_rules)

  const isCustom = (r: ComplianceRule) => (f.odv[r.id] ?? '') !== '' || added.has(r.id)
  const counts: Record<RuleFilter, number> = {
    all: list.length,
    exempt: list.filter((r) => f.exclusions[r.id] !== undefined).length,
    custom: list.filter(isCustom).length,
    fix: list.filter((r) => r.has_fix).length,
    manual: list.filter((r) => !!r.manual_reason).length,
  }
  const s = search.trim().toLowerCase()
  const shown = list.filter((r) => {
    if (filter === 'exempt' && f.exclusions[r.id] === undefined) return false
    if (filter === 'custom' && !isCustom(r)) return false
    if (filter === 'fix' && !r.has_fix) return false
    if (filter === 'manual' && !r.manual_reason) return false
    if (!s) return true
    return r.title.toLowerCase().includes(s) || r.id.toLowerCase().includes(s) || (r.tags ?? []).some((t) => t.toLowerCase() === s)
  })

  const enforce = (id: string) =>
    setF((cur) => {
      const ex = { ...cur.exclusions }
      delete ex[id]
      return { ...cur, exclusions: ex }
    })
  const setOdv = (id: string, v: string) =>
    setF((cur) => {
      const odv = { ...cur.odv }
      if (v === '') delete odv[id]
      else odv[id] = v
      return { ...cur, odv }
    })
  const setReason = (id: string, reason: string) => setF((cur) => ({ ...cur, exclusions: { ...cur.exclusions, [id]: reason } }))
  const removeExtra = (id: string) =>
    setF((cur) => {
      const ex = { ...cur.exclusions }
      const odv = { ...cur.odv }
      delete ex[id]
      delete odv[id]
      return { ...cur, extra_rules: cur.extra_rules.filter((x) => x !== id), exclusions: ex, odv }
    })

  const exemptN = counts.exempt
  const odvN = Object.keys(f.odv).length
  return (
    <Card>
      <CardHeader
        title={<StepTitle n={3} title="Tailor rules" />}
        description={
          <>
            <span className="tabular-nums">{list.length}</span> rules · <span className="tabular-nums">{list.length - exemptN}</span> enforced
            {exemptN > 0 && (
              <>
                {' '}
                · <span className="tabular-nums">{exemptN}</span> exempt
              </>
            )}
            {odvN > 0 && (
              <>
                {' '}
                · <span className="tabular-nums">{odvN}</span> custom {odvN === 1 ? 'value' : 'values'}
              </>
            )}
            {f.extra_rules.length > 0 && (
              <>
                {' '}
                · <span className="tabular-nums">{f.extra_rules.length}</span> added
              </>
            )}
            . Exempt rules are neither scanned nor enforced. Rules for macOS versions a Mac doesn&apos;t run show as not applicable.
          </>
        }
        actions={
          !readOnly && (
            <Button size="sm" icon={<ListPlus />} onClick={() => setPickerOpen(true)}>
              Add rules
            </Button>
          )
        }
      />
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-2.5">
        <Tabs<RuleFilter>
          variant="pills"
          value={filter}
          onChange={setFilter}
          items={[
            { key: 'all', label: 'All', count: counts.all },
            { key: 'exempt', label: 'Exempted', count: counts.exempt },
            { key: 'custom', label: 'Customized', count: counts.custom },
            { key: 'fix', label: 'Has fix', count: counts.fix },
            { key: 'manual', label: 'Manual', count: counts.manual },
          ]}
        />
        <SearchInput value={search} onChange={setSearch} placeholder="Search rules by title or ID…" className="w-full sm:w-72" />
      </div>
      {rulesLoading ? (
        <LoadingBlock label="Loading rules…" />
      ) : rulesError ? (
        <Alert tone="critical" className="m-4">
          {rulesError}
        </Alert>
      ) : shown.length === 0 ? (
        <EmptyState compact title="No rules match" description={s ? 'Try a different search.' : 'Nothing in this filter yet.'} />
      ) : (
        <div>
          {bySection(shown).map(([section, items]) => {
            const isCollapsed = collapsed.has(section) && !s
            return (
              <section key={section} className="border-b border-line last:border-b-0">
                <button
                  type="button"
                  className="flex w-full items-center gap-2 bg-surface-sunken/40 px-4 py-2 text-left focus-ring"
                  onClick={() =>
                    setCollapsed((cur) => {
                      const n = new Set(cur)
                      if (n.has(section)) n.delete(section)
                      else n.add(section)
                      return n
                    })
                  }
                  aria-expanded={!isCollapsed}
                >
                  {isCollapsed ? <ChevronRight className="size-3.5 text-ink-faint" /> : <ChevronDown className="size-3.5 text-ink-faint" />}
                  <span className="micro-label text-ink">{sectionLabel(section)}</span>
                  <span className="text-xs text-ink-faint tabular-nums">{items.length}</span>
                </button>
                {!isCollapsed && (
                  <ul>
                    {items.map((r) => (
                      <RuleRow
                        key={r.id}
                        r={r}
                        benchmark={f.benchmark}
                        readOnly={readOnly}
                        exemptReason={f.exclusions[r.id]}
                        odvValue={f.odv[r.id] ?? ''}
                        added={added.has(r.id)}
                        onEnforce={() => enforce(r.id)}
                        onExempt={() => setExempting(r)}
                        onReason={(v) => setReason(r.id, v)}
                        onOdv={(v) => setOdv(r.id, v)}
                        onRemove={() => removeExtra(r.id)}
                      />
                    ))}
                  </ul>
                )}
              </section>
            )
          })}
        </div>
      )}
      <ExemptDialog
        rule={exempting}
        onClose={() => setExempting(null)}
        onConfirm={(reason) => {
          if (exempting) setReason(exempting.id, reason)
          setExempting(null)
        }}
      />
      <AddRulesDialog
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        rules={allRules}
        loading={allLoading}
        existing={new Set(list.map((r) => r.id))}
        onAdd={(ids) => setF((cur) => ({ ...cur, extra_rules: [...cur.extra_rules, ...ids.filter((x) => !cur.extra_rules.includes(x))] }))}
      />
    </Card>
  )
}

interface RuleRowProps {
  r: ComplianceRule
  benchmark: string
  readOnly: boolean
  exemptReason: string | undefined
  odvValue: string
  added: boolean
  onEnforce: () => void
  onExempt: () => void
  onReason: (v: string) => void
  onOdv: (v: string) => void
  onRemove: () => void
}

function RuleRow({ r, benchmark, readOnly, exemptReason, odvValue, added, onEnforce, onExempt, onReason, onOdv, onRemove }: RuleRowProps) {
  const [open, setOpen] = useState(false)
  const exempt = exemptReason !== undefined
  return (
    <li className={cn('border-t border-line first:border-t-0', exempt && 'bg-surface-sunken/30')}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2.5">
        <button type="button" className="flex min-w-0 flex-1 basis-72 items-start gap-2 text-left focus-ring rounded" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
          {open ? <ChevronDown className="mt-0.5 size-4 shrink-0 text-ink-faint" /> : <ChevronRight className="mt-0.5 size-4 shrink-0 text-ink-faint" />}
          <span className="min-w-0">
            <span className={cn('block text-[13px] font-medium', exempt ? 'text-ink-muted line-through decoration-ink-faint/60' : 'text-ink')}>{r.title}</span>
            <span className="mt-0.5 flex flex-wrap items-center gap-1.5">
              <span className="font-mono text-[11px] text-ink-faint">{r.id}</span>
              <RuleSeverityBadge severity={r.severity} />
              <RuleCapabilityBadges hasFix={r.has_fix} hasProfile={r.has_profile} manualReason={r.manual_reason} />
              {added && (
                <Badge tone="accent" icon={<Plus />}>
                  Added
                </Badge>
              )}
              {exempt && <Badge icon={<MinusCircle />}>Exempt</Badge>}
            </span>
          </span>
        </button>
        {r.odv && !exempt && <OdvInput r={r} value={odvValue} onChange={onOdv} readOnly={readOnly} />}
        <div className="flex items-center gap-1.5">
          <EnforceSwitch exempt={exempt} disabled={readOnly} onEnforce={onEnforce} onExempt={onExempt} />
          {added && !readOnly && (
            <IconButton label="Remove added rule" size="xs" variant="ghost" onClick={onRemove}>
              <X />
            </IconButton>
          )}
        </div>
      </div>
      {exempt && (
        <div className="flex flex-wrap items-center gap-2 px-4 pb-2.5 pl-10">
          <label className="text-xs text-ink-muted" htmlFor={`reason-${r.id}`}>
            Reason
          </label>
          <Input
            id={`reason-${r.id}`}
            value={exemptReason}
            onChange={(e) => onReason(e.target.value)}
            disabled={readOnly}
            className={cn('h-7 max-w-xl flex-1 text-xs', !exemptReason.trim() && 'border-critical/60')}
            placeholder="Why this rule doesn't apply (required)"
          />
        </div>
      )}
      {open && (
        <div className="flex flex-col gap-4 px-4 pb-4 pl-10">
          {r.manual_reason && (
            <p className="text-xs text-ink-muted">
              <span className="font-medium text-ink">Not scanned automatically:</span> {r.manual_reason}
            </p>
          )}
          {r.odv && (
            <p className="text-xs text-ink-muted">
              <span className="font-medium text-ink">Organization-defined value:</span> {r.odv.hint.description}
              {r.odv.hint.min !== undefined || r.odv.hint.max !== undefined ? (
                <span className="tabular-nums">
                  {' '}
                  (range {r.odv.hint.min ?? '…'}–{r.odv.hint.max ?? '…'})
                </span>
              ) : null}
              . Benchmark default: <span className="font-mono text-ink">{String(r.odv.default ?? '—')}</span>
            </p>
          )}
          <Discussion text={r.discussion} />
          {r.references && Object.keys(r.references).length > 0 && (
            <div>
              <p className="micro-label mb-1.5">References</p>
              <References refs={r.references} />
            </div>
          )}
          {(r.platforms?.length ?? 0) > 0 && (
            <p className="text-xs text-ink-muted">
              Applies to macOS {r.platforms!.join(', ')}
              {r.arch ? ` · ${r.arch} only` : ''}
            </p>
          )}
          <RuleScripts ruleId={r.id} benchmark={benchmark} />
        </div>
      )}
    </li>
  )
}

/** Two-state segmented control so the current choice is always readable as text. */
function EnforceSwitch({ exempt, disabled, onEnforce, onExempt }: { exempt: boolean; disabled?: boolean; onEnforce: () => void; onExempt: () => void }) {
  const base = 'inline-flex items-center gap-1 px-2 h-7 text-xs font-medium transition-colors focus-ring disabled:cursor-not-allowed'
  return (
    <div className="inline-flex overflow-hidden rounded-md border border-line-strong/70" role="radiogroup" aria-label="Enforce or exempt">
      <button type="button" role="radio" aria-checked={!exempt} disabled={disabled} onClick={() => exempt && onEnforce()} className={cn(base, !exempt ? 'bg-accent-500/15 text-ink' : 'text-ink-muted hover:text-ink')}>
        {!exempt && <Check className="size-3.5 text-accent-400" aria-hidden="true" />}
        Enforce
      </button>
      <button
        type="button"
        role="radio"
        aria-checked={exempt}
        disabled={disabled}
        onClick={() => !exempt && onExempt()}
        className={cn(base, 'border-l border-line-strong/70', exempt ? 'bg-line/60 text-ink' : 'text-ink-muted hover:text-ink')}
      >
        {exempt && <MinusCircle className="size-3.5" aria-hidden="true" />}
        Exempt
      </button>
    </div>
  )
}

function OdvInput({ r, value, onChange, readOnly }: { r: ComplianceRule; value: string; onChange: (v: string) => void; readOnly: boolean }) {
  const odv = r.odv!
  const def = odv.default === null || odv.default === undefined ? '' : String(odv.default)
  const problem = odvProblem(r, value)
  const choices = enumChoices(r)
  const label = `Value for ${r.title}`
  return (
    <div className="flex items-center gap-1" title={odv.hint.description}>
      {choices ? (
        <Select value={value} onChange={(e) => onChange(e.target.value)} disabled={readOnly} className="h-7 w-40 text-xs" aria-label={label}>
          <option value="">Default ({def || 'none'})</option>
          {choices.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </Select>
      ) : isNumeric(r) ? (
        <Input
          type="number"
          min={odv.hint.min}
          max={odv.hint.max}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={def}
          disabled={readOnly}
          className={cn('h-7 w-24 text-xs tabular-nums', problem && 'border-critical/60')}
          aria-label={label}
          aria-invalid={!!problem}
        />
      ) : (
        <Input mono value={value} onChange={(e) => onChange(e.target.value)} placeholder={def} disabled={readOnly} className={cn('h-7 w-44 text-xs', problem && 'border-critical/60')} aria-label={label} aria-invalid={!!problem} />
      )}
      {value !== '' && !readOnly ? (
        <IconButton label="Reset to benchmark default" size="xs" variant="ghost" onClick={() => onChange('')}>
          <RotateCcw />
        </IconButton>
      ) : (
        <span className="w-7" aria-hidden="true" />
      )}
      {problem && <span className="text-[11px] text-critical">{problem}</span>}
    </div>
  )
}

function ExemptDialog({ rule, onClose, onConfirm }: { rule: ComplianceRule | null; onClose: () => void; onConfirm: (reason: string) => void }) {
  const [reason, setReason] = useState('')
  useEffect(() => {
    if (rule) setReason('')
  }, [rule])
  return (
    <Modal
      open={!!rule}
      onClose={onClose}
      size="sm"
      title="Exempt this rule"
      description={rule?.title}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} data-shortcut="cancel">
            Cancel
          </Button>
          <Button variant="primary" data-shortcut="save" disabled={!reason.trim()} onClick={() => onConfirm(reason.trim())}>
            Exempt rule
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-ink-muted">Exempt rules are not scanned, not remediated and left out of the profile. The reason is kept with the baseline for auditors.</p>
        <Field label="Reason" required>
          <Textarea
            rows={3}
            autoFocus
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="e.g. Bluetooth is required for our conference room keyboards (risk accepted by IT security, 2026-10)"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && reason.trim()) onConfirm(reason.trim())
            }}
          />
        </Field>
      </div>
    </Modal>
  )
}

function AddRulesDialog({ open, onClose, rules, loading, existing, onAdd }: { open: boolean; onClose: () => void; rules: ComplianceRule[] | undefined; loading: boolean; existing: Set<string>; onAdd: (ids: string[]) => void }) {
  const [search, setSearch] = useState('')
  const [from, setFrom] = useState('')
  const [picked, setPicked] = useState<Set<string>>(new Set())
  useEffect(() => {
    if (open) setPicked(new Set())
  }, [open])
  const candidates = (rules ?? []).filter((r) => !r.in_benchmark && !existing.has(r.id))
  const benchmarks = [...new Set(candidates.flatMap((r) => (r.tags ?? []).filter((t) => benchmarkOrder.includes(t))))].sort((a, b) => benchmarkOrder.indexOf(a) - benchmarkOrder.indexOf(b))
  const s = search.trim().toLowerCase()
  const shown = candidates.filter((r) => (!from || (r.tags ?? []).includes(from)) && (!s || r.title.toLowerCase().includes(s) || r.id.toLowerCase().includes(s)))
  const toggle = (id: string) =>
    setPicked((cur) => {
      const n = new Set(cur)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  return (
    <Modal
      open={open}
      onClose={onClose}
      size="xl"
      title="Add rules from other benchmarks"
      description="Added rules are scanned and enforced like the benchmark's own, and can be removed again at any time."
      footer={
        <>
          <Button variant="ghost" onClick={onClose} data-shortcut="cancel">
            Cancel
          </Button>
          <Button
            variant="primary"
            data-shortcut="save"
            icon={<Plus />}
            disabled={picked.size === 0}
            onClick={() => {
              onAdd([...picked])
              onClose()
            }}
          >
            {picked.size ? `Add ${picked.size} ${picked.size === 1 ? 'rule' : 'rules'}` : 'Add rules'}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap gap-2">
          <SearchInput value={search} onChange={setSearch} placeholder="Search by title or ID…" className="min-w-56 flex-1" autoFocus />
          <Select value={from} onChange={(e) => setFrom(e.target.value)} className="w-56" aria-label="From benchmark">
            <option value="">Any benchmark</option>
            {benchmarks.map((b) => (
              <option key={b} value={b}>
                {benchmarkName(b)}
              </option>
            ))}
          </Select>
        </div>
        {loading ? (
          <LoadingBlock label="Loading the full catalog…" />
        ) : shown.length === 0 ? (
          <EmptyState compact title={candidates.length ? 'No rules match' : 'Every catalog rule is already included'} />
        ) : (
          <ul className="max-h-[55vh] overflow-y-auto rounded-lg border border-line">
            {shown.map((r) => {
              const on = picked.has(r.id)
              const inBench = (r.tags ?? []).filter((t) => benchmarkOrder.includes(t) && t !== 'all_rules')
              return (
                <li key={r.id} className="border-b border-line last:border-b-0">
                  <label className={cn('flex cursor-pointer items-start gap-3 px-3 py-2 hover:bg-surface-sunken/60', on && 'bg-accent-500/6')}>
                    <input type="checkbox" checked={on} onChange={() => toggle(r.id)} className="mt-1 size-4 shrink-0 accent-accent-600" />
                    <span className="min-w-0 flex-1">
                      <span className="block text-[13px] font-medium text-ink">{r.title}</span>
                      <span className="mt-0.5 flex flex-wrap items-center gap-1.5">
                        <span className="font-mono text-[11px] text-ink-faint">{r.id}</span>
                        <span className="text-[11px] text-ink-muted">{sectionLabel(r.section)}</span>
                        <RuleCapabilityBadges hasFix={r.has_fix} hasProfile={r.has_profile} manualReason={r.manual_reason} />
                      </span>
                      {inBench.length > 0 && <span className="mt-0.5 block text-[11px] text-ink-faint">In {inBench.map((t) => benchmarkName(t)).join(', ')}</span>}
                    </span>
                  </label>
                </li>
              )
            })}
          </ul>
        )}
        <p className="text-xs text-ink-faint">
          {shown.length.toLocaleString()} of {candidates.length.toLocaleString()} rules not already in this baseline
        </p>
      </div>
    </Modal>
  )
}

// ---- profile preview ----

function ProfilePreview({ id, dirty }: { id: string; dirty: boolean }) {
  const [open, setOpen] = useState(false)
  const q = useComplianceProfile(id, open)
  const covered = q.data?.rules ?? []
  return (
    <Card>
      <button type="button" className="flex w-full items-start justify-between gap-4 px-4 py-3 text-left focus-ring rounded-xl" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        <span className="min-w-0">
          <span className="flex items-center gap-2 text-[13px] font-semibold text-ink">
            <FileCode2 className="size-4 text-ink-faint" aria-hidden="true" />
            Profile preview
          </span>
          <span className="mt-0.5 block text-xs text-ink-muted">The configuration profile this baseline generates from its saved settings.</span>
        </span>
        {open ? <ChevronDown className="mt-0.5 size-4 text-ink-faint" /> : <ChevronRight className="mt-0.5 size-4 text-ink-faint" />}
      </button>
      {open && (
        <CardBody className="flex flex-col gap-3 border-t border-line">
          {dirty && <Alert tone="info">You have unsaved changes. The preview shows the last saved version; save to regenerate it.</Alert>}
          {q.isPending ? (
            <div className="flex items-center gap-2 text-xs text-ink-muted">
              <Spinner className="size-3.5" /> Generating…
            </div>
          ) : q.error ? (
            q.error instanceof ApiError && q.error.status === 422 ? (
              <p className="text-[13px] text-ink-muted">No profile: {q.error.message}</p>
            ) : (
              <Alert tone="critical">{errorMessage(q.error)}</Alert>
            )
          ) : q.data ? (
            <>
              <div className="flex flex-wrap items-center gap-2 text-xs text-ink-muted">
                <Badge icon={<ShieldCheck />}>
                  Covers {covered.length} {covered.length === 1 ? 'rule' : 'rules'}
                </Badge>
                {q.data.deployed ? (
                  <Badge tone="good" icon={<Check />}>
                    Deployed
                  </Badge>
                ) : (
                  <Badge icon={<Wrench />}>Not deployed (turn on “Deploy configuration profile”)</Badge>
                )}
                {q.data.profile_id && (
                  <Link to={`/profiles/${q.data.profile_id}`} className="text-accent-700 hover:underline dark:text-accent-300">
                    Open in Profiles
                  </Link>
                )}
              </div>
              <CodeBlock text={q.data.payload} filename="Compliance profile (.mobileconfig)" maxHeight="24rem" />
            </>
          ) : null}
        </CardBody>
      )}
      {open && <CardFooter className="justify-start text-xs text-ink-muted">Rules without profile settings are enforced only by scanning and, if enabled, auto-remediation.</CardFooter>}
    </Card>
  )
}
