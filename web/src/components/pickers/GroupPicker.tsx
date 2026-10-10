import { useMemo, useState } from 'react'
import { Users, X } from 'lucide-react'
import { useGroups } from '../../api/queries'
import { SearchInput } from '../ui/SearchInput'
import { Badge } from '../ui/Badge'
import { cn } from '../../lib/cn'

export interface GroupPickerProps {
  value: string[]
  onChange: (ids: string[]) => void
  disabled?: boolean
  className?: string
  exclude?: string[]
  /** Only show smart or static groups. */
  smart?: boolean
}

export function GroupPicker({ value, onChange, disabled, className, exclude, smart }: GroupPickerProps) {
  const groups = useGroups()
  const [q, setQ] = useState('')
  const [open, setOpen] = useState(false)
  const all = groups.data?.items ?? []
  const byId = useMemo(() => new Map(all.map((g) => [g.id, g])), [all])
  const excl = useMemo(() => new Set([...(exclude ?? []), ...value]), [exclude, value])
  const filtered = all.filter((g) => !excl.has(g.id) && (smart === undefined || g.is_smart === smart) && (!q || g.name.toLowerCase().includes(q.toLowerCase())))

  return (
    <div className={cn('flex flex-col gap-2', className)}>
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {value.map((id) => {
            const g = byId.get(id)
            return (
              <span key={id} className="inline-flex items-center gap-1.5 rounded-full border border-line bg-surface-sunken pl-2.5 pr-1 py-0.5 text-xs text-ink">
                <Users className="size-3 text-ink-faint" />
                <span className="max-w-[200px] truncate">{g?.name ?? (groups.isPending ? '…' : id.slice(0, 8))}</span>
                {g && (
                  <Badge tone={g.is_smart ? 'accent' : 'neutral'} className="px-1.5 py-0 text-[10px]">
                    {g.is_smart ? 'Smart' : 'Static'}
                  </Badge>
                )}
                {!disabled && (
                  <button type="button" onClick={() => onChange(value.filter((v) => v !== id))} className="rounded-full p-0.5 text-ink-faint hover:bg-line hover:text-ink" aria-label="Remove">
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
          <SearchInput value={q} onChange={setQ} placeholder="Search groups…" onFocus={() => setOpen(true)} onBlur={() => setTimeout(() => setOpen(false), 150)} />
          {open && (
            <div className="absolute z-30 mt-1 w-full max-h-64 overflow-y-auto rounded-xl border border-line bg-surface-raised shadow-xl">
              {filtered.length === 0 ? (
                <p className="px-3 py-3 text-xs text-ink-muted">{all.length === 0 ? 'No groups exist yet.' : 'No matching groups.'}</p>
              ) : (
                filtered.map((g) => (
                  <button
                    key={g.id}
                    type="button"
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => onChange([...value, g.id])}
                    className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-surface-sunken"
                  >
                    <Users className="size-4 text-ink-faint shrink-0" />
                    <span className="min-w-0 flex-1 truncate text-ink">{g.name}</span>
                    <Badge tone={g.is_smart ? 'accent' : 'neutral'}>{g.is_smart ? 'Smart' : 'Static'}</Badge>
                    <span className="text-xs text-ink-muted tabular-nums">{g.member_count}</span>
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
