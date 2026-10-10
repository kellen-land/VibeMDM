import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from './Button'
import { Select } from './Input'

export function Pagination({
  total,
  limit,
  offset,
  onChange,
  pageSizes = [25, 50, 100, 200],
}: {
  total: number
  limit: number
  offset: number
  onChange: (next: { limit: number; offset: number }) => void
  pageSizes?: number[]
}) {
  if (total <= 0) return null
  const page = Math.floor(offset / limit) + 1
  const pages = Math.max(1, Math.ceil(total / limit))
  const from = offset + 1
  const to = Math.min(total, offset + limit)
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-3 border-t border-line text-xs text-ink-muted">
      <div className="flex items-center gap-2">
        <span>
          Showing <span className="text-ink tabular-nums">{from.toLocaleString()}–{to.toLocaleString()}</span> of <span className="text-ink tabular-nums">{total.toLocaleString()}</span>
        </span>
        <Select
          className="h-7 w-auto text-xs py-0"
          value={String(limit)}
          onChange={(e) => onChange({ limit: Number(e.target.value), offset: 0 })}
          options={pageSizes.map((n) => ({ value: String(n), label: `${n} / page` }))}
          aria-label="Rows per page"
        />
      </div>
      <div className="flex items-center gap-1">
        <Button size="xs" variant="ghost" disabled={page <= 1} onClick={() => onChange({ limit, offset: Math.max(0, offset - limit) })} icon={<ChevronLeft />}>
          Prev
        </Button>
        <span className="px-2 tabular-nums">
          Page {page} of {pages}
        </span>
        <Button size="xs" variant="ghost" disabled={page >= pages} onClick={() => onChange({ limit, offset: offset + limit })} iconRight={<ChevronRight />}>
          Next
        </Button>
      </div>
    </div>
  )
}
