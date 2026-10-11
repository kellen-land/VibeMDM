import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Briefcase, Building2, Eye } from 'lucide-react'
import { normalizeMobileScope, type MobileScope, type MobileScopePreview, type MobileScopeSelection } from '../../api/types'
import { useBuildings, useDepartments, useMobileGroups } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { Toggle } from '../ui/Checkbox'
import { Field, FormSection } from '../ui/Field'
import { Tabs } from '../ui/Tabs'
import { Button } from '../ui/Button'
import { Badge } from '../ui/Badge'
import { useToast } from '../ui/Toast'
import { GroupPicker } from './GroupPicker'
import { MobileDevicePicker } from './MobileDevicePicker'
import { NamedPicker, type NamedOption } from './NamedPicker'

function selectionCount(s: MobileScopeSelection): number {
  return s.mobile_device_ids.length + s.group_ids.length + s.buildings.length + s.departments.length
}

function SelectionEditor({ value, onChange, disabled, prefix }: { value: MobileScopeSelection; onChange: (s: MobileScopeSelection) => void; disabled?: boolean; prefix: string }) {
  const buildings = useBuildings()
  const departments = useDepartments()
  const buildingOpts = useMemo<NamedOption[]>(() => (buildings.data?.items ?? []).map((b) => ({ value: b.name, label: b.name })), [buildings.data])
  const departmentOpts = useMemo<NamedOption[]>(() => (departments.data?.items ?? []).map((d) => ({ value: d.name, label: d.name })), [departments.data])
  const set = (patch: Partial<MobileScopeSelection>) => onChange({ ...value, ...patch })
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Field label={`${prefix} mobile devices`}>
        <MobileDevicePicker value={value.mobile_device_ids} onChange={(ids) => set({ mobile_device_ids: ids })} disabled={disabled} />
      </Field>
      <Field label={`${prefix} mobile device groups`}>
        <GroupPicker kind="mobile" value={value.group_ids} onChange={(ids) => set({ group_ids: ids })} disabled={disabled} />
      </Field>
      <Field label={`${prefix} buildings`}>
        <NamedPicker value={value.buildings} onChange={(v) => set({ buildings: v })} options={buildingOpts} loading={buildings.isPending} disabled={disabled} icon={<Building2 />} placeholder="Search buildings…" emptyText="No buildings defined. Add them under Settings → Buildings and networks." />
      </Field>
      <Field label={`${prefix} departments`}>
        <NamedPicker value={value.departments} onChange={(v) => set({ departments: v })} options={departmentOpts} loading={departments.isPending} disabled={disabled} icon={<Briefcase />} placeholder="Search departments…" emptyText="No departments defined. Add them under Settings → Buildings and networks." />
      </Field>
    </div>
  )
}

/** Scope for mobile objects: Targets and Exclusions over iPhones, iPads and mobile device groups. */
export function MobileScopeEditor({ value: raw, onChange, disabled }: { value: MobileScope; onChange: (s: MobileScope) => void; disabled?: boolean }) {
  const value = useMemo(() => normalizeMobileScope(raw), [raw])
  const toast = useToast()
  const [tab, setTab] = useState<'targets' | 'exclusions'>('targets')
  const [preview, setPreview] = useState<MobileScopePreview | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const set = (patch: Partial<MobileScope>) => onChange({ ...value, ...patch })

  const runPreview = async () => {
    setPreviewing(true)
    try {
      setPreview(await api.post<MobileScopePreview>('/api/v1/mobile-scope/preview', { scope: value }))
    } catch (e) {
      toast.error('Preview failed', errorMessage(e))
    } finally {
      setPreviewing(false)
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs
          variant="pills"
          value={tab}
          onChange={setTab}
          items={[
            { key: 'targets', label: 'Targets', count: value.all_mobile_devices ? undefined : selectionCount(value.targets) },
            { key: 'exclusions', label: 'Exclusions', count: selectionCount(value.exclusions) || undefined },
          ]}
        />
        {!disabled && (
          <Button size="sm" icon={<Eye />} loading={previewing} onClick={runPreview}>
            Preview
          </Button>
        )}
      </div>
      {tab === 'targets' && (
        <FormSection title="Targets" description="Which iPhones and iPads this applies to. Group membership is checked whenever the profile is saved or synced.">
          <Toggle checked={value.all_mobile_devices} onChange={(v) => set({ all_mobile_devices: v })} disabled={disabled} label="All mobile devices" description="Every enrolled iPhone and iPad. Exclusions still apply." />
          {!value.all_mobile_devices && <SelectionEditor value={value.targets} onChange={(targets) => set({ targets })} disabled={disabled} prefix="Target" />}
        </FormSection>
      )}
      {tab === 'exclusions' && (
        <FormSection title="Exclusions" description="Exclusions always win: anything listed here is left out even if it matches a target.">
          <SelectionEditor value={value.exclusions} onChange={(exclusions) => set({ exclusions })} disabled={disabled} prefix="Excluded" />
        </FormSection>
      )}
      {preview && (
        <div className="rounded-xl border border-line">
          <div className="flex items-center justify-between border-b border-line px-4 py-2.5">
            <p className="text-sm font-medium text-ink">
              {preview.count.toLocaleString()} {preview.count === 1 ? 'device' : 'devices'} in scope
            </p>
            <Button size="xs" variant="ghost" onClick={() => setPreview(null)}>
              Dismiss
            </Button>
          </div>
          {preview.mobile_devices.length > 0 ? (
            <ul className="divide-y divide-line">
              {preview.mobile_devices.map((d) => (
                <li key={d.id} className="flex items-center justify-between px-4 py-2 text-sm">
                  <Link to={`/mobile-devices/${d.id}`} className="text-ink hover:underline">
                    {d.name || d.serial_number}
                  </Link>
                  <span className="text-xs text-ink-muted">{d.model_name || d.device_type}</span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="px-4 py-3 text-sm text-ink-muted">No devices are in scope.</p>
          )}
        </div>
      )}
    </div>
  )
}

/** Short summary of a mobile scope for lists. */
export function MobileScopeSummary({ scope: raw }: { scope: MobileScope }) {
  const scope = normalizeMobileScope(raw)
  const groups = useMobileGroups()
  const byId = new Map((groups.data?.items ?? []).map((g) => [g.id, g.name]))
  const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`
  const parts: string[] = []
  if (scope.all_mobile_devices) parts.push('All mobile devices')
  else {
    const t = scope.targets
    if (t.mobile_device_ids.length) parts.push(plural(t.mobile_device_ids.length, 'device'))
    if (t.group_ids.length) parts.push(t.group_ids.length <= 2 ? t.group_ids.map((g) => byId.get(g) ?? '…').join(', ') : plural(t.group_ids.length, 'group'))
    if (t.buildings.length) parts.push(t.buildings.length <= 2 ? t.buildings.join(', ') : plural(t.buildings.length, 'building'))
    if (t.departments.length) parts.push(t.departments.length <= 2 ? t.departments.join(', ') : plural(t.departments.length, 'department'))
  }
  if (parts.length === 0) return <Badge tone="neutral">No targets</Badge>
  const ex = selectionCount(scope.exclusions)
  return (
    <span className="text-sm">
      {parts.join(' · ')}
      {ex > 0 && <span className="text-ink-muted"> · {plural(ex, 'exclusion')}</span>}
    </span>
  )
}
