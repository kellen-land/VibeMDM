import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ArrowDown, ArrowUp, Eraser, Play, Plus, Save, Terminal, Trash2 } from 'lucide-react'
import { qk, useAppLabels, useInvalidate, usePackages, usePolicy, usePolicyLogs, useScripts } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { emptyInstallomatorOptions, emptyScope, normalizeScope, type Policy, type PolicyApp, type PolicyFrequency, type PolicyPackage, type PolicyScript, type RunResult, type ScriptPriority } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Button, IconButton } from '../components/ui/Button'
import { Field, FormGrid, FormSection } from '../components/ui/Field'
import { Input, Select, Textarea } from '../components/ui/Input'
import { Checkbox, Toggle } from '../components/ui/Checkbox'
import { Tabs } from '../components/ui/Tabs'
import { Alert } from '../components/ui/Alert'
import { Badge } from '../components/ui/Badge'
import { LoadingBlock } from '../components/ui/Spinner'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { Modal } from '../components/ui/Modal'
import { Pagination } from '../components/ui/Pagination'
import { useToast } from '../components/ui/Toast'
import { ScopeEditor } from '../components/pickers/ScopeEditor'
import { ScriptPicker } from '../components/pickers/ScriptPicker'
import { TargetPicker, emptyTargets, type Targets } from '../components/pickers/TargetPicker'
import { ComputerPicker } from '../components/pickers/ComputerPicker'
import { PolicyLogTable } from '../components/activity/PolicyLogTable'
import { frequencyLabels } from './PoliciesPage'
import { formatBytes } from '../lib/format'
import { LabelCombobox, allLabels } from '../components/pickers/LabelCombobox'
import { InstallomatorCommandPreview, InstallomatorOptionsEditor } from '../components/installomator/Installomator'
import { InstallomatorFooter } from './AppsPage'
import { LocalAccountsSection, accountProblem } from './policies/LocalAccountsSection'

type Tab = 'general' | 'scope' | 'scripts' | 'packages' | 'apps' | 'accounts' | 'files' | 'maintenance' | 'restart' | 'interaction' | 'selfservice' | 'logs'

const blank = (): Policy => ({
  id: '',
  name: '',
  enabled: true,
  category: '',
  triggers: { startup: false, login: false, checkin: true, enrollment_complete: false, network_change: false, custom: '' },
  frequency: 'once_per_computer',
  scope: emptyScope(),
  scripts: [],
  packages: [],
  apps: [],
  accounts: [],
  run_command: '',
  update_inventory: false,
  reboot: { enabled: false, message: '', delay_minutes: 0 },
  user_interaction: { start_message: '', complete_message: '' },
  self_service: { enabled: false, display_name: '', description: '', button_text: 'Install' },
  created_at: '',
  updated_at: '',
})

