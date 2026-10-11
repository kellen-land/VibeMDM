import { useMemo, useState } from 'react'
import { Smartphone, X } from 'lucide-react'
import { useMobileDevices } from '../../api/queries'
import { useDebounced } from '../../lib/hooks'
import { SearchInput } from '../ui/SearchInput'
import { Spinner } from '../ui/Spinner'
import { cn } from '../../lib/cn'
import type { MobileDevice } from '../../api/types'

export interface MobileDevicePickerProps {
  value: string[]
  onChange: (ids: string[]) => void
  disabled?: boolean
  placeholder?: string
  className?: string
  /** Hide these IDs from the result list. */
  exclude?: string[]
}

/** Multi-select mobile device picker: chips for the selection plus a searchable result list. */
export function MobileDevicePicker({ value, onChange, disabled, placeholder = 'Search mobile devices by name, serial, user…', className, exclude }: MobileDevicePickerProps) {
  const [q, setQ] = useState('')
  const dq = useDebounced(q, 250)
  const [open, setOpen] = useState(false)
  const selectedQ = useMobileDevices({ ids: value, limit: 500 }, { enabled: value.length > 0 })
  const results = useMobileDevices({ q: dq, limit: 20, sort: 'name' }, { enabled: open })
  const selectedMap = useMemo(() => new Map((selectedQ.data?.items ?? []).map((c) => [c.id, c])), [selectedQ.data])
  const excl = useMemo(() => new Set([...(exclude ?? []), ...value]), [exclude, value])

  const add = (c: MobileDevice) => {
    if (!value.includes(c.id)) onChange([...value, c.id])
  }
  const remove = (id: string) => onChange(value.filter((v) => v !== id))

  return (
    <div className={cn('flex flex-col gap-2', className)}>
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {value.map((id) => {
            const c = selectedMap.get(id)
            return (
              <span key={id} className="inline-flex items-center gap-1.5 rounded-full border border-line bg-surface-sunken pl-2.5 pr-1 py-0.5 text-xs text-ink">
                <Smartphone className="size-3 text-ink-faint" />
                <span className="max-w-[200px] truncate">{c ? c.name || c.serial_number : selectedQ.isPending ? '…' : id.slice(0, 8)}</span>
                {!disabled && (
                  <button type="button" onClick={() => remove(id)} className="rounded-full p-0.5 text-ink-faint hover:bg-line hover:text-ink" aria-label="Remove">
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
              {results.isPending ? (
                <div className="flex items-center gap-2 px-3 py-3 text-xs text-ink-muted">
                  <Spinner className="size-3.5" /> Searching…
                </div>
              ) : (results.data?.items ?? []).filter((c) => !excl.has(c.id)).length === 0 ? (
                <p className="px-3 py-3 text-xs text-ink-muted">No matching mobile devices.</p>
              ) : (
                (results.data?.items ?? [])
                  .filter((c) => !excl.has(c.id))
                  .map((c) => (
                    <button
                      key={c.id}
                      type="button"
                      onMouseDown={(e) => e.preventDefault()}
                      onClick={() => add(c)}
                      className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-surface-sunken"
                    >
                      <Smartphone className="size-4 text-ink-faint shrink-0" />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-ink">{c.name || c.serial_number}</span>
                        <span className="block truncate text-xs text-ink-muted font-mono">
                          {c.serial_number} {c.model_name && `· ${c.model_name}`}
                        </span>
                      </span>
                    </button>
                  ))
              )}
              {results.data && results.data.total > (results.data.items?.length ?? 0) && (
                <p className="border-t border-line px-3 py-1.5 text-[11px] text-ink-faint">Showing first {results.data.items.length} of {results.data.total}. Refine your search.</p>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
