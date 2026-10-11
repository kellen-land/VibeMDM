import { useMemo, useState } from 'react'
import { Building2, Briefcase, Eye, Network, UserRound } from 'lucide-react'
import { normalizeScope, type Scope, type ScopeLimitations, type ScopePreview, type ScopeSelection } from '../../api/types'
import { useBuildings, useDepartments, useGroups, useNetworkSegments } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { Toggle } from '../ui/Checkbox'
import { Field, FormSection } from '../ui/Field'
import { Tabs } from '../ui/Tabs'
import { Button } from '../ui/Button'
import { Badge } from '../ui/Badge'
import { useToast } from '../ui/Toast'
import { ComputerPicker } from './ComputerPicker'
import { GroupPicker } from './GroupPicker'
import { NamedPicker, type NamedOption } from './NamedPicker'
import { ChipInput } from './ChipInput'
import { MemberTable } from '../../pages/GroupEditorPage'

type ScopeTab = 'targets' | 'limitations' | 'exclusions'

function selectionCount(s: ScopeSelection): number {
  return s.computer_ids.length + s.group_ids.length + s.buildings.length + s.departments.length + s.network_segments.length + s.users.length
}
function limitationCount(l: ScopeLimitations): number {
  return l.users.length + l.network_segments.length
}

/** Lists used by every scope section; shared so the three tabs stay in sync. */
function useScopeVocabulary() {
  const buildings = useBuildings()
  const departments = useDepartments()
  const segments = useNetworkSegments()
  const buildingOpts = useMemo<NamedOption[]>(() => (buildings.data?.items ?? []).map((b) => ({ value: b.name, label: b.name, hint: b.computers ? `${b.computers}` : undefined })), [buildings.data])
  const departmentOpts = useMemo<NamedOption[]>(() => (departments.data?.items ?? []).map((d) => ({ value: d.name, label: d.name, hint: d.computers ? `${d.computers}` : undefined })), [departments.data])
  const segmentOpts = useMemo<NamedOption[]>(() => (segments.data?.items ?? []).map((n) => ({ value: n.id, label: n.name, hint: `${n.starting_address}–${n.ending_address}` })), [segments.data])
  return { buildings, departments, segments, buildingOpts, departmentOpts, segmentOpts }
}

type Vocab = ReturnType<typeof useScopeVocabulary>

/** Pickers for one ScopeSelection (targets or exclusions). */
function SelectionEditor({ value, onChange, disabled, vocab, withUsers, prefix }: { value: ScopeSelection; onChange: (s: ScopeSelection) => void; disabled?: boolean; vocab: Vocab; withUsers?: boolean; prefix: string }) {
  const set = (patch: Partial<ScopeSelection>) => onChange({ ...value, ...patch })
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Field label={`${prefix} computers`}>
        <ComputerPicker value={value.computer_ids} onChange={(ids) => set({ computer_ids: ids })} disabled={disabled} />
      </Field>
      <Field label={`${prefix} computer groups`}>
        <GroupPicker value={value.group_ids} onChange={(ids) => set({ group_ids: ids })} disabled={disabled} />
      </Field>
      <Field label={`${prefix} buildings`}>
        <NamedPicker value={value.buildings} onChange={(v) => set({ buildings: v })} options={vocab.buildingOpts} loading={vocab.buildings.isPending} disabled={disabled} icon={<Building2 />} placeholder="Search buildings…" emptyText="No buildings defined. Add them under Settings → Buildings and networks." />
      </Field>
      <Field label={`${prefix} departments`}>
        <NamedPicker value={value.departments} onChange={(v) => set({ departments: v })} options={vocab.departmentOpts} loading={vocab.departments.isPending} disabled={disabled} icon={<Briefcase />} placeholder="Search departments…" emptyText="No departments defined. Add them under Settings → Buildings and networks." />
      </Field>
      <Field label={`${prefix} network segments`} hint="Matched against the computer's last reported IP address.">
        <NamedPicker value={value.network_segments} onChange={(v) => set({ network_segments: v })} options={vocab.segmentOpts} loading={vocab.segments.isPending} disabled={disabled} icon={<Network />} placeholder="Search network segments…" emptyText="No network segments defined. Add them under Settings → Buildings and networks." />
      </Field>
      {withUsers && (
        <Field label={`${prefix} users`} hint="Console or assigned username. Press Enter after each one.">
          <ChipInput value={value.users} onChange={(v) => set({ users: v })} disabled={disabled} icon={<UserRound />} placeholder="e.g. jappleseed" mono />
        </Field>
      )}
    </div>
  )
}

