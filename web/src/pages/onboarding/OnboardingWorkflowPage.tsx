import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowDown, ArrowUp, Ban, ChevronDown, Laptop, LogOut, Play, Plus, Power, RotateCcw, X } from 'lucide-react'
import { qk, useApps, useInvalidate, useOnboardingRuns, useOnboardingWorkflow, useOnboardingWorkflows } from '../../api/queries'
import { api, ApiError, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { OnboardingConfig, OnboardingField, OnboardingFinalAction, OnboardingStep, OnboardingStepType, OnboardingWorkflow } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button, IconButton } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Checkbox, Toggle } from '../../components/ui/Checkbox'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select, Textarea } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { Tabs } from '../../components/ui/Tabs'
import { Dropdown } from '../../components/ui/Dropdown'
import { LoadingBlock } from '../../components/ui/Spinner'
import { useToast } from '../../components/ui/Toast'
import { ScopeEditor, ScopeSummary } from '../../components/pickers/ScopeEditor'
import { ChipInput } from '../../components/pickers/ChipInput'
import { PolicyPicker, ScriptPicker } from '../../components/pickers/ScriptPicker'
import {
  RunOnComputersDialog,
  RunsTable,
  emptyWorkflow,
  newField,
  newStep,
  normalizeWorkflow,
  orderedFieldKeys,
  renderNameTemplate,
  sampleNameValues,
  stepTypeMeta,
  validateWorkflow,
} from '../../components/onboarding/onboarding'
import { ErrorList } from '../updates/SoftwareUpdatesPage'
import { cn } from '../../lib/cn'
import { useCloneSource, useEditMode } from '../../components/editor/useEditMode'
import { ObjectBar } from '../../components/editor/ObjectBar'
import { DetailsLayout, SummaryCard } from '../../components/templates/DetailsLayout'
import { RelativeTime } from '../../components/ui/RelativeTime'

type TabKey = 'workflow' | 'runs'

