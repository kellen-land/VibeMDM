import { useMemo, useState } from 'react'
import { Check, ChevronsUpDown, X } from 'lucide-react'
import { useAppLabels } from '../../api/queries'
import { Input } from '../ui/Input'
import { Spinner } from '../ui/Spinner'
import { cn } from '../../lib/cn'

const MAX = 60

/** Single-select combobox over the ~1000 Installomator labels. Type to filter; matches starting with the query sort first. */
export function LabelCombobox({ value, onChange, disabled, className }: { value: string; onChange: (label: string) => void; disabled?: boolean; className?: string }) {
  const labels = useAppLabels()
  const [q, setQ] = useState('')
  const [open, setOpen] = useState(false)
  const all = labels.data?.labels ?? []
  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase()
    if (!needle) return all.slice(0, MAX)
    const starts: string[] = []
    const contains: string[] = []
    for (const l of all) {
      const low = l.toLowerCase()
      if (low.startsWith(needle)) starts.push(l)
      else if (low.includes(needle)) contains.push(l)
      if (starts.length >= MAX) break
    }
    return [...starts, ...contains].slice(0, MAX)
  }, [all, q])
  const total = q.trim() ? all.filter((l) => l.toLowerCase().includes(q.trim().toLowerCase())).length : all.length
  const valid = !value || all.includes(value)

  return (
    <div className={cn('relative', className)}>
      <div className="relative">
        <Input
          value={open ? q : value}
          onChange={(e) => setQ(e.target.value)}
          onFocus={() => {
            setQ('')
            setOpen(true)
          }}
          onBlur={() => setTimeout(() => setOpen(false), 150)}
          disabled={disabled}
          mono
          placeholder={labels.isPending ? 'Loading labels…' : 'Type to search labels, e.g. googlechrome'}
          autoComplete="off"
          spellCheck={false}
          className={cn('pr-16', !valid && 'border-critical/60')}
          aria-autocomplete="list"
          aria-expanded={open}
        />
        <span className="absolute right-2 top-1/2 -translate-y-1/2 flex items-center gap-1 text-ink-faint">
          {value && !disabled && (
            <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => onChange('')} className="rounded p-0.5 hover:text-ink" aria-label="Clear label">
              <X className="size-3.5" />
            </button>
          )}
          {labels.isPending ? <Spinner className="size-3.5" /> : <ChevronsUpDown className="size-3.5" />}
        </span>
      </div>
      {!valid && <p className="mt-1 text-xs text-critical">“{value}” is not a known Installomator label.</p>}
      {open && !disabled && (
        <div className="absolute z-30 mt-1 w-full max-h-72 overflow-y-auto rounded-xl border border-line bg-surface-raised shadow-xl">
          {labels.isPending ? (
            <div className="flex items-center gap-2 px-3 py-3 text-xs text-ink-muted">
              <Spinner className="size-3.5" /> Loading {all.length || ''} labels…
            </div>
          ) : filtered.length === 0 ? (
            <p className="px-3 py-3 text-xs text-ink-muted">No labels match “{q}”.</p>
          ) : (
            <>
              {filtered.map((l) => (
                <button
                  key={l}
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => {
                    onChange(l)
                    setOpen(false)
                  }}
                  className={cn('flex w-full items-center gap-3 px-3 py-1.5 text-left text-sm font-mono hover:bg-surface-sunken', l === value && 'text-accent-700 dark:text-accent-300')}
                >
                  <span className="min-w-0 flex-1 truncate">{l}</span>
                  {l === value && <Check className="size-4 shrink-0" />}
                </button>
              ))}
              {total > filtered.length && <p className="border-t border-line px-3 py-1.5 text-[11px] text-ink-faint">Showing {filtered.length} of {total.toLocaleString()}. Keep typing to narrow.</p>}
            </>
          )}
        </div>
      )}
    </div>
  )
}