export function PolicyEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const policy = usePolicy(isNew ? undefined : id)
  const [p, setP] = useState<Policy>(blank())
  const [saving, setSaving] = useState(false)
  const [del, setDel] = useState(false)
  const [runOpen, setRunOpen] = useState(false)
  const tab = (sp.get('tab') as Tab) || 'general'
  const setTab = (t: Tab) => setSp(t === 'general' ? {} : { tab: t }, { replace: true })

  useEffect(() => {
    if (policy.data) {
      const { status: _s, ...rest } = policy.data
      void _s
      setP({ ...blank(), ...rest, scripts: rest.scripts ?? [], packages: rest.packages ?? [], accounts: rest.accounts ?? [], apps: (rest.apps ?? []).map((a) => ({ ...a, options: { ...emptyInstallomatorOptions(), ...a.options } })), scope: normalizeScope(rest.scope) })
    }
  }, [policy.data])

  const set = (patch: Partial<Policy>) => setP((cur) => ({ ...cur, ...patch }))

  const save = async () => {
    if (!p.name.trim()) {
      toast.error('Name is required')
      return
    }
    const problem = accountProblem(p.accounts ?? [])
    if (problem) {
      setTab('accounts')
      toast.error('Check Local Accounts', problem)
      return
    }
    setSaving(true)
    try {
      const body = { ...p, name: p.name.trim(), accounts: (p.accounts ?? []).map(({ verify_password: _v, ...a }) => a) }
      const out = isNew ? await api.post<Policy>('/api/v1/policies', body) : await api.put<Policy>(`/api/v1/policies/${id}`, body)
      await invalidate(qk.policies, qk.policy(out.id), qk.dashboard)
      toast.success(isNew ? 'Policy created' : 'Policy saved', 'Scoped agents were asked to check in.')
      if (isNew) navigate(`/policies/${out.id}`, { replace: true })
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/policies/${id}`)
      toast.success('Policy deleted')
      await invalidate(qk.policies, qk.dashboard)
      navigate('/policies')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (!isNew && policy.isPending) return <LoadingBlock />
  if (!isNew && policy.error) return <Alert tone="critical">{errorMessage(policy.error)}</Alert>
  const ro = !isAdmin
  const st = policy.data?.status ?? {}

  const tabs: { key: Tab; label: string; count?: number }[] = [
    { key: 'general', label: 'General' },
    { key: 'scope', label: 'Scope' },
    { key: 'scripts', label: 'Scripts', count: p.scripts?.length },
    { key: 'packages', label: 'Packages', count: p.packages?.length },
    { key: 'apps', label: 'Apps (Installomator)', count: p.apps?.length },
    { key: 'accounts', label: 'Local Accounts', count: p.accounts?.length },
    { key: 'files', label: 'Files & Processes' },
    { key: 'maintenance', label: 'Maintenance' },
    { key: 'restart', label: 'Restart' },
    { key: 'interaction', label: 'User Interaction' },
    { key: 'selfservice', label: 'Self Service' },
  ]
  if (!isNew) tabs.push({ key: 'logs', label: 'Logs' })

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Policies', to: '/policies' }, { label: isNew ? 'New policy' : policy.data?.name ?? '' }]}
        title={isNew ? 'New policy' : policy.data?.name}
        meta={
          !isNew ? (
            <>
              <Badge tone={p.enabled ? 'good' : 'neutral'} dot>
                {p.enabled ? 'Enabled' : 'Disabled'}
              </Badge>
              <span className="text-xs text-ink-muted tabular-nums">
                {st.completed ?? 0} completed · {st.failed ?? 0} failed · {(st.pending ?? 0) + (st.running ?? 0)} pending
              </span>
            </>
          ) : undefined
        }
        actions={
          isAdmin && (
            <>
              {!isNew && (
                <>
                  <Button variant="danger-outline" icon={<Trash2 />} onClick={() => setDel(true)}>
                    Delete
                  </Button>
                  <Button icon={<Play />} onClick={() => setRunOpen(true)}>
                    Run now
                  </Button>
                </>
              )}
              <Button variant="primary" icon={<Save />} loading={saving} onClick={save}>
                {isNew ? 'Create policy' : 'Save changes'}
              </Button>
            </>
          )
        }
      />
      <Tabs className="mb-5" value={tab} onChange={setTab} items={tabs} />

      {tab === 'general' && (
        <Card>
          <CardHeader title="General" />
          <CardBody className="flex flex-col gap-8">
            <FormGrid cols={2}>
              <Field label="Display name" required>
                <Input value={p.name} onChange={(e) => set({ name: e.target.value })} disabled={ro} />
              </Field>
              <Field label="Category">
                <Input value={p.category} onChange={(e) => set({ category: e.target.value })} disabled={ro} placeholder="e.g. Software" />
              </Field>
            </FormGrid>
            <Toggle checked={p.enabled} onChange={(v) => set({ enabled: v })} disabled={ro} label="Enabled" description="Disabled policies never run automatically but can still be run manually." />
            <FormSection title="Triggers" description="Events that cause the agent to evaluate this policy. Pick one or more.">
              <div className="grid gap-3 sm:grid-cols-2">
                <Checkbox checked={p.triggers.startup} onChange={(e) => set({ triggers: { ...p.triggers, startup: e.target.checked } })} disabled={ro} label="Startup" description="When the computer boots." />
                <Checkbox checked={p.triggers.login} onChange={(e) => set({ triggers: { ...p.triggers, login: e.target.checked } })} disabled={ro} label="Login" description="When a user logs in." />
                <Checkbox checked={p.triggers.checkin} onChange={(e) => set({ triggers: { ...p.triggers, checkin: e.target.checked } })} disabled={ro} label="Recurring check-in" description="On the agent's regular check-in interval." />
                <Checkbox checked={p.triggers.enrollment_complete} onChange={(e) => set({ triggers: { ...p.triggers, enrollment_complete: e.target.checked } })} disabled={ro} label="Enrollment complete" description="Once, right after a computer enrolls." />
                <Checkbox checked={p.triggers.network_change} onChange={(e) => set({ triggers: { ...p.triggers, network_change: e.target.checked } })} disabled={ro} label="Network state change" description="When the network configuration changes." />
                <Field label="Custom event" hint="Fired from the Management tab or by a custom trigger task.">
                  <Input value={p.triggers.custom} onChange={(e) => set({ triggers: { ...p.triggers, custom: e.target.value.trim() } })} disabled={ro} placeholder="e.g. install-office" mono />
                </Field>
              </div>
            </FormSection>
            <FormSection title="Execution frequency" description="How often the policy may run on each computer.">
              <Select className="max-w-xs" value={p.frequency} onChange={(e) => set({ frequency: e.target.value as PolicyFrequency })} disabled={ro} options={Object.entries(frequencyLabels).map(([value, label]) => ({ value, label }))} />
            </FormSection>
          </CardBody>
        </Card>
      )}

      {tab === 'scope' && (
        <Card>
          <CardHeader title="Scope" description="Which computers this policy targets." />
          <CardBody>
            <ScopeEditor value={p.scope} onChange={(scope) => set({ scope })} disabled={ro} />
          </CardBody>
        </Card>
      )}

      {tab === 'scripts' && <ScriptsSection value={p.scripts ?? []} onChange={(scripts) => set({ scripts })} disabled={ro} />}
      {tab === 'packages' && <PackagesSection value={p.packages ?? []} onChange={(packages) => set({ packages })} disabled={ro} />}
      {tab === 'apps' && <AppsSection value={p.apps ?? []} onChange={(apps) => set({ apps })} disabled={ro} />}
      {tab === 'accounts' && <LocalAccountsSection value={p.accounts ?? []} onChange={(accounts) => set({ accounts })} disabled={ro} />}

      {tab === 'files' && (
        <Card>
          <CardHeader title="Files and processes" description="Run an arbitrary command after scripts, packages and Installomator apps." />
          <CardBody>
            <Field label="Execute command" hint="Runs as root via /bin/sh after all other payloads.">
              <Textarea mono rows={6} value={p.run_command} onChange={(e) => set({ run_command: e.target.value })} disabled={ro} placeholder="e.g. /usr/bin/defaults write ..." />
            </Field>
          </CardBody>
        </Card>
      )}

      {tab === 'maintenance' && (
        <Card>
          <CardHeader title="Maintenance" />
          <CardBody>
            <Checkbox checked={p.update_inventory} onChange={(e) => set({ update_inventory: e.target.checked })} disabled={ro} label="Update inventory" description="Submit a fresh inventory report after the policy completes." />
          </CardBody>
        </Card>
      )}

      {tab === 'restart' && (
        <Card>
          <CardHeader title="Restart options" />
          <CardBody className="flex flex-col gap-5">
            <Toggle checked={p.reboot.enabled} onChange={(v) => set({ reboot: { ...p.reboot, enabled: v } })} disabled={ro} label="Restart after the policy completes" />
            <FormGrid cols={2}>
              <Field label="Message shown to the user">
                <Textarea rows={3} value={p.reboot.message} onChange={(e) => set({ reboot: { ...p.reboot, message: e.target.value } })} disabled={ro || !p.reboot.enabled} placeholder="This computer will restart in 5 minutes. Please save your work." />
              </Field>
              <Field label="Delay (minutes)" hint="0 restarts immediately.">
                <Input type="number" min={0} className="max-w-[160px]" value={p.reboot.delay_minutes} onChange={(e) => set({ reboot: { ...p.reboot, delay_minutes: Math.max(0, Number(e.target.value) || 0) } })} disabled={ro || !p.reboot.enabled} />
              </Field>
            </FormGrid>
          </CardBody>
        </Card>
      )}

      {tab === 'interaction' && (
        <Card>
          <CardHeader title="User interaction" description="Messages shown to the logged-in user via notification." />
          <CardBody>
            <FormGrid cols={2}>
              <Field label="Start message">
                <Textarea rows={3} value={p.user_interaction.start_message} onChange={(e) => set({ user_interaction: { ...p.user_interaction, start_message: e.target.value } })} disabled={ro} />
              </Field>
              <Field label="Complete message">
                <Textarea rows={3} value={p.user_interaction.complete_message} onChange={(e) => set({ user_interaction: { ...p.user_interaction, complete_message: e.target.value } })} disabled={ro} />
              </Field>
            </FormGrid>
          </CardBody>
        </Card>
      )}

      {tab === 'selfservice' && (
        <Card>
          <CardHeader title="Self Service" description="Let users run this policy themselves from the Self Service app." />
          <CardBody className="flex flex-col gap-5">
            <p className="text-xs text-ink-muted">
              Shown in the Self Service app on each Mac in scope (Settings → Self Service).
            </p>
            <Toggle checked={p.self_service.enabled} onChange={(v) => set({ self_service: { ...p.self_service, enabled: v } })} disabled={ro} label="Make available in Self Service" />
            <FormGrid cols={2}>
              <Field label="Display name">
                <Input value={p.self_service.display_name} onChange={(e) => set({ self_service: { ...p.self_service, display_name: e.target.value } })} disabled={ro || !p.self_service.enabled} placeholder={p.name} />
              </Field>
              <Field label="Button text">
                <Input value={p.self_service.button_text} onChange={(e) => set({ self_service: { ...p.self_service, button_text: e.target.value } })} disabled={ro || !p.self_service.enabled} placeholder="Install" />
              </Field>
              <Field label="Description" className="md:col-span-2">
                <Textarea rows={3} value={p.self_service.description} onChange={(e) => set({ self_service: { ...p.self_service, description: e.target.value } })} disabled={ro || !p.self_service.enabled} />
              </Field>
              <Field label="Category" hint="Groups the item in Self Service.">
                <Input value={p.self_service.category ?? ''} onChange={(e) => set({ self_service: { ...p.self_service, category: e.target.value } })} disabled={ro || !p.self_service.enabled} placeholder={p.category || 'General'} />
              </Field>
              <Field label="Featured">
                <Checkbox checked={!!p.self_service.featured} onChange={(e) => set({ self_service: { ...p.self_service, featured: e.target.checked } })} disabled={ro || !p.self_service.enabled} label="Show on the Discover page" />
              </Field>
            </FormGrid>
          </CardBody>
        </Card>
      )}

      {tab === 'logs' && !isNew && id && <PolicyLogsTab policyId={id} />}

      <ConfirmDialog open={del} onClose={() => setDel(false)} onConfirm={doDelete} destructive title={`Delete "${p.name}"?`} description="The policy and its logs are removed." confirmLabel="Delete policy" />
      {!isNew && id && <RunPolicyDialog open={runOpen} onClose={() => setRunOpen(false)} policyId={id} policyName={p.name} />}
    </>
  )
}

function ScriptsSection({ value, onChange, disabled }: { value: PolicyScript[]; onChange: (v: PolicyScript[]) => void; disabled?: boolean }) {
  const scripts = useScripts()
  const byId = useMemo(() => new Map((scripts.data?.items ?? []).map((s) => [s.id, s])), [scripts.data])
  const [adding, setAdding] = useState('')
  const add = () => {
    if (!adding || value.some((s) => s.script_id === adding)) return
    const s = byId.get(adding)
    onChange([...value, { script_id: adding, priority: s?.priority ?? 'after', parameters: Array(8).fill('') }])
    setAdding('')
  }
  const update = (i: number, patch: Partial<PolicyScript>) => onChange(value.map((s, j) => (j === i ? { ...s, ...patch } : s)))
  return (
    <Card>
      <CardHeader title="Scripts" description="Scripts run in order; those with priority “before” run ahead of package installs." />
      <CardBody className="flex flex-col gap-4">
        {value.length === 0 && <p className="text-sm text-ink-muted">No scripts added.</p>}
        {value.map((ps, i) => {
          const s = byId.get(ps.script_id)
          const labels = s?.parameter_labels ?? []
          const params = [...(ps.parameters ?? [])]
          while (params.length < 8) params.push('')
          return (
            <div key={ps.script_id} className="rounded-xl border border-line">
              <div className="flex flex-wrap items-center gap-3 border-b border-line px-4 py-3">
                <span className="font-medium text-ink flex-1 min-w-0 truncate">{s?.name ?? 'Unknown script'}</span>
                <Select className="w-44" value={ps.priority} onChange={(e) => update(i, { priority: e.target.value as ScriptPriority })} disabled={disabled} options={[{ value: 'before', label: 'Before packages' }, { value: 'after', label: 'After packages' }]} />
                {!disabled && (
                  <IconButton label="Remove script" variant="ghost" onClick={() => onChange(value.filter((_, j) => j !== i))}>
                    <Trash2 />
                  </IconButton>
                )}
              </div>
              <div className="grid gap-3 px-4 py-3 sm:grid-cols-2 lg:grid-cols-4">
                {params.map((v, k) => (
                  <Field key={k} label={`$${k + 4}${labels[k] ? ` — ${labels[k]}` : ''}`}>
                    <Input value={v} onChange={(e) => update(i, { parameters: params.map((x, j) => (j === k ? e.target.value : x)) })} disabled={disabled} placeholder={labels[k] || 'Optional'} />
                  </Field>
                ))}
              </div>
            </div>
          )
        })}
        {!disabled && (
          <div className="flex items-end gap-2">
            <Field label="Add script" className="flex-1 max-w-md">
              <ScriptPicker value={adding} onChange={setAdding} />
            </Field>
            <Button icon={<Plus />} onClick={add} disabled={!adding}>
              Add
            </Button>
          </div>
        )}
      </CardBody>
    </Card>
  )
}

function PackagesSection({ value, onChange, disabled }: { value: PolicyPackage[]; onChange: (v: PolicyPackage[]) => void; disabled?: boolean }) {
  const packages = usePackages()
  const byId = useMemo(() => new Map((packages.data?.items ?? []).map((p) => [p.id, p])), [packages.data])
  const [adding, setAdding] = useState('')
  const add = () => {
    if (!adding || value.some((p) => p.package_id === adding)) return
    onChange([...value, { package_id: adding, action: 'install' }])
    setAdding('')
  }
  return (
    <Card>
      <CardHeader title="Packages" description="Packages are downloaded from the server and installed in order of their priority." />
      <CardBody className="flex flex-col gap-4">
        {value.length === 0 && <p className="text-sm text-ink-muted">No packages added.</p>}
        {value.map((pp, i) => {
          const pk = byId.get(pp.package_id)
          return (
            <div key={pp.package_id} className="flex flex-wrap items-center gap-3 rounded-xl border border-line px-4 py-3">
              <span className="min-w-0 flex-1">
                <span className="block font-medium text-ink truncate">{pk?.name ?? 'Unknown package'}</span>
                <span className="block text-xs text-ink-muted font-mono truncate">
                  {pk?.filename} {pk && `· ${formatBytes(pk.size_bytes)}`}
                </span>
              </span>
              <Badge>Install</Badge>
              {!disabled && (
                <IconButton label="Remove package" variant="ghost" onClick={() => onChange(value.filter((_, j) => j !== i))}>
                  <Trash2 />
                </IconButton>
              )}
            </div>
          )
        })}
        {!disabled && (
          <div className="flex items-end gap-2">
            <Field label="Add package" className="flex-1 max-w-md">
              <Select value={adding} onChange={(e) => setAdding(e.target.value)}>
                <option value="">{packages.isPending ? 'Loading…' : (packages.data?.items.length ?? 0) === 0 ? 'No packages uploaded' : 'Select a package…'}</option>
                {(packages.data?.items ?? [])
                  .filter((pk) => !value.some((v) => v.package_id === pk.id))
                  .map((pk) => (
                    <option key={pk.id} value={pk.id}>
                      {pk.name} ({pk.filename})
                    </option>
                  ))}
              </Select>
            </Field>
            <Button icon={<Plus />} onClick={add} disabled={!adding}>
              Add
            </Button>
          </div>
        )}
      </CardBody>
    </Card>
  )
}

function AppsSection({ value, onChange, disabled }: { value: PolicyApp[]; onChange: (v: PolicyApp[]) => void; disabled?: boolean }) {
  const labels = useAppLabels()
  const known = useMemo(() => new Set(allLabels(labels.data)), [labels.data])
  const [adding, setAdding] = useState('')
  const [previews, setPreviews] = useState<Set<number>>(() => new Set())
  const add = () => {
    if (!adding) return
    onChange([...value, { label: adding, options: emptyInstallomatorOptions() }])
    setAdding('')
  }
  const update = (i: number, patch: Partial<PolicyApp>) => onChange(value.map((a, j) => (j === i ? { ...a, ...patch } : a)))
  const move = (i: number, d: -1 | 1) => {
    const j = i + d
    if (j < 0 || j >= value.length) return
    const next = [...value]
    ;[next[i], next[j]] = [next[j], next[i]]
    onChange(next)
    setPreviews(new Set())
  }
  const remove = (i: number) => {
    onChange(value.filter((_, j) => j !== i))
    setPreviews(new Set())
  }
  const togglePreview = (i: number) =>
    setPreviews((s) => {
      const n = new Set(s)
      if (n.has(i)) n.delete(i)
      else n.add(i)
      return n
    })
  return (
    <Card>
      <CardHeader title="Apps (Installomator)" description="Installomator labels run in this order, after packages and before Files & Processes. Each download's signature is checked against the label's Team ID." />
      <CardBody className="flex flex-col gap-4">
        {value.length === 0 && <p className="text-sm text-ink-muted">No apps added.</p>}
        {value.map((pa, i) => {
          const custom = pa.label.startsWith('custom:')
          const unknown = !!labels.data && !known.has(pa.label)
          return (
            <div key={`${i}-${pa.label}`} className="rounded-xl border border-line">
              <div className="flex flex-wrap items-center gap-3 border-b border-line px-4 py-3">
                <span className="w-6 text-center text-xs tabular-nums text-ink-faint">{i + 1}</span>
                <span className="min-w-0 flex-1 truncate font-mono text-sm text-ink">{pa.label}</span>
                {custom && <Badge tone="accent">Custom</Badge>}
                {unknown && <Badge tone="critical">Unknown label</Badge>}
                <Button size="xs" variant="ghost" icon={<Terminal />} onClick={() => togglePreview(i)}>
                  {previews.has(i) ? 'Hide command' : 'Preview command'}
                </Button>
                {!disabled && (
                  <>
                    <IconButton label="Move up" variant="ghost" onClick={() => move(i, -1)} disabled={i === 0}>
                      <ArrowUp />
                    </IconButton>
                    <IconButton label="Move down" variant="ghost" onClick={() => move(i, 1)} disabled={i === value.length - 1}>
                      <ArrowDown />
                    </IconButton>
                    <IconButton label="Remove app" variant="ghost" onClick={() => remove(i)}>
                      <Trash2 />
                    </IconButton>
                  </>
                )}
              </div>
              <div className="flex flex-col gap-4 px-4 py-3">
                <InstallomatorOptionsEditor compact value={pa.options} onChange={(options) => update(i, { options })} disabled={disabled} />
                {previews.has(i) && <InstallomatorCommandPreview body={{ label: pa.label, options: pa.options }} />}
              </div>
            </div>
          )
        })}
        {!disabled && (
          <div className="flex items-end gap-2">
            <Field label="Add app" className="max-w-md flex-1">
              <LabelCombobox value={adding} onChange={setAdding} placeholder="Search Installomator and custom labels…" />
            </Field>
            <Button icon={<Plus />} onClick={add} disabled={!adding || (!!labels.data && !known.has(adding))}>
              Add
            </Button>
          </div>
        )}
        <InstallomatorFooter />
      </CardBody>
    </Card>
  )
}

function PolicyLogsTab({ policyId }: { policyId: string }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [status, setStatus] = useState('')
  const [page, setPage] = useState({ limit: 25, offset: 0 })
  const logs = usePolicyLogs(policyId, { status: status || undefined, ...page })
  const [flushOpen, setFlushOpen] = useState(false)
  const [flushComputers, setFlushComputers] = useState<string[]>([])
  const [flushing, setFlushing] = useState(false)

  const flush = async () => {
    setFlushing(true)
    try {
      let deleted = 0
      if (flushComputers.length === 0) {
        const r = await api.post<{ deleted: number }>(`/api/v1/policies/${policyId}/flush`, {})
        deleted = r.deleted
      } else {
        for (const cid of flushComputers) {
          const r = await api.post<{ deleted: number }>(`/api/v1/policies/${policyId}/flush`, { computer_id: cid })
          deleted += r.deleted
        }
      }
      toast.success(`Flushed ${deleted} log ${deleted === 1 ? 'entry' : 'entries'}`, 'Affected computers can run the policy again.')
      await invalidate(['policy', policyId], qk.policies, ['policy-logs'], qk.dashboard)
      setFlushOpen(false)
      setFlushComputers([])
    } catch (e) {
      toast.error('Flush failed', errorMessage(e))
    } finally {
      setFlushing(false)
    }
  }

  return (
    <Card>
      <CardHeader
        title="Logs"
        description="Each run on each computer. Flushing logs lets a “once” policy run again."
        actions={
          <>
            <Select
              className="w-40"
              value={status}
              onChange={(e) => {
                setStatus(e.target.value)
                setPage({ ...page, offset: 0 })
              }}
              options={[
                { value: '', label: 'All statuses' },
                { value: 'completed', label: 'Completed' },
                { value: 'failed', label: 'Failed' },
                { value: 'pending', label: 'Pending' },
                { value: 'running', label: 'Running' },
              ]}
            />
            {isAdmin && (
              <Button icon={<Eraser />} onClick={() => setFlushOpen(true)}>
                Flush logs
              </Button>
            )}
          </>
        }
      />
      <PolicyLogTable logs={logs.data?.items} loading={logs.isPending} showPolicy={false} />
      <Pagination total={logs.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />
      <Modal
        open={flushOpen}
        onClose={() => setFlushOpen(false)}
        title="Flush policy logs"
        description="Deletes run history for this policy. Leave the computer list empty to flush logs for every computer."
        size="md"
        footer={
          <>
            <Button variant="ghost" onClick={() => setFlushOpen(false)}>
              Cancel
            </Button>
            <Button variant="danger" loading={flushing} onClick={flush}>
              {flushComputers.length === 0 ? 'Flush all logs' : `Flush for ${flushComputers.length} computer${flushComputers.length === 1 ? '' : 's'}`}
            </Button>
          </>
        }
      >
        <Field label="Limit to computers (optional)">
          <ComputerPicker value={flushComputers} onChange={setFlushComputers} />
        </Field>
      </Modal>
    </Card>
  )
}

function RunPolicyDialog({ open, onClose, policyId, policyName }: { open: boolean; onClose: () => void; policyId: string; policyName: string }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const [mode, setMode] = useState<'scope' | 'pick'>('scope')
  const [targets, setTargets] = useState<Targets>(emptyTargets())
  const [busy, setBusy] = useState(false)
  const run = async () => {
    setBusy(true)
    try {
      const body = mode === 'scope' ? { scoped: true, computer_ids: [], group_ids: [] } : { scoped: false, ...targets }
      const r = await api.post<RunResult>(`/api/v1/policies/${policyId}/run`, body)
      if (r.queued === 0 && r.skipped.length === 0) toast.warning('No computers targeted')
      else if (r.skipped.length) toast.warning(`Queued on ${r.queued}; ${r.skipped.length} skipped`, r.skipped.join('\n'))
      else toast.success(`Queued on ${r.queued} computer${r.queued === 1 ? '' : 's'}`)
      await invalidate(['tasks'], ['policy', policyId], qk.policies, qk.dashboard)
      onClose()
    } catch (e) {
      toast.error('Run failed', errorMessage(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Run "${policyName}" now`}
      description="Runs immediately on the agent, ignoring triggers and frequency."
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={busy} disabled={mode === 'pick' && targets.computer_ids.length + targets.group_ids.length === 0} onClick={run}>
            Run policy
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <label className="flex items-center gap-2 text-sm">
            <input type="radio" name="runmode" checked={mode === 'scope'} onChange={() => setMode('scope')} className="accent-accent-600" />
            Every computer in the policy's scope
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input type="radio" name="runmode" checked={mode === 'pick'} onChange={() => setMode('pick')} className="accent-accent-600" />
            Specific computers or groups
          </label>
        </div>
        {mode === 'pick' && <TargetPicker value={targets} onChange={setTargets} />}
      </div>
    </Modal>
  )
}
