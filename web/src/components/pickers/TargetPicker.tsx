import { Field } from '../ui/Field'
import { ComputerPicker } from './ComputerPicker'
import { GroupPicker } from './GroupPicker'

export interface Targets {
  computer_ids: string[]
  group_ids: string[]
}

/** Pick computers and/or groups for an ad-hoc run (scripts, policies). */
export function TargetPicker({ value, onChange, disabled }: { value: Targets; onChange: (t: Targets) => void; disabled?: boolean }) {
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Field label="Computers">
        <ComputerPicker value={value.computer_ids} onChange={(ids) => onChange({ ...value, computer_ids: ids })} disabled={disabled} />
      </Field>
      <Field label="Computer groups">
        <GroupPicker value={value.group_ids} onChange={(ids) => onChange({ ...value, group_ids: ids })} disabled={disabled} />
      </Field>
    </div>
  )
}

export const emptyTargets = (): Targets => ({ computer_ids: [], group_ids: [] })