export function ScopeEditor({ value: raw, onChange, disabled }: { value: Scope; onChange: (s: Scope) => void; disabled?: boolean }) {
  const value = useMemo(() => normalizeScope(raw), [raw])
  const vocab = useScopeVocabulary()
  const toast = useToast()
  const [tab, setTab] = useState<ScopeTab>('targets')
  const [preview, setPreview] = useState<ScopePreview | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const set = (patch: Partial<Scope>) => onChange({ ...value, ...patch })

  const runPreview = async () => {
    setPreviewing(true)
    try {
      setPreview(await api.post<ScopePreview>('/api/v1/scope/preview', { scope: value }))
    } catch (e) {
      toast.error('Preview failed', errorMessage(e))
    } finally {
      setPreviewing(false)
    }
  }

  const targetCount = value.all_computers ? undefined : selectionCount(value.targets)
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs
          variant="pills"
          value={tab}
          onChange={setTab}
          items={[
            { key: 'targets', label: 'Targets', count: targetCount },
            { key: 'limitations', label: 'Limitations', count: limitationCount(value.limitations) || undefined },
            { key: 'exclusions', label: 'Exclusions', count: selectionCount(value.exclusions) || undefined },
          ]}
        />
        <Button size="sm" icon={<Eye />} loading={previewing} onClick={runPreview}>
          Preview
        </Button>
      </div>
      <p className="text-xs text-ink-muted -mt-2">
        A computer is in scope when it matches a <span className="text-ink">target</span>, passes every <span className="text-ink">limitation</span>, and is not <span className="text-ink">excluded</span>.
      </p>

      {tab === 'targets' && (
        <FormSection title="Targets" description="Which computers this applies to. Group and location membership is re-evaluated whenever the item is deployed or a trigger fires.">
          <Toggle checked={value.all_computers} onChange={(v) => set({ all_computers: v })} disabled={disabled} label="All computers" description="Target every enrolled computer. Limitations and exclusions still apply." />
          {!value.all_computers && <SelectionEditor value={value.targets} onChange={(targets) => set({ targets })} disabled={disabled} vocab={vocab} prefix="Target" />}
        </FormSection>
      )}

      {tab === 'limitations' && (
        <FormSection title="Limitations" description="Targets stay in scope only if they match every limitation set here. Values within one category are OR'd together.">
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Users" hint="Console or assigned username. Press Enter after each one.">
              <ChipInput value={value.limitations.users} onChange={(users) => set({ limitations: { ...value.limitations, users } })} disabled={disabled} icon={<UserRound />} placeholder="e.g. jappleseed" mono />
            </Field>
            <Field label="Network segments" hint="Only computers whose last IP falls inside one of these ranges stay in scope.">
              <NamedPicker value={value.limitations.network_segments} onChange={(network_segments) => set({ limitations: { ...value.limitations, network_segments } })} options={vocab.segmentOpts} loading={vocab.segments.isPending} disabled={disabled} icon={<Network />} placeholder="Search network segments…" emptyText="No network segments defined. Add them under Settings → Buildings and networks." />
            </Field>
          </div>
        </FormSection>
      )}

      {tab === 'exclusions' && (
        <FormSection title="Exclusions" description="Exclusions always win. Anything listed here is left out even if it matches a target.">
          <SelectionEditor value={value.exclusions} onChange={(exclusions) => set({ exclusions })} disabled={disabled} vocab={vocab} withUsers prefix="Excluded" />
        </FormSection>
      )}

      {preview && (
        <div className="rounded-xl border border-line">
          <div className="flex items-center justify-between px-4 py-2.5 border-b border-line">
            <p className="text-sm font-medium text-ink">
              Preview: {preview.count.toLocaleString()} {preview.count === 1 ? 'computer' : 'computers'} in scope
              {preview.count > preview.computers.length && <span className="text-xs text-ink-muted font-normal"> (showing first {preview.computers.length})</span>}
            </p>
            <Button size="xs" variant="ghost" onClick={() => setPreview(null)}>
              Dismiss
            </Button>
          </div>
          <MemberTable rows={preview.computers} emptyTitle="No computers are in scope" />
        </div>
      )}
    </div>
  )
}

/** Short human summary of a scope, e.g. "All computers · limited to 2 segments · 1 exclusion". */
export function ScopeSummary({ scope: raw }: { scope: Scope }) {
  const scope = normalizeScope(raw)
  const groups = useGroups()
  const byId = new Map((groups.data?.items ?? []).map((g) => [g.id, g.name]))
  const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`
  const parts: string[] = []
  if (scope.all_computers) parts.push('All computers')
  else {
    const t = scope.targets
    if (t.computer_ids.length) parts.push(plural(t.computer_ids.length, 'computer'))
    if (t.group_ids.length) {
      const names = t.group_ids.map((g) => byId.get(g) ?? '…')
      parts.push(names.length <= 2 ? names.join(', ') : plural(t.group_ids.length, 'group'))
    }
    if (t.buildings.length) parts.push(t.buildings.length <= 2 ? t.buildings.join(', ') : plural(t.buildings.length, 'building'))
    if (t.departments.length) parts.push(t.departments.length <= 2 ? t.departments.join(', ') : plural(t.departments.length, 'department'))
    if (t.network_segments.length) parts.push(plural(t.network_segments.length, 'segment'))
    if (t.users.length) parts.push(plural(t.users.length, 'user'))
    if (parts.length === 0) parts.push('No targets')
  }
  const lim: string[] = []
  if (scope.limitations.users.length) lim.push(plural(scope.limitations.users.length, 'user'))
  if (scope.limitations.network_segments.length) lim.push(plural(scope.limitations.network_segments.length, 'segment'))
  const ex = selectionCount(scope.exclusions)
  return (
    <span className="text-sm">
      {parts.join(' · ')}
      {lim.length > 0 && <span className="text-ink-muted"> · limited to {lim.join(', ')}</span>}
      {ex > 0 && <span className="text-ink-muted"> · {plural(ex, 'exclusion')}</span>}
    </span>
  )
}

/** Tiny badge for places that just need "scoped / all / none". */
export function ScopeBadge({ scope: raw }: { scope: Scope }) {
  const scope = normalizeScope(raw)
  if (scope.all_computers) return <Badge tone="accent">All computers</Badge>
  const n = selectionCount(scope.targets)
  return n === 0 ? <Badge tone="neutral">No targets</Badge> : <Badge>{n} target{n === 1 ? '' : 's'}</Badge>
}