export function OnboardingWorkflowPage() {
  const { id, tab } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const wf = useOnboardingWorkflow(isNew ? undefined : id)
  const list = useOnboardingWorkflows()
  const fieldKeys = list.data?.field_keys
  const clone = useCloneSource<OnboardingWorkflow>()
  const [f, setF] = useState<OnboardingWorkflow>(() => (isNew && clone ? normalizeWorkflow(clone) : emptyWorkflow()))
  const [errors, setErrors] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [runOpen, setRunOpen] = useState(false)
  // The tab is part of the URL, but changing the URL would end edit mode (and ask about unsaved
  // changes), so while editing the tab only changes here.
  const [tabState, setTabState] = useState<TabKey>(tab === 'runs' ? 'runs' : 'workflow')
  useEffect(() => setTabState(tab === 'runs' ? 'runs' : 'workflow'), [tab])
  const active: TabKey = !isNew && tabState === 'runs' ? 'runs' : 'workflow'
  const setTab = (k: TabKey) => {
    setTabState(k)
    if (!ed.editing) navigate(`/onboarding/${id}${k === 'runs' ? '/runs' : ''}`, { replace: true })
  }

  const load = useCallback(() => {
    if (wf.data) setF(normalizeWorkflow(wf.data))
  }, [wf.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: f, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])
  const readOnly = ed.readOnly

  const set = (patch: Partial<OnboardingWorkflow>) => setF((cur) => ({ ...cur, ...patch }))
  const setC = (patch: Partial<OnboardingConfig>) => setF((cur) => ({ ...cur, config: { ...cur.config, ...patch } }))

  const save = async () => {
    const local = validateWorkflow(f)
    if (local.length) {
      setErrors(local.join('\n'))
      if (active !== 'workflow') setTab('workflow')
      window.scrollTo({ top: 0, behavior: 'smooth' })
      return false
    }
    setErrors(null)
    setSaving(true)
    const body: OnboardingWorkflow = {
      ...f,
      name: f.name.trim(),
      config: {
        ...f.config,
        title: f.config.title.trim(),
        fields: (f.config.fields ?? []).map((x) => ({ ...x, label: x.label.trim(), options: (x.options ?? []).filter(Boolean) })),
        steps: (f.config.steps ?? []).map((s) => ({ ...s, title: s.title.trim() })),
      },
    }
    try {
      const out = isNew ? await api.post<OnboardingWorkflow>('/api/v1/onboarding/workflows', body) : await api.put<OnboardingWorkflow>(`/api/v1/onboarding/workflows/${id}`, body)
      await invalidate(qk.onboarding)
      toast.success(isNew ? 'Workflow created' : 'Workflow saved')
      if (isNew) navigate(`/onboarding/${out.id}`, { replace: true })
      return true
    } catch (e) {
      const msg = errorMessage(e)
      setErrors(msg)
      window.scrollTo({ top: 0, behavior: 'smooth' })
      if (active !== 'workflow') setTab('workflow')
      toast.error('Save failed', e instanceof ApiError && e.status === 400 ? 'See the problems listed above the form.' : msg)
      return false
    } finally {
      setSaving(false)
    }
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/onboarding/workflows/${id}`)
      await invalidate(qk.onboarding)
      toast.success('Workflow deleted')
      navigate('/onboarding')
      return true
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
      return false
    }
  }

  if (!isNew && wf.isPending) return <LoadingBlock />
  if (!isNew && (wf.error || !wf.data)) return <Alert tone="critical">{wf.error ? errorMessage(wf.error) : 'Workflow not found'}</Alert>

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Onboarding', to: '/onboarding' }, { label: isNew ? 'New workflow' : wf.data?.name ?? '' }]}
        title={isNew ? 'New onboarding workflow' : wf.data?.name ?? 'Onboarding workflow'}
        description="Shown by the VibeMDM agent, with swiftDialog, the first time a user is logged in after enrollment."
      />

      {!isNew && <Tabs<TabKey> items={[{ key: 'workflow', label: 'Workflow' }, { key: 'runs', label: 'Runs', shortcut: 'logs' }]} value={active} onChange={setTab} className="mb-5" />}

      {active === 'runs' && !isNew ? (
        <RunsTab workflowId={id!} fieldKeys={fieldKeys} canRun={isAdmin} onRun={() => setRunOpen(true)} />
      ) : (
        <DetailsLayout
          aside={
            !isNew && (
              <SummaryCard
                rows={[
                  { label: 'Status', value: <Badge tone={f.enabled ? 'good' : 'neutral'} dot>{f.enabled ? 'Enabled' : 'Disabled'}</Badge> },
                  { label: 'Priority', value: <span className="tabular-nums">{f.priority}</span> },
                  { label: 'Scope', value: <ScopeSummary scope={f.scope} /> },
                  { label: 'Fields', value: <span className="tabular-nums">{f.config.fields?.length ?? 0}</span> },
                  { label: 'Steps', value: <span className="tabular-nums">{f.config.steps?.length ?? 0}</span> },
                  { label: 'Last changed', value: wf.data?.updated_at ? <RelativeTime value={wf.data.updated_at} /> : '' },
                ]}
              />
            )
          }
        >
        <div className="flex flex-col gap-6">
          {errors && (
            <Alert tone="critical" title="The workflow couldn't be saved">
              <ErrorList message={errors} />
            </Alert>
          )}

          <Card>
            <CardHeader title="Basics" description="Only the highest-priority enabled workflow whose scope includes a Mac runs on it, once." />
            <CardBody className="flex flex-col gap-5">
              <FormGrid cols={3}>
                <Field label="Name" required>
                  <Input value={f.name} onChange={(e) => set({ name: e.target.value })} placeholder="e.g. New hire setup" disabled={readOnly} />
                </Field>
                <Field label="Priority" hint="Lower numbers win when several workflows include the same Mac. Default 100.">
                  <Input type="number" min={0} step={1} value={f.priority} onChange={(e) => set({ priority: Math.max(0, Math.floor(Number(e.target.value) || 0)) })} disabled={readOnly} className="max-w-32" />
                </Field>
                <div className="flex items-end pb-1.5">
                  <Toggle checked={f.enabled} onChange={(v) => set({ enabled: v })} disabled={readOnly} label="Enabled" description="Disabled workflows never run automatically; admins can still run them by hand." />
                </div>
              </FormGrid>
              <div>
                <p className="mb-2 text-sm font-medium text-ink">Scope</p>
                {readOnly ? <ScopeSummary scope={f.scope} /> : <ScopeEditor value={f.scope} onChange={(scope) => set({ scope })} />}
              </div>
            </CardBody>
          </Card>

          <WelcomeCard c={f.config} setC={setC} readOnly={readOnly} fieldKeys={fieldKeys} />
          <FieldsCard c={f.config} setC={setC} readOnly={readOnly} fieldKeys={fieldKeys} />
          <NameTemplateCard c={f.config} setC={setC} readOnly={readOnly} />
          <StepsCard c={f.config} setC={setC} readOnly={readOnly} />
          <FinishCard c={f.config} setC={setC} readOnly={readOnly} />
        </div>
        </DetailsLayout>
      )}

      <ObjectBar
        ed={ed}
        isNew={isNew}
        listPath="/onboarding"
        noun="workflow"
        name={wf.data?.name}
        saving={saving}
        onSave={save}
        onDelete={doDelete}
        deleteDescription="The workflow and its run history are removed. Macs that are mid-run finish what they have started."
        onClone={() => navigate('/onboarding/new', { state: { clone: { ...f, id: '', name: `Copy of ${f.name}`, enabled: false } } })}
        historyId={id}
        extras={
          !isNew &&
          isAdmin && (
            <Button variant="ghost" icon={<Play />} onClick={() => setRunOpen(true)}>
              Run on computers…
            </Button>
          )
        }
      />
      {!isNew && wf.data && <RunOnComputersDialog open={runOpen} onClose={() => setRunOpen(false)} workflow={wf.data} />}
    </>
  )
}

// ---- runs ----

function RunsTab({ workflowId, fieldKeys, canRun, onRun }: { workflowId: string; fieldKeys?: Record<string, string>; canRun: boolean; onRun: () => void }) {
  const q = useOnboardingRuns(workflowId, { limit: 200 })
  return (
    <Card>
      <RunsTable
        runs={q.data}
        loading={q.isPending}
        error={q.error ? errorMessage(q.error) : null}
        fieldKeys={fieldKeys}
        emptyDescription="Runs appear when a Mac in scope checks in with a user logged in, or when an admin runs the workflow on chosen computers."
        emptyAction={
          canRun ? (
            <Button icon={<Play />} onClick={onRun}>
              Run on computers…
            </Button>
          ) : undefined
        }
      />
    </Card>
  )
}

// ---- welcome screen ----

type ConfigProps = { c: OnboardingConfig; setC: (p: Partial<OnboardingConfig>) => void; readOnly: boolean }

function WelcomeCard({ c, setC, readOnly, fieldKeys }: ConfigProps & { fieldKeys?: Record<string, string> }) {
  return (
    <Card>
      <CardHeader title="Welcome screen" description="The first thing the user sees. Fields from the next section are collected on this screen." />
      <CardBody>
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)]">
          <div className="flex flex-col gap-4">
            <Field label="Title" hint="Defaults to “Setting up your Mac” when left blank.">
              <Input value={c.title} onChange={(e) => setC({ title: e.target.value })} placeholder="Welcome to Acme" disabled={readOnly} />
            </Field>
            <Field label="Message" hint="Markdown is allowed: **bold**, lists and links render in the dialog. The preview shows plain text.">
              <Textarea value={c.message} onChange={(e) => setC({ message: e.target.value })} rows={5} placeholder={'Hi! We are setting up this Mac for you.\n\nPlease fill in the details below, then wait while your apps install.'} disabled={readOnly} />
            </Field>
            <FormGrid cols={2}>
              <Field label="Icon" hint="A path on the Mac, an image URL, or an SF Symbol such as SF=laptopcomputer.">
                <Input value={c.icon} onChange={(e) => setC({ icon: e.target.value })} placeholder="SF=laptopcomputer" disabled={readOnly} mono />
              </Field>
              <Field label="Banner image URL" hint="Optional wide image shown across the top of the dialog.">
                <Input value={c.banner} onChange={(e) => setC({ banner: e.target.value })} placeholder="https://…/banner.png" disabled={readOnly} mono />
              </Field>
            </FormGrid>
            <Toggle checked={c.blur_screen} onChange={(v) => setC({ blur_screen: v })} disabled={readOnly} label="Blur the screen behind the dialog" description="Keeps the user focused on setup until it finishes." />
          </div>
          <div>
            <p className="micro-label mb-2">Preview</p>
            <DialogPreview c={c} fieldKeys={fieldKeys} />
          </div>
        </div>
      </CardBody>
    </Card>
  )
}

function IconPreview({ icon }: { icon: string }) {
  const [broken, setBroken] = useState(false)
  useEffect(() => setBroken(false), [icon])
  const v = icon.trim()
  if (v.startsWith('SF=')) {
    return (
      <div className="flex size-14 shrink-0 flex-col items-center justify-center rounded-xl bg-accent-500/12 text-accent-400 ring-1 ring-accent-500/25" title={`SF Symbol ${v.slice(3)}`}>
        <Laptop className="size-6" aria-hidden="true" />
        <span className="mt-0.5 max-w-12 truncate text-[9px] font-mono text-ink-muted">{v.slice(3)}</span>
      </div>
    )
  }
  if (/^https?:\/\//.test(v) && !broken) return <img src={v} alt="" className="size-14 shrink-0 rounded-xl object-contain" onError={() => setBroken(true)} />
  return (
    <div className="flex size-14 shrink-0 items-center justify-center rounded-xl bg-line/50 text-ink-faint" title={v ? 'Shown from the Mac at run time' : 'Default icon'}>
      <Laptop className="size-6" aria-hidden="true" />
    </div>
  )
}

function BannerPreview({ url }: { url: string }) {
  const [broken, setBroken] = useState(false)
  useEffect(() => setBroken(false), [url])
  const v = url.trim()
  if (!v) return null
  if (/^https?:\/\//.test(v) && !broken) return <img src={v} alt="" className="h-20 w-full object-cover" onError={() => setBroken(true)} />
  return <div className="flex h-12 items-center justify-center bg-line/40 text-[11px] text-ink-faint">Banner image</div>
}

/** A simple mock of the swiftDialog welcome screen, so admins can see what users will get. */
function DialogPreview({ c, fieldKeys }: { c: OnboardingConfig; fieldKeys?: Record<string, string> }) {
  const labels = new Map(orderedFieldKeys(fieldKeys))
  const fields = c.fields ?? []
  return (
    <div className={cn('rounded-xl border border-line-strong/60 bg-surface-raised shadow-xl ring-1 ring-black/20 overflow-hidden', c.blur_screen && 'outline outline-4 outline-line/40')} aria-label="Dialog preview">
      <BannerPreview url={c.banner} />
      <div className="flex gap-4 px-5 pt-5">
        <IconPreview icon={c.icon} />
        <div className="min-w-0 flex-1">
          <p className="text-[15px] font-semibold leading-5 text-ink">{c.title.trim() || 'Setting up your Mac'}</p>
          <p className="mt-1.5 whitespace-pre-wrap break-words text-xs leading-relaxed text-ink-muted">{c.message.trim() || 'Your message appears here.'}</p>
        </div>
      </div>
      <div className="flex flex-col gap-2.5 px-5 pb-4 pt-4">
        {fields.length === 0 && <p className="text-[11px] text-ink-faint">No fields are collected.</p>}
        {fields.map((fld, i) => {
          const label = fld.label.trim() || labels.get(fld.key) || fld.key
          const opts = (fld.options ?? []).filter(Boolean)
          return (
            <div key={`${fld.key}-${i}`} className="grid grid-cols-[minmax(0,7rem)_1fr] items-center gap-3">
              <span className="truncate text-right text-xs text-ink">
                {label}
                {fld.required && <span className="ml-0.5 text-critical">*</span>}
              </span>
              {opts.length > 0 ? (
                <span className="flex h-7 items-center justify-between rounded-md border border-line-strong/70 bg-surface-sunken px-2 text-xs text-ink-faint">
                  <span className="truncate">{opts[0]}</span>
                  <ChevronDown className="size-3.5" aria-hidden="true" />
                </span>
              ) : (
                <span className="flex h-7 items-center truncate rounded-md border border-line-strong/70 bg-surface-sunken px-2 text-xs text-ink-faint">{fld.placeholder || ''}</span>
              )}
            </div>
          )
        })}
      </div>
      <div className="flex justify-end border-t border-line bg-surface-sunken/40 px-5 py-3">
        <span className="inline-flex h-7 items-center rounded-md bg-accent-500 px-3 text-xs font-semibold text-[#03252d]">Continue</span>
      </div>
    </div>
  )
}

// ---- fields ----

function ReorderButtons({ index, count, onMove, onRemove }: { index: number; count: number; onMove: (dir: -1 | 1) => void; onRemove: () => void }) {
  return (
    <span className="flex shrink-0 items-center gap-1">
      <IconButton label="Move up" size="xs" variant="ghost" onClick={() => onMove(-1)} disabled={index === 0}>
        <ArrowUp />
      </IconButton>
      <IconButton label="Move down" size="xs" variant="ghost" onClick={() => onMove(1)} disabled={index === count - 1}>
        <ArrowDown />
      </IconButton>
      <IconButton label="Remove" size="xs" variant="ghost" onClick={onRemove} className="text-ink-muted hover:text-critical">
        <X />
      </IconButton>
    </span>
  )
}

function moveItem<T>(list: T[], from: number, dir: -1 | 1): T[] {
  const to = from + dir
  if (to < 0 || to >= list.length) return list
  const next = [...list]
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item as T)
  return next
}

function FieldsCard({ c, setC, readOnly, fieldKeys }: ConfigProps & { fieldKeys?: Record<string, string> }) {
  const fields = c.fields ?? []
  const all = orderedFieldKeys(fieldKeys)
  const used = new Set(fields.map((x) => x.key))
  const remaining = all.filter(([k]) => !used.has(k))
  const labelOf = new Map(all)
  const update = (i: number, patch: Partial<OnboardingField>) => setC({ fields: fields.map((x, j) => (j === i ? { ...x, ...patch } : x)) })
  return (
    <Card>
      <CardHeader
        title="Fields"
        description="Details the user types on the welcome screen; each one is written to the Mac's inventory record. swiftDialog uses commas as separators, so labels, placeholders and options cannot contain them."
        actions={
          !readOnly && (
            <Select
              value=""
              onChange={(e) => {
                const k = e.target.value
                if (k) setC({ fields: [...fields, newField(k, labelOf.get(k) ?? k)] })
              }}
              className="h-8 text-xs"
              aria-label="Add a field"
              disabled={remaining.length === 0}
              options={[{ value: '', label: remaining.length ? 'Add field…' : 'All fields added' }, ...remaining.map(([k, l]) => ({ value: k, label: l }))]}
            />
          )
        }
      />
      <CardBody className="flex flex-col gap-3">
        {fields.length === 0 && <p className="text-xs text-ink-muted">No fields yet. {readOnly ? '' : 'Pick one from “Add field…” to collect it on the welcome screen.'}</p>}
        {fields.map((fld, i) => (
          <div key={`${fld.key}-${i}`} className="rounded-lg border border-line p-3.5">
            <div className="mb-3 flex items-center gap-2">
              <Badge tone="accent" className="font-mono">
                {fld.key}
              </Badge>
              <span className="min-w-0 flex-1 truncate text-sm font-medium text-ink">{fld.label.trim() || labelOf.get(fld.key) || fld.key}</span>
              {fld.required && <Badge>Required</Badge>}
              {(fld.options ?? []).length > 0 && <Badge>Picker</Badge>}
              {!readOnly && <ReorderButtons index={i} count={fields.length} onMove={(d) => setC({ fields: moveItem(fields, i, d) })} onRemove={() => setC({ fields: fields.filter((_, j) => j !== i) })} />}
            </div>
            <FormGrid cols={2}>
              <Field label="Label" hint={fld.key === 'computer_name' ? 'A collected computer name overrides the name template below.' : undefined}>
                <Input value={fld.label} onChange={(e) => update(i, { label: e.target.value })} placeholder={labelOf.get(fld.key)} disabled={readOnly} />
              </Field>
              <Field label="Placeholder">
                <Input value={fld.placeholder} onChange={(e) => update(i, { placeholder: e.target.value })} placeholder="Shown inside the empty box" disabled={readOnly} />
              </Field>
              <Field label="Options" hint="Press Enter after each one. With options set, the field becomes a picker instead of a text box.">
                <ChipInput value={fld.options ?? []} onChange={(v) => update(i, { options: v.map((o) => o.replace(/,/g, ' ').trim()).filter(Boolean) })} split={/[,\n]+/} placeholder="Add an option and press Enter" disabled={readOnly} />
              </Field>
              <div className="flex flex-col gap-4">
                <Checkbox checked={fld.required} onChange={(e) => update(i, { required: e.target.checked })} label="Required" description="The user cannot continue until it is filled in." disabled={readOnly} />
                <FormGrid cols={2}>
                  <Field label="Pattern" hint="Optional regular expression the value must match.">
                    <Input value={fld.regex} onChange={(e) => update(i, { regex: e.target.value })} placeholder="^[A-Z]-\d{4}$" disabled={readOnly} mono />
                  </Field>
                  <Field label="Pattern error" hint="Shown when the value does not match.">
                    <Input value={fld.regex_error} onChange={(e) => update(i, { regex_error: e.target.value })} placeholder="Use the format A-1234" disabled={readOnly} />
                  </Field>
                </FormGrid>
              </div>
            </FormGrid>
          </div>
        ))}
      </CardBody>
    </Card>
  )
}

// ---- computer name ----

const nameTokens: { token: string; label: string }[] = [
  { token: '{serial}', label: 'Serial number' },
  { token: '{asset_tag}', label: 'Asset tag' },
  { token: '{user}', label: 'Username' },
  { token: '{model}', label: 'Model' },
]

function NameTemplateCard({ c, setC, readOnly }: ConfigProps) {
  const ref = useRef<HTMLInputElement>(null)
  const tpl = c.computer_name_template
  const hasNameField = (c.fields ?? []).some((x) => x.key === 'computer_name')
  const insert = (token: string) => {
    const el = ref.current
    if (!el) {
      setC({ computer_name_template: tpl + token })
      return
    }
    const s = el.selectionStart ?? tpl.length
    const e = el.selectionEnd ?? s
    setC({ computer_name_template: tpl.slice(0, s) + token + tpl.slice(e) })
    requestAnimationFrame(() => {
      el.focus()
      el.setSelectionRange(s + token.length, s + token.length)
    })
  }
  const preview = tpl.trim() ? renderNameTemplate(tpl) : ''
  return (
    <Card>
      <CardHeader title="Computer name" description="Optionally rename the Mac once the user has filled in the form. Leave blank to keep its current name." />
      <CardBody className="flex flex-col gap-4">
        <Field label="Template" hint="Characters other than letters, digits and hyphens become hyphens.">
          <Input ref={ref} value={tpl} onChange={(e) => setC({ computer_name_template: e.target.value })} placeholder="MAC-{serial}" disabled={readOnly} mono />
        </Field>
        {!readOnly && (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="mr-1 text-xs text-ink-muted">Insert</span>
            {nameTokens.map((t) => (
              <button
                key={t.token}
                type="button"
                onClick={() => insert(t.token)}
                className="inline-flex items-center gap-1.5 rounded-full border border-line bg-surface-sunken px-2.5 py-0.5 text-xs text-ink hover:border-line-strong focus-ring"
                title={t.label}
              >
                <span className="font-mono">{t.token}</span>
              </button>
            ))}
          </div>
        )}
        <p className="text-xs text-ink-muted">
          Preview with sample values ({Object.entries(sampleNameValues).map(([k, v]) => `${k} = ${v}`).join(', ')}): {preview ? <span className="font-mono text-ink">{preview}</span> : <span className="text-ink-faint">no rename</span>}
        </p>
        {hasNameField && <Alert tone="info">This workflow collects a computer name from the user, which overrides the template whenever they fill it in.</Alert>}
      </CardBody>
    </Card>
  )
}

// ---- steps ----

function AppPicker({ value, onChange, disabled }: { value: string; onChange: (id: string) => void; disabled?: boolean }) {
  const apps = useApps()
  const items = apps.data?.items ?? []
  return (
    <Select value={value} onChange={(e) => onChange(e.target.value)} disabled={disabled}>
      <option value="">{apps.isPending ? 'Loading apps…' : items.length ? 'Select an app…' : 'No apps in the Mac apps catalog'}</option>
      {items.map((a) => (
        <option key={a.id} value={a.id}>
          {a.name}
          {a.label && a.label !== a.name ? ` (${a.label})` : ''}
        </option>
      ))}
    </Select>
  )
}

function StepsCard({ c, setC, readOnly }: ConfigProps) {
  const steps = c.steps ?? []
  const update = (i: number, patch: Partial<OnboardingStep>) => setC({ steps: steps.map((s, j) => (j === i ? { ...s, ...patch } : s)) })
  const add = (type: OnboardingStepType) => setC({ steps: [...steps, newStep(type)] })
  const types = Object.keys(stepTypeMeta) as OnboardingStepType[]
  return (
    <Card>
      <CardHeader
        title="Steps"
        description="Run in order after the user continues, with a live progress list in the dialog. A failing step stops the run unless it may continue on error."
        actions={
          !readOnly && (
            <Dropdown
              align="right"
              trigger={
                <Button size="sm" icon={<Plus />} iconRight={<ChevronDown />}>
                  Add step
                </Button>
              }
              items={types.map((t) => {
                const Icon = stepTypeMeta[t].icon
                return { label: stepTypeMeta[t].label, icon: <Icon />, onSelect: () => add(t) }
              })}
            />
          )
        }
      />
      <CardBody className="flex flex-col gap-3">
        {steps.length === 0 && <p className="text-xs text-ink-muted">No steps yet. {readOnly ? '' : 'Add apps, policies, scripts, an inventory update or a pause.'}</p>}
        {steps.map((s, i) => {
          const meta = stepTypeMeta[s.type] ?? stepTypeMeta.recon
          const Icon = meta.icon
          return (
            <div key={i} className="rounded-lg border border-line p-3.5">
              <div className="mb-3 flex items-center gap-2">
                <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-line/50 text-ink-muted">
                  <Icon className="size-4" aria-hidden="true" />
                </span>
                <span className="text-xs text-ink-muted">{i + 1}.</span>
                <span className="min-w-0 flex-1 truncate text-sm font-medium text-ink">
                  {s.title.trim() || <span className="text-ink-faint">Untitled step</span>}
                  <span className="ml-2 text-xs font-normal text-ink-muted">{meta.label}</span>
                </span>
                {s.continue_on_error && <Badge>Continues on error</Badge>}
                {!readOnly && <ReorderButtons index={i} count={steps.length} onMove={(d) => setC({ steps: moveItem(steps, i, d) })} onRemove={() => setC({ steps: steps.filter((_, j) => j !== i) })} />}
              </div>
              <FormGrid cols={2}>
                <Field label="Title" required hint="Shown in the progress list. No commas.">
                  <Input value={s.title} onChange={(e) => update(i, { title: e.target.value })} placeholder={meta.label} disabled={readOnly} />
                </Field>
                {s.type === 'app' && (
                  <Field label="App" required>
                    <AppPicker value={s.app_id ?? ''} onChange={(v) => update(i, { app_id: v })} disabled={readOnly} />
                  </Field>
                )}
                {s.type === 'policy' && (
                  <Field label="Policy" required>
                    <PolicyPicker value={s.policy_id ?? ''} onChange={(v) => update(i, { policy_id: v })} disabled={readOnly} />
                  </Field>
                )}
                {s.type === 'script' && (
                  <Field label="Script" required>
                    <ScriptPicker value={s.script_id ?? ''} onChange={(v) => update(i, { script_id: v })} disabled={readOnly} />
                  </Field>
                )}
                {s.type === 'wait' && (
                  <Field label="Seconds" required hint="1–3600.">
                    <Input type="number" min={1} max={3600} step={1} value={s.seconds ?? ''} onChange={(e) => update(i, { seconds: Math.floor(Number(e.target.value) || 0) })} disabled={readOnly} className="max-w-32" />
                  </Field>
                )}
                {s.type === 'recon' && (
                  <Field label="What it does">
                    <p className="pt-2 text-xs text-ink-muted">{meta.description}</p>
                  </Field>
                )}
                <Field label="Subtitle" hint="Optional second line under the title.">
                  <Input value={s.subtitle} onChange={(e) => update(i, { subtitle: e.target.value })} placeholder="e.g. This can take a few minutes" disabled={readOnly} />
                </Field>
                <Field label="Icon" hint="Optional: path, URL or SF=symbol.name. Apps use their own icon when blank.">
                  <Input value={s.icon} onChange={(e) => update(i, { icon: e.target.value })} placeholder="SF=arrow.down.circle" disabled={readOnly} mono />
                </Field>
              </FormGrid>
              <Checkbox className="mt-4" checked={s.continue_on_error} onChange={(e) => update(i, { continue_on_error: e.target.checked })} label="Continue on error" description="Mark the step failed and move on instead of stopping the run." disabled={readOnly} />
            </div>
          )
        })}
      </CardBody>
    </Card>
  )
}

// ---- finish ----

const finalActions: { value: OnboardingFinalAction; label: string; icon: ReactNode; text: string }[] = [
  { value: 'none', label: 'Nothing', icon: <Ban />, text: 'Close the dialog and leave the user at their desktop.' },
  { value: 'restart', label: 'Restart', icon: <RotateCcw />, text: 'Restart the Mac after the final message.' },
  { value: 'logout', label: 'Log out', icon: <LogOut />, text: 'Log the user out so a fresh session picks up every change.' },
  { value: 'shutdown', label: 'Shut down', icon: <Power />, text: 'Shut the Mac down, for example when it is being handed over later.' },
]

function FinishCard({ c, setC, readOnly }: ConfigProps) {
  const current = c.final_action || 'none'
  return (
    <Card>
      <CardHeader title="Finish" description="What happens once every step has run." />
      <CardBody className="flex flex-col gap-5">
        <div>
          <p className="mb-1.5 text-sm font-medium text-ink">Final action</p>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" role="radiogroup" aria-label="Final action">
            {finalActions.map((a) => {
              const activeOpt = current === a.value
              return (
                <button
                  key={a.value}
                  type="button"
                  role="radio"
                  aria-checked={activeOpt}
                  disabled={readOnly}
                  onClick={() => setC({ final_action: a.value })}
                  className={cn(
                    'flex items-start gap-3 rounded-lg border p-3.5 text-left transition-colors focus-ring disabled:cursor-not-allowed',
                    activeOpt ? 'border-accent-500/60 bg-accent-500/8' : 'border-line hover:border-line-strong',
                  )}
                >
                  <span className={cn('mt-0.5 [&>svg]:size-4', activeOpt ? 'text-accent-400' : 'text-ink-faint')}>{a.icon}</span>
                  <span className="min-w-0">
                    <span className="flex items-center gap-2">
                      <span className="text-sm font-medium text-ink">{a.label}</span>
                      {activeOpt && <Badge tone="accent">Selected</Badge>}
                    </span>
                    <span className="mt-1 block text-xs text-ink-muted">{a.text}</span>
                  </span>
                </button>
              )
            })}
          </div>
        </div>
        <Field label="Final message" hint="Shown when the steps are done, before the final action. Markdown is allowed.">
          <Textarea value={c.final_message} onChange={(e) => setC({ final_message: e.target.value })} rows={3} placeholder="All set! Your Mac will restart in a moment." disabled={readOnly} />
        </Field>
      </CardBody>
    </Card>
  )
}
