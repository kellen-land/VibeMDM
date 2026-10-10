import { useMemo } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import type { Criterion, CriterionField } from '../../api/types'
import { useCriteriaFields, useGroups } from '../../api/queries'
import { Button, IconButton } from '../ui/Button'
import { Input, Select } from '../ui/Input'
import { cn } from '../../lib/cn'

export const operatorLabels: Record<string, string> = {
  is: 'is',
  is_not: 'is not',
  like: 'contains',
  not_like: 'does not contain',
  matches_regex: 'matches regex',
  has_value: 'has a value',
  has_no_value: 'has no value',
  greater_than: 'greater than',
  less_than: 'less than',
  greater_or_equal: 'greater than or equal to',
  less_or_equal: 'less than or equal to',
  more_than_x_days_ago: 'more than X days ago',
  less_than_x_days_ago: 'less than X days ago',
  before_date: 'before (YYYY-MM-DD)',
  after_date: 'after (YYYY-MM-DD)',
  member_of: 'member of',
  not_member_of: 'not member of',
}

const noValueOps = new Set(['has_value', 'has_no_value'])
const daysOps = new Set(['more_than_x_days_ago', 'less_than_x_days_ago'])
const dateOps = new Set(['before_date', 'after_date'])

export function newCriterion(fields?: CriterionField[]): Criterion {
  const f = fields?.[0]
  return { and_or: 'and', field: f?.key ?? 'computer_name', operator: f?.operators[0] ?? 'is', value: '', open_paren: false, close_paren: false }
}

function groupOf(f: CriterionField): string {
  if (f.key.startsWith('ea:')) return 'Extension attributes'
  if (f.key.startsWith('application_')) return 'Applications'
  if (f.key.startsWith('profile_')) return 'Profiles'
  if (f.key === 'local_username') return 'Local users'
  if (f.key === 'group_membership') return 'Groups'
  if (['filevault_enabled', 'sip_enabled', 'firewall_enabled', 'gatekeeper_enabled', 'is_supervised', 'activation_lock_enabled', 'secure_boot_level'].includes(f.key)) return 'Security'
  if (['mdm_enrolled', 'agent_enrolled', 'agent_version', 'last_checkin', 'last_inventory', 'mdm_enrolled_at', 'agent_enrolled_at', 'created_at'].includes(f.key)) return 'Management'
  if (['last_user', 'assigned_user', 'assigned_email', 'department', 'building', 'site', 'asset_tag'].includes(f.key)) return 'User & location'
  if (['model_name', 'model_identifier', 'cpu', 'cpu_cores', 'memory_mb', 'disk_total_mb', 'disk_free_mb', 'architecture'].includes(f.key)) return 'Hardware'
  if (['os_version', 'os_build'].includes(f.key)) return 'Operating system'
  return 'General'
}

