import { useMemo, useState, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { SearchInput } from '../ui/SearchInput'
import { cn } from '../../lib/cn'

export interface NamedOption {
  /** Value stored in the selection (an ID or a name, depending on the picker). */
  value: string
  label: string
  hint?: string
}

export interface NamedPickerProps {
  value: string[]
  onChange: (v: string[]) => void
  options: NamedOption[]
  loading?: boolean
  disabled?: boolean
  className?: string
  icon?: ReactNode
  placeholder?: string
  /** Shown when there are no options at all (e.g. "No buildings defined yet."). */
  emptyText?: string
}

/** Multi-select over a small, fully loaded option list (buildings, departments, network segments). Mirrors GroupPicker. */
export function NamedPicker({ value, onChange, options, loading, disabled, className, icon, placeholder = 'Search…', emptyText = 'Nothing to pick from.' }: NamedPickerProps) {
  const [q, setQ] = useState('')
  const [open, setOpen] = useState(false)
  const byValue = useMemo(() => new Map(options.map((o) => [o.value, o])), [options])
  const chosen = useMemo(() => new Set(value), [value])
  const filtered = options.filter((o) => !chosen.has(o.value) && (!q || o.label.toLowerCase().includes(q.toLowerCase()) || (o.hint ?? '').toLowerCase().includes(q.toLowerCase())))

  return (
    <div className={cn('flex flex-col gap-2', className)}>
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {value.map((v) => {
            const o = byValue.get(v)
            return (
              <span key={v} className="inline-flex items-center gap-1.5 rounded-full border border-line bg-surface-sunken pl-2.5 pr-1 py-0.5 text-xs text-ink">
                {icon && <span className="text-ink-faint [&>svg]:size-3">{icon}</span>}
                <span className="max-w-[220px] truncate">{o?.label ?? (loading ? '…' : v)}</span>
                {o?.hint && <span className="text-[10px] text-ink-faint font-mono">{o.hint}</span>}
                {!disabled && (
                  <button type="button" onClick={() => onChange(value.filter((x) => x !== v))} className="rounded-full p-0.5 text-ink-faint hover:bg-line hover:text-ink" aria-label="Remove">
                    <X className="size-3" />
                  </button>
                )}
              </span>
            )
          })}
        </div>
      )}
      {!disabled && (
        <div className="relative">
          <SearchInput value={q} onChange={setQ} placeholder={placeholder} onFocus={() => setOpen(true)} onBlur={() => setTimeout(() => setOpen(false), 150)} />
          {open && (
            <div className="absolute z-30 mt-1 w-full max-h-64 overflow-y-auto rounded-xl border border-line bg-surface-raised shadow-xl">
              {filtered.length === 0 ? (
                <p className="px-3 py-3 text-xs text-ink-muted">{loading ? 'Loading…' : options.length === 0 ? emptyText : 'No matches.'}</p>
              ) : (
                filtered.map((o) => (
                  <button
                    key={o.value}
                    type="button"
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => onChange([...value, o.value])}
                    className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-surface-sunken"
                  >
                    {icon && <span className="text-ink-faint shrink-0 [&>svg]:size-4">{icon}</span>}
                    <span className="min-w-0 flex-1 truncate text-ink">{o.label}</span>
                    {o.hint && <span className="text-xs text-ink-muted font-mono">{o.hint}</span>}
                  </button>
                ))
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
