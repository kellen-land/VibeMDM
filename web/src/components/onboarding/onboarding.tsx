import { useEffect, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { AppWindow, CheckCircle2, ChevronDown, ChevronRight, Clock, ClipboardList, Laptop, ListChecks, LoaderCircle, MinusCircle, RefreshCw, Terminal, Timer, XCircle, type LucideIcon } from 'lucide-react'
import type { OnboardingConfig, OnboardingField, OnboardingRun, OnboardingRunResult, OnboardingStep, OnboardingStepType, OnboardingWorkflow } from '../../api/types'
import { api, errorMessage } from '../../api/client'
import { qk, useInvalidate, useOnboardingWorkflows } from '../../api/queries'
import { emptyScope, normalizeScope } from '../../api/types'
import { Badge, type BadgeTone } from '../ui/Badge'
import { Button } from '../ui/Button'
import { Checkbox } from '../ui/Checkbox'
import { Field } from '../ui/Field'
import { Select } from '../ui/Input'
import { Modal } from '../ui/Modal'
import { Spinner } from '../ui/Spinner'
import { EmptyState } from '../ui/EmptyState'
import { DescriptionList } from '../ui/DescriptionList'
import { CodeBlock } from '../ui/CopyButton'
import { RelativeTime } from '../ui/RelativeTime'
import { useToast } from '../ui/Toast'
import { ComputerPicker } from '../pickers/ComputerPicker'
import { formatMs, titleCase } from '../../lib/format'
import { cn } from '../../lib/cn'

// ---- shapes & defaults ----

/** Display order for the collectable fields; the server's map has no order. */
export const fieldKeyOrder = ['computer_name', 'asset_tag', 'assigned_user', 'assigned_real_name', 'assigned_email', 'assigned_position', 'department', 'building', 'room']

/** Fallback labels, used until the list endpoint answers. */
export const fallbackFieldKeys: Record<string, string> = {
  computer_name: 'Computer name',
  asset_tag: 'Asset tag',
  building: 'Building',
  department: 'Department',
  room: 'Room',
  assigned_user: 'Username',
  assigned_real_name: 'Full name',
  assigned_email: 'Email',
  assigned_position: 'Position',
}

export function orderedFieldKeys(keys: Record<string, string> | undefined): [string, string][] {
  const src = keys && Object.keys(keys).length ? keys : fallbackFieldKeys
  const known = fieldKeyOrder.filter((k) => k in src).map((k) => [k, src[k]] as [string, string])
  const rest = Object.keys(src)
    .filter((k) => !fieldKeyOrder.includes(k))
    .sort()
    .map((k) => [k, src[k]] as [string, string])
  return [...known, ...rest]
}

export const emptyConfig = (): OnboardingConfig => ({
  title: 'Setting up your Mac',
  message: '',
  icon: '',
  banner: '',
  blur_screen: false,
  fields: [],
  computer_name_template: '',
  steps: [],
  final_action: 'none',
  final_message: '',
})

export const emptyWorkflow = (): OnboardingWorkflow => ({
  id: '',
  name: '',
  enabled: true,
  priority: 100,
  scope: emptyScope(),
  config: emptyConfig(),
  created_at: '',
  updated_at: '',
})

/** Fill in missing arrays so partial server payloads are safe to edit and send back. */
export function normalizeWorkflow(w: Partial<OnboardingWorkflow> | null | undefined): OnboardingWorkflow {
  const base = emptyWorkflow()
  const c = { ...emptyConfig(), ...(w?.config ?? {}) }
  return {
    ...base,
    ...(w ?? {}),
    priority: typeof w?.priority === 'number' ? w.priority : 100,
    scope: normalizeScope(w?.scope),
    config: {
      ...c,
      fields: (c.fields ?? []).map((f) => ({ ...f, options: f.options ?? [] })),
      steps: (c.steps ?? []).map((s) => ({ ...s })),
      final_action: c.final_action || 'none',
    },
  }
}

export const newField = (key: string, label: string): OnboardingField => ({ key, label, required: false, placeholder: '', options: [], regex: '', regex_error: '' })

export const newStep = (type: OnboardingStepType): OnboardingStep => ({
  type,
  title: type === 'recon' ? 'Updating inventory' : type === 'wait' ? 'Waiting' : '',
  subtitle: '',
  icon: '',
  continue_on_error: type === 'recon' || type === 'wait',
  ...(type === 'wait' ? { seconds: 10 } : {}),
})

export const stepTypeMeta: Record<OnboardingStepType, { label: string; icon: LucideIcon; description: string }> = {
  app: { label: 'Install app', icon: AppWindow, description: 'Install an app from the Mac Apps catalog with Installomator.' },
  policy: { label: 'Run policy', icon: ListChecks, description: 'Run an existing policy now, ignoring its triggers and frequency.' },
  script: { label: 'Run script', icon: Terminal, description: 'Run a script from the library as root.' },
  recon: { label: 'Update inventory', icon: RefreshCw, description: 'Submit a fresh inventory report, including the details the user entered.' },
  wait: { label: 'Wait', icon: Timer, description: 'Pause for a number of seconds, for example to let a login finish.' },
}

/** Client-side checks mirroring onboarding.Validate, so most mistakes never round-trip. */
export function validateWorkflow(w: OnboardingWorkflow): string[] {
  const e: string[] = []
  if (!w.name.trim()) e.push('Name is required')
  if (!Number.isInteger(w.priority) || w.priority < 0) e.push('Priority must be a whole number (0 or more)')
  const c = w.config
  const fields = c.fields ?? []
  const steps = c.steps ?? []
  const seen = new Set<string>()
  fields.forEach((f, i) => {
    const name = f.label.trim() || f.key || `field ${i + 1}`
    if (!f.key) e.push(`Field ${i + 1}: choose what it collects`)
    if (seen.has(f.key)) e.push(`Field "${name}" appears twice`)
    seen.add(f.key)
    if ([f.label, f.placeholder, f.regex_error, ...(f.options ?? [])].some((s) => s.includes(','))) e.push(`Field "${name}": commas are not allowed in labels, placeholders or options`)
    if (f.regex.trim()) {
      try {
        new RegExp(f.regex)
      } catch {
        e.push(`Field "${name}": the pattern is not a valid regular expression`)
      }
    }
  })
  if (fields.length === 0 && steps.length === 0) e.push('Add at least one field or step')
  steps.forEach((s, i) => {
    const n = `Step ${i + 1}`
    if (!s.title.trim()) e.push(`${n}: a title is required`)
    else if (s.title.includes(',')) e.push(`${n}: commas are not allowed in the title`)
    if (s.type === 'app' && !s.app_id) e.push(`${n}: choose an app from the Mac Apps catalog`)
    if (s.type === 'policy' && !s.policy_id) e.push(`${n}: choose a policy`)
    if (s.type === 'script' && !s.script_id) e.push(`${n}: choose a script`)
    if (s.type === 'wait' && !(Number.isInteger(s.seconds) && (s.seconds ?? 0) >= 1 && (s.seconds ?? 0) <= 3600)) e.push(`${n}: wait between 1 and 3600 seconds`)
  })
  return e
}

/** Sample values used by the computer-name preview. */
export const sampleNameValues: Record<string, string> = { serial: 'C02XK1ABJGH5', asset_tag: 'A-1042', user: 'jappleseed', model: 'MacBookPro' }

/** Mirrors the agent's rendering: substitute tokens, then keep only hostname-safe characters. */
export function renderNameTemplate(tpl: string, values: Record<string, string> = sampleNameValues): string {
  let out = tpl
  for (const [k, v] of Object.entries(values)) out = out.split(`{${k}}`).join(v)
  return out.replace(/[^A-Za-z0-9-]+/g, '-').replace(/^-+|-+$/g, '')
}

// ---- status ----

const runStatusMetas: Record<string, { label: string; icon: LucideIcon; tone: BadgeTone; spin?: boolean }> = {
  queued: { label: 'Queued', icon: Clock, tone: 'info' },
  running: { label: 'Running', icon: LoaderCircle, tone: 'info', spin: true },
  completed: { label: 'Completed', icon: CheckCircle2, tone: 'good' },
  failed: { label: 'Failed', icon: XCircle, tone: 'critical' },
}

export function RunStatusBadge({ status }: { status: string }) {
  const m = runStatusMetas[status] ?? { label: titleCase(status || 'unknown'), icon: Clock, tone: 'neutral' as BadgeTone }
  const Icon = m.icon
  return (
    <Badge tone={m.tone} icon={<Icon className={m.spin ? 'animate-spin' : undefined} />}>
      {m.label}
    </Badge>
  )
}

const stepStatusMetas: Record<string, { label: string; icon: LucideIcon; ink: string }> = {
  success: { label: 'Succeeded', icon: CheckCircle2, ink: 'text-good' },
  failed: { label: 'Failed', icon: XCircle, ink: 'text-critical' },
  skipped: { label: 'Skipped', icon: MinusCircle, ink: 'text-ink-faint' },
}

export function stepCounts(run: OnboardingRun): { ok: number; failed: number; total: number } {
  const steps = run.steps ?? []
  return { ok: steps.filter((s) => s.status === 'success').length, failed: steps.filter((s) => s.status === 'failed').length, total: steps.length }
}

export function StepSummary({ run }: { run: OnboardingRun }) {
  const { ok, failed, total } = stepCounts(run)
  if (total === 0) return <span className="text-ink-faint">—</span>
  return (
    <span className="inline-flex items-center gap-2 text-xs tabular-nums text-ink-muted">
      <span className="inline-flex items-center gap-1" title={`${ok} of ${total} steps succeeded`}>
        <CheckCircle2 className={cn('size-3.5', ok ? 'text-good' : 'text-ink-faint')} aria-hidden="true" />
        {ok}/{total}
        <span className="sr-only">steps succeeded</span>
      </span>
      {failed > 0 && (
        <span className="inline-flex items-center gap-1" title={`${failed} failed`}>
          <XCircle className="size-3.5 text-critical" aria-hidden="true" />
          {failed}
          <span className="sr-only">failed</span>
        </span>
      )}
    </span>
  )
}

// ---- runs table with expandable details ----

export function RunDetail({ run, fieldKeys }: { run: OnboardingRun; fieldKeys?: Record<string, string> }) {
  const entry = Object.entries(run.user_entry ?? {}).filter(([, v]) => v !== '')
  const steps = run.steps ?? []
  const labels = fieldKeys && Object.keys(fieldKeys).length ? fieldKeys : fallbackFieldKeys
  return (
    <div className="flex flex-col gap-5">
      <div>
        <p className="micro-label mb-2">User entry</p>
        {entry.length === 0 ? (
          <p className="text-xs text-ink-faint">Nothing was collected{run.status === 'queued' || run.status === 'running' ? ' yet' : ''}.</p>
        ) : (
          <DescriptionList cols={3} items={entry.map(([k, v]) => ({ label: labels[k] ?? titleCase(k), value: v }))} />
        )}
      </div>
      <div>
        <p className="micro-label mb-2">Steps</p>
        {steps.length === 0 ? (
          <p className="text-xs text-ink-faint">{run.status === 'queued' ? 'The Mac has not started this run.' : run.status === 'running' ? 'Steps report as they finish.' : 'No steps were recorded.'}</p>
        ) : (
          <ol className="divide-y divide-line rounded-lg border border-line">
            {steps.map((s, i) => {
              const m = stepStatusMetas[s.status] ?? { label: titleCase(s.status || 'unknown'), icon: MinusCircle, ink: 'text-ink-faint' }
              const Icon = m.icon
              const type = stepTypeMeta[s.type as OnboardingStepType]
              return (
                <li key={i} className="flex items-start gap-3 px-3 py-2 text-sm">
                  <Icon className={cn('mt-0.5 size-4 shrink-0', m.ink)} aria-hidden="true" />
                  <div className="min-w-0 flex-1">
                    <p className="text-ink">
                      {s.title || type?.label || titleCase(s.type)}
                      <span className="ml-2 text-xs text-ink-muted">
                        {m.label}
                        {type ? ` · ${type.label}` : ''}
                      </span>
                    </p>
                    {s.detail && <p className="mt-0.5 whitespace-pre-wrap break-words font-mono text-xs text-ink-muted">{s.detail}</p>}
                  </div>
                  <span className="shrink-0 text-xs tabular-nums text-ink-muted">{s.duration_ms ? formatMs(s.duration_ms) : ''}</span>
                </li>
              )
            })}
          </ol>
        )}
      </div>
      {run.log && (
        <div>
          <p className="micro-label mb-2">Log</p>
          <CodeBlock text={run.log} maxHeight="20rem" />
        </div>
      )}
    </div>
  )
}

export function RunsTable({
  runs,
  loading,
  error,
  showComputer = true,
  showWorkflow = false,
  fieldKeys,
  emptyTitle = 'No runs yet',
  emptyDescription,
  emptyAction,
}: {
  runs: OnboardingRun[] | undefined
  loading?: boolean
  error?: string | null
  showComputer?: boolean
  showWorkflow?: boolean
  fieldKeys?: Record<string, string>
  emptyTitle?: ReactNode
  emptyDescription?: ReactNode
  emptyAction?: ReactNode
}) {
  const [open, setOpen] = useState<Set<string>>(() => new Set())
  const toggle = (id: string) =>
    setOpen((cur) => {
      const n = new Set(cur)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  const cols = 5 + (showComputer ? 1 : 0) + (showWorkflow ? 1 : 0)
  const empty = !loading && !error && runs && runs.length === 0
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-line text-left">
            <th className="w-8 px-3 py-2 first:pl-4">
              <span className="sr-only">Details</span>
            </th>
            {showComputer && <th className="micro-label whitespace-nowrap px-3 py-2">Computer</th>}
            {showWorkflow && <th className="micro-label whitespace-nowrap px-3 py-2">Workflow</th>}
            <th className="micro-label whitespace-nowrap px-3 py-2">Status</th>
            <th className="micro-label whitespace-nowrap px-3 py-2">Steps</th>
            <th className="micro-label whitespace-nowrap px-3 py-2">Started</th>
            <th className="micro-label whitespace-nowrap px-3 py-2 last:pr-4">Completed</th>
          </tr>
        </thead>
        <tbody>
          {loading && (!runs || runs.length === 0) && (
            <tr>
              <td colSpan={cols} className="py-12 text-center text-ink-muted">
                <span className="inline-flex items-center gap-2">
                  <Spinner className="size-4" /> Loading…
                </span>
              </td>
            </tr>
          )}
          {error && (
            <tr>
              <td colSpan={cols} className="py-10 text-center text-sm text-critical">
                {error}
              </td>
            </tr>
          )}
          {empty && (
            <tr>
              <td colSpan={cols}>
                <EmptyState title={emptyTitle} description={emptyDescription} action={emptyAction} compact icon={<ClipboardList />} />
              </td>
            </tr>
          )}
          {runs?.map((r) => {
            const isOpen = open.has(r.id)
            return [
              <tr key={r.id} onClick={() => toggle(r.id)} className={cn('cursor-pointer border-b border-line transition-colors hover:bg-surface-sunken/60', isOpen && 'bg-surface-sunken/40', loading && 'opacity-60')}>
                <td className="px-3 py-2 pl-4 align-middle">
                  <button type="button" className="inline-flex text-ink-faint hover:text-ink focus-ring rounded" aria-expanded={isOpen} aria-label={isOpen ? 'Hide details' : 'Show details'} onClick={(e) => {
                      e.stopPropagation()
                      toggle(r.id)
                    }}
                  >
                    {isOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                  </button>
                </td>
                {showComputer && (
                  <td className="px-3 py-2 align-middle text-[13px]">
                    <Link to={`/computers/${r.computer_id}/onboarding`} onClick={(e) => e.stopPropagation()} className="inline-flex items-center gap-1.5 font-medium text-ink hover:underline">
                      <Laptop className="size-3.5 text-ink-faint" aria-hidden="true" />
                      {r.computer_name || r.computer_id.slice(0, 8)}
                    </Link>
                  </td>
                )}
                {showWorkflow && (
                  <td className="px-3 py-2 align-middle text-[13px]">
                    <Link to={`/onboarding/${r.workflow_id}`} onClick={(e) => e.stopPropagation()} className="font-medium text-ink hover:underline">
                      {r.workflow_name || r.workflow_id.slice(0, 8)}
                    </Link>
                  </td>
                )}
                <td className="px-3 py-2 align-middle">
                  <RunStatusBadge status={r.status} />
                </td>
                <td className="px-3 py-2 align-middle">
                  <StepSummary run={r} />
                </td>
                <td className="px-3 py-2 align-middle text-[13px]">
                  <RelativeTime value={r.started_at} className="text-ink-muted" emptyLabel="not started" />
                </td>
                <td className="px-3 py-2 pr-4 align-middle text-[13px]">
                  <RelativeTime value={r.completed_at} className="text-ink-muted" emptyLabel="—" />
                </td>
              </tr>,
              isOpen ? (
                <tr key={`${r.id}-detail`} className="border-b border-line bg-surface-sunken/30">
                  <td colSpan={cols} className="px-4 py-4 sm:pl-12">
                    <RunDetail run={r} fieldKeys={fieldKeys} />
                  </td>
                </tr>
              ) : null,
            ]
          })}
        </tbody>
      </table>
    </div>
  )
}

// ---- running a workflow ----

/** POST a run and report the outcome with the same voice as compliance scans. */
export function useRunOnboarding() {
  const toast = useToast()
  const invalidate = useInvalidate()
  return async (workflowId: string, computerIds: string[], force: boolean, opts: { computerName?: string } = {}): Promise<boolean> => {
    try {
      const r = await api.post<OnboardingRunResult>(`/api/v1/onboarding/workflows/${workflowId}/run`, { computer_ids: computerIds, force })
      const skipped = Object.entries(r.skipped ?? {})
      const target = opts.computerName ?? `${r.queued} ${r.queued === 1 ? 'Mac' : 'Macs'}`
      if (r.queued === 0 && skipped.length) toast.warning('Onboarding not queued', skipped.map(([n, why]) => `${n}: ${why}`).slice(0, 3).join(' · '))
      else if (r.queued === 0) toast.info('Nothing queued', 'No computers were selected.')
      else if (skipped.length) toast.warning(`Onboarding queued for ${target}`, `${skipped.length} skipped. ${skipped[0][0]}: ${skipped[0][1]}`)
      else toast.success(`Onboarding queued for ${target}`, 'The dialog opens on each Mac when the agent next checks in with a user logged in.')
      await invalidate(qk.onboarding, ...computerIds.map((id) => qk.computerTasks(id)))
      return r.queued > 0
    } catch (e) {
      toast.error('Onboarding failed to queue', errorMessage(e))
      return false
    }
  }
}

function ForceCheckbox({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return <Checkbox checked={checked} onChange={(e) => onChange(e.target.checked)} label="Run again on Macs that already ran this workflow" description="Each Mac runs a workflow once. Force replaces its previous run and shows the dialog again at the next check-in." />
}

/** Workflow page: pick computers, optionally force. */
export function RunOnComputersDialog({ open, onClose, workflow }: { open: boolean; onClose: () => void; workflow: OnboardingWorkflow }) {
  const run = useRunOnboarding()
  const [ids, setIds] = useState<string[]>([])
  const [force, setForce] = useState(false)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (!open) {
      setIds([])
      setForce(false)
    }
  }, [open])
  const submit = async () => {
    setBusy(true)
    const ok = await run(workflow.id, ids, force)
    setBusy(false)
    if (ok) onClose()
  }
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Run “${workflow.name}” on computers`}
      description="Queues the onboarding dialog on the chosen Macs regardless of scope. Each Mac needs the VibeMDM agent and a logged-in user."
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" onClick={submit} loading={busy} disabled={ids.length === 0}>
            Run on {ids.length || ''} {ids.length === 1 ? 'Mac' : 'Macs'}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {!workflow.enabled && <p className="text-xs text-ink-muted">This workflow is disabled; a manual run still works.</p>}
        <Field label="Computers">
          <ComputerPicker value={ids} onChange={setIds} />
        </Field>
        <ForceCheckbox checked={force} onChange={setForce} />
      </div>
    </Modal>
  )
}

/** Computer page: pick a workflow, optionally force. */
export function RunWorkflowDialog({ open, onClose, computerId, computerName }: { open: boolean; onClose: () => void; computerId: string; computerName: string }) {
  const run = useRunOnboarding()
  const wfs = useOnboardingWorkflows()
  const [wf, setWf] = useState('')
  const [force, setForce] = useState(false)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (!open) {
      setWf('')
      setForce(false)
    }
  }, [open])
  const items = [...(wfs.data?.items ?? [])].sort((a, b) => Number(b.enabled) - Number(a.enabled) || a.priority - b.priority || a.name.localeCompare(b.name))
  const submit = async () => {
    setBusy(true)
    const ok = await run(wf, [computerId], force, { computerName })
    setBusy(false)
    if (ok) onClose()
  }
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Run an onboarding workflow on ${computerName}`}
      description="Shows the workflow's welcome dialog the next time the agent checks in with a user logged in, regardless of the workflow's scope."
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" onClick={submit} loading={busy} disabled={!wf}>
            Run workflow
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Field label="Workflow">
          <Select value={wf} onChange={(e) => setWf(e.target.value)}>
            <option value="">{wfs.isPending ? 'Loading workflows…' : items.length ? 'Select a workflow…' : 'No workflows defined'}</option>
            {items.map((w) => (
              <option key={w.id} value={w.id}>
                {w.name}
                {!w.enabled ? ' (disabled)' : ''}
              </option>
            ))}
          </Select>
        </Field>
        <ForceCheckbox checked={force} onChange={setForce} />
      </div>
    </Modal>
  )
}