export function CriteriaBuilder({ value, onChange, disabled }: { value: Criterion[]; onChange: (c: Criterion[]) => void; disabled?: boolean }) {
  const fieldsQ = useCriteriaFields()
  const groups = useGroups()
  const fields = fieldsQ.data ?? []
  const byKey = useMemo(() => new Map(fields.map((f) => [f.key, f])), [fields])
  const grouped = useMemo(() => {
    const m = new Map<string, CriterionField[]>()
    for (const f of fields) {
      const g = groupOf(f)
      if (!m.has(g)) m.set(g, [])
      m.get(g)!.push(f)
    }
    return [...m.entries()]
  }, [fields])

  const update = (i: number, patch: Partial<Criterion>) => {
    const next = value.map((c, j) => (j === i ? { ...c, ...patch } : c))
    onChange(next)
  }
  const setField = (i: number, key: string) => {
    const f = byKey.get(key)
    const cur = value[i]
    const op = f && f.operators.includes(cur.operator) ? cur.operator : (f?.operators[0] ?? 'is')
    update(i, { field: key, operator: op, value: f?.type === 'bool' ? 'true' : '' })
  }
  const remove = (i: number) => onChange(value.filter((_, j) => j !== i))
  const add = () => onChange([...value, newCriterion(fields)])

  return (
    <div className="flex flex-col gap-2">
      {value.length === 0 && <p className="text-sm text-ink-muted">No criteria yet. A smart group with no criteria matches no computers.</p>}
      {value.map((c, i) => {
        const f = byKey.get(c.field)
        const ops = f?.operators ?? ['is', 'is_not']
        const showValue = !noValueOps.has(c.operator)
        return (
          <div key={i} className="grid grid-cols-[auto_auto_1fr_auto] gap-2 items-center rounded-lg border border-line bg-surface-sunken/40 p-2 sm:grid-cols-[72px_28px_minmax(160px,1.2fr)_minmax(150px,1fr)_minmax(140px,1fr)_28px_auto]">
            <div>
              {i === 0 ? (
                <span className="inline-flex h-9 items-center px-2 text-xs font-medium text-ink-muted">Where</span>
              ) : (
                <Select className="h-9 text-xs" value={c.and_or} onChange={(e) => update(i, { and_or: e.target.value as 'and' | 'or' })} disabled={disabled} options={[{ value: 'and', label: 'and' }, { value: 'or', label: 'or' }]} />
              )}
            </div>
            <ParenToggle active={c.open_paren} onClick={() => update(i, { open_paren: !c.open_paren })} disabled={disabled} label="(" />
            <Select value={c.field} onChange={(e) => setField(i, e.target.value)} disabled={disabled || fieldsQ.isPending} className="col-span-2 sm:col-span-1">
              {!f && c.field && <option value={c.field}>{c.field}</option>}
              {grouped.map(([g, fs]) => (
                <optgroup key={g} label={g}>
                  {fs.map((fl) => (
                    <option key={fl.key} value={fl.key}>
                      {fl.label}
                    </option>
                  ))}
                </optgroup>
              ))}
            </Select>
            <Select value={c.operator} onChange={(e) => update(i, { operator: e.target.value, value: noValueOps.has(e.target.value) ? '' : c.value })} disabled={disabled} className="col-span-2 sm:col-span-1">
              {ops.map((o) => (
                <option key={o} value={o}>
                  {operatorLabels[o] ?? o}
                </option>
              ))}
            </Select>
            <div className="col-span-3 sm:col-span-1">
              {showValue ? (
                f?.type === 'bool' ? (
                  <Select value={c.value || 'true'} onChange={(e) => update(i, { value: e.target.value })} disabled={disabled} options={[{ value: 'true', label: 'True' }, { value: 'false', label: 'False' }]} />
                ) : f?.type === 'group' ? (
                  <Select value={c.value} onChange={(e) => update(i, { value: e.target.value })} disabled={disabled}>
                    <option value="">Select a group…</option>
                    {(groups.data?.items ?? []).map((g) => (
                      <option key={g.id} value={g.id}>
                        {g.name}
                      </option>
                    ))}
                  </Select>
                ) : daysOps.has(c.operator) ? (
                  <Input type="number" min={0} value={c.value} onChange={(e) => update(i, { value: e.target.value })} placeholder="Days" disabled={disabled} />
                ) : dateOps.has(c.operator) ? (
                  <Input type="date" value={c.value} onChange={(e) => update(i, { value: e.target.value })} disabled={disabled} />
                ) : (
                  <Input type={f?.type === 'number' ? 'number' : 'text'} value={c.value} onChange={(e) => update(i, { value: e.target.value })} placeholder="Value" disabled={disabled} />
                )
              ) : (
                <span className="inline-flex h-9 items-center px-2 text-xs text-ink-faint">no value needed</span>
              )}
            </div>
            <ParenToggle active={c.close_paren} onClick={() => update(i, { close_paren: !c.close_paren })} disabled={disabled} label=")" />
            <IconButton label="Remove criterion" variant="ghost" onClick={() => remove(i)} disabled={disabled}>
              <Trash2 />
            </IconButton>
          </div>
        )
      })}
      {!disabled && (
        <div>
          <Button size="sm" variant="secondary" icon={<Plus />} onClick={add} disabled={fieldsQ.isPending}>
            Add criterion
          </Button>
        </div>
      )}
    </div>
  )
}

function ParenToggle({ active, onClick, disabled, label }: { active: boolean; onClick: () => void; disabled?: boolean; label: string }) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={label === '(' ? 'Open parenthesis' : 'Close parenthesis'}
      className={cn(
        'h-9 w-7 rounded-md border text-sm font-mono transition-colors focus-ring disabled:opacity-50',
        active ? 'border-accent-500 bg-accent-100 text-accent-800 dark:bg-accent-900/50 dark:text-accent-100' : 'border-line text-ink-faint hover:border-line-strong hover:text-ink',
      )}
    >
      {label}
    </button>
  )
}

/** Human-readable one-line summary of criteria. */
export function describeCriteria(criteria: Criterion[] | null | undefined, fields?: CriterionField[]): string {
  if (!criteria || criteria.length === 0) return 'No criteria'
  const byKey = new Map((fields ?? []).map((f) => [f.key, f.label]))
  return criteria
    .map((c, i) => {
      const parts = [i > 0 ? c.and_or.toUpperCase() : '', c.open_paren ? '(' : '', byKey.get(c.field) ?? c.field, operatorLabels[c.operator] ?? c.operator, noValueOps.has(c.operator) ? '' : JSON.stringify(c.value), c.close_paren ? ')' : '']
      return parts.filter(Boolean).join(' ')
    })
    .join(' ')
}
