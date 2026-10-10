import { useState } from 'react'
import { Laptop, X } from 'lucide-react'
import { useComputers } from '../../api/queries'
import { useDebounced } from '../../lib/hooks'
import { cn } from '../../lib/cn'
import { SearchInput } from '../ui/SearchInput'
import { Spinner } from '../ui/Spinner'

/** Compact single-select computer filter: a search box that collapses to a removable chip once a computer is chosen. */
export function ComputerSelect({ value, onChange, placeholder = 'All computers', className }: { value: string; onChange: (id: string) => void; placeholder?: string; className?: string }) {
  const [q, setQ] = useState('')
  const dq = useDebounced(q, 250)
  const [open, setOpen] = useState(false)
  const selected = useComputers({ ids: value ? [value] : [], limit: 1 }, { enabled: !!value })
  const results = useComputers({ q: dq, limit: 20, sort: 'name' }, { enabled: open && !value })

  if (value) {
    const c = selected.data?.items?.find((x) => x.id === value)
    return (
      <span className={cn('inline-flex h-[34px] items-center gap-1.5 rounded-md border border-line-strong/70 bg-surface-sunken pl-2.5 pr-1 text-[13px] text-ink', className)}>
        <Laptop className="size-3.5 shrink-0 text-ink-faint" />
        <span className="min-w-0 flex-1 truncate">{c ? c.name || c.hostname || c.serial_number : selected.isPending ? '…' : value.slice(0, 8)}</span>
        <button type="button" onClick={() => onChange('')} className="rounded p-1 text-ink-faint hover:bg-line hover:text-ink" aria-label="Clear computer filter">
          <X className="size-3.5" />
        </button>
      </span>
    )
  }
  const items = results.data?.items ?? []
  return (
    <div className={cn('relative', className)}>
      <SearchInput value={q} onChange={setQ} placeholder={placeholder} onFocus={() => setOpen(true)} onBlur={() => setTimeout(() => setOpen(false), 150)} aria-label="Filter by computer" />
      {open && (
        <div className="absolute z-30 mt-1 max-h-64 w-full min-w-[16rem] overflow-y-auto rounded-xl border border-line bg-surface-raised shadow-xl">
          {results.isPending ? (
            <div className="flex items-center gap-2 px-3 py-3 text-xs text-ink-muted">
              <Spinner className="size-3.5" /> Searching…
            </div>
          ) : items.length === 0 ? (
            <p className="px-3 py-3 text-xs text-ink-muted">No matching computers.</p>
          ) : (
            items.map((c) => (
              <button
                key={c.id}
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  onChange(c.id)
                  setQ('')
                  setOpen(false)
                }}
                className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-surface-sunken"
              >
                <Laptop className="size-4 shrink-0 text-ink-faint" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-ink">{c.name || c.hostname || c.serial_number}</span>
                  <span className="block truncate font-mono text-xs text-ink-muted">{c.serial_number}</span>
                </span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}
