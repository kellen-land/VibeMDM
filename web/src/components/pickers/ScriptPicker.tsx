import { useScripts, usePolicies, useProfiles } from '../../api/queries'
import { Select, type SelectProps } from '../ui/Input'

export function ScriptPicker({ value, onChange, ...rest }: Omit<SelectProps, 'value' | 'onChange' | 'options'> & { value: string; onChange: (id: string) => void }) {
  const scripts = useScripts()
  const items = scripts.data?.items ?? []
  return (
    <Select value={value} onChange={(e) => onChange(e.target.value)} {...rest}>
      <option value="">{scripts.isPending ? 'Loading scripts…' : items.length ? 'Select a script…' : 'No scripts available'}</option>
      {items.map((s) => (
        <option key={s.id} value={s.id}>
          {s.name}
          {s.category ? ` (${s.category})` : ''}
        </option>
      ))}
    </Select>
  )
}

export function PolicyPicker({ value, onChange, ...rest }: Omit<SelectProps, 'value' | 'onChange' | 'options'> & { value: string; onChange: (id: string) => void }) {
  const policies = usePolicies()
  const items = policies.data?.items ?? []
  return (
    <Select value={value} onChange={(e) => onChange(e.target.value)} {...rest}>
      <option value="">{policies.isPending ? 'Loading policies…' : items.length ? 'Select a policy…' : 'No policies available'}</option>
      {items.map((p) => (
        <option key={p.id} value={p.id}>
          {p.name}
          {!p.enabled ? ' (disabled)' : ''}
        </option>
      ))}
    </Select>
  )
}

export function ProfilePicker({ value, onChange, ...rest }: Omit<SelectProps, 'value' | 'onChange' | 'options'> & { value: string; onChange: (id: string) => void }) {
  const profiles = useProfiles()
  const items = profiles.data?.items ?? []
  return (
    <Select value={value} onChange={(e) => onChange(e.target.value)} {...rest}>
      <option value="">{profiles.isPending ? 'Loading profiles…' : items.length ? 'Select a profile…' : 'No profiles available'}</option>
      {items.map((p) => (
        <option key={p.id} value={p.id}>
          {p.name} — {p.identifier}
        </option>
      ))}
    </Select>
  )
}
