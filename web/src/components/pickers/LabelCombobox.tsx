import { useMemo, useState } from 'react'
import { Check, ChevronsUpDown, X } from 'lucide-react'
import { useAppLabels } from '../../api/queries'
import { Input } from '../ui/Input'
import { Spinner } from '../ui/Spinner'
import { cn } from '../../lib/cn'

const MAX = 60

function match(list: string[], needle: string, max: number): string[] {
  if (!needle) return list.slice(0, max)
  const starts: string[] = []
  const contains: string[] = []
  for (const l of list) {
    const low = l.toLowerCase()
    if (low.startsWith(needle)) starts.push(l)
    else if (low.includes(needle)) contains.push(l)
    if (starts.length >= max) break
  }
  return [...starts, ...contains].slice(0, max)
}

/** Every label a Mac App or policy step may use: custom labels ("custom:<key>") and shipped Installomator labels. */
export function allLabels(data: { labels: string[]; custom_labels: string[] | null } | undefined): string[] {
  return data ? [...(data.custom_labels ?? []), ...data.labels] : []
}

/**
 * Single-select combobox over custom labels (listed first, under "Custom") and the ~1000 shipped Installomator labels.
 * Type to filter; matches starting with the query sort first.
 */
export function LabelCombobox({ value, onChange, disabled, className, placeholder }: { value: string; onChange: (label: string) => void; disabled?: boolean; className?: string; placeholder?: string }) {
  const labels = useAppLabels()
  const [q, setQ] = useState('')
  const [open, setOpen] = useState(false)
  const shipped = labels.data?.labels ?? []
  const custom = labels.data?.custom_labels ?? []
  const needle = q.trim().toLowerCase()
  const filteredCustom = useMemo(() => match(custom, needle, MAX), [custom, needle])
  const filtered = useMemo(() => match(shipped, needle, MAX), [shipped, needle])
  const total = needle ? shipped.filter((l) => l.toLowerCase().includes(needle)).length : shipped.length
  const valid = !value || !labels.data || shipped.includes(value) || custom.includes(value)
  const all = shipped

  const option = (l: string, display = l) => (
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
      <span className="min-w-0 flex-1 truncate">{display}</span>
      {l === value && <Check className="size-4 shrink-0" />}
    </button>
  )

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
          placeholder={labels.isPending ? 'Loading labels…' : placeholder ?? 'Type to search labels, e.g. googlechrome'}
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
      {!valid && <p className="mt-1 text-xs text-critical">“{value}” is not a known Installomator or custom label.</p>}
      {open && !disabled && (
        <div className="absolute z-30 mt-1 w-full max-h-72 overflow-y-auto rounded-xl border border-line bg-surface-raised shadow-xl">
          {labels.isPending ? (
            <div className="flex items-center gap-2 px-3 py-3 text-xs text-ink-muted">
              <Spinner className="size-3.5" /> Loading {all.length || ''} labels…
            </div>
          ) : filtered.length === 0 && filteredCustom.length === 0 ? (
            <p className="px-3 py-3 text-xs text-ink-muted">No labels match “{q}”.</p>
          ) : (
            <>
              {filteredCustom.length > 0 && (
                <>
                  <p className="micro-label px-3 pb-1 pt-2 text-ink-faint">Custom</p>
                  {filteredCustom.map((l) => option(l))}
                  {filtered.length > 0 && <p className="micro-label border-t border-line px-3 pb-1 pt-2 text-ink-faint">Installomator</p>}
                </>
              )}
              {filtered.map((l) => option(l))}
              {total > filtered.length && <p className="border-t border-line px-3 py-1.5 text-[11px] text-ink-faint">Showing {filtered.length} of {total.toLocaleString()}. Keep typing to narrow.</p>}
            </>
          )}
        </div>
      )}
    </div>
  )
}
