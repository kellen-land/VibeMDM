import type { Scope } from '../../api/types'
import { Toggle } from '../ui/Checkbox'
import { Field, FormSection } from '../ui/Field'
import { ComputerPicker } from './ComputerPicker'
import { GroupPicker } from './GroupPicker'
import { useGroups } from '../../api/queries'

export function ScopeEditor({ value, onChange, disabled }: { value: Scope; onChange: (s: Scope) => void; disabled?: boolean }) {
  const set = (patch: Partial<Scope>) => onChange({ ...value, ...patch })
  return (
    <div className="flex flex-col gap-8">
      <FormSection title="Targets" description="Which computers this applies to. Group membership is evaluated when the item is deployed or a trigger fires.">
        <Toggle checked={value.all_computers} onChange={(v) => set({ all_computers: v })} disabled={disabled} label="All computers" description="Target every enrolled computer. Exclusions below still apply." />
        {!value.all_computers && (
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Computers">
              <ComputerPicker value={value.computer_ids ?? []} onChange={(ids) => set({ computer_ids: ids })} disabled={disabled} />
            </Field>
            <Field label="Computer groups">
              <GroupPicker value={value.group_ids ?? []} onChange={(ids) => set({ group_ids: ids })} disabled={disabled} />
            </Field>
          </div>
        )}
      </FormSection>
      <FormSection title="Exclusions" description="Computers and groups that are always excluded, even if they match a target above.">
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="Excluded computers">
            <ComputerPicker value={value.excluded_computer_ids ?? []} onChange={(ids) => set({ excluded_computer_ids: ids })} disabled={disabled} />
          </Field>
          <Field label="Excluded groups">
            <GroupPicker value={value.excluded_group_ids ?? []} onChange={(ids) => set({ excluded_group_ids: ids })} disabled={disabled} />
          </Field>
        </div>
      </FormSection>
    </div>
  )
}

/** Short human summary of a scope, e.g. "All computers · 2 exclusions". */
export function ScopeSummary({ scope }: { scope: Scope }) {
  const groups = useGroups()
  const byId = new Map((groups.data?.items ?? []).map((g) => [g.id, g.name]))
  const parts: string[] = []
  if (scope.all_computers) parts.push('All computers')
  else {
    if (scope.computer_ids?.length) parts.push(`${scope.computer_ids.length} computer${scope.computer_ids.length === 1 ? '' : 's'}`)
    if (scope.group_ids?.length) {
      const names = scope.group_ids.map((g) => byId.get(g) ?? '…')
      parts.push(names.length <= 2 ? names.join(', ') : `${scope.group_ids.length} groups`)
    }
    if (parts.length === 0) parts.push('No targets')
  }
  const ex = (scope.excluded_computer_ids?.length ?? 0) + (scope.excluded_group_ids?.length ?? 0)
  return (
    <span className="text-sm">
      {parts.join(' · ')}
      {ex > 0 && <span className="text-ink-muted"> · {ex} excluded</span>}
    </span>
  )
}
