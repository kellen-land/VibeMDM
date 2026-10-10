import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'

export interface DLItem {
  label: ReactNode
  value: ReactNode
  mono?: boolean
  span?: boolean
}

export function DescriptionList({ items, cols = 2, className }: { items: DLItem[]; cols?: 1 | 2 | 3; className?: string }) {
  return (
    <dl className={cn('grid gap-x-6 gap-y-3', cols === 1 ? 'grid-cols-1' : cols === 2 ? 'grid-cols-1 sm:grid-cols-2' : 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3', className)}>
      {items.map((it, i) => (
        <div key={i} className={cn('min-w-0', it.span && 'sm:col-span-full')}>
          <dt className="text-xs text-ink-muted">{it.label}</dt>
          <dd className={cn('mt-0.5 text-sm text-ink break-words', it.mono && 'font-mono text-xs')}>{it.value === '' || it.value === null || it.value === undefined ? <span className="text-ink-faint">—</span> : it.value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function KeyValueTable({ data, className }: { data: Record<string, unknown> | null | undefined; className?: string }) {
  const entries = Object.entries(data ?? {}).sort(([a], [b]) => a.localeCompare(b))
  if (entries.length === 0) return <p className="text-sm text-ink-faint">No data reported.</p>
  return (
    <div className={cn('overflow-x-auto rounded-lg border border-line', className)}>
      <table className="w-full text-sm">
        <tbody>
          {entries.map(([k, v]) => (
            <tr key={k} className="border-b border-line last:border-b-0">
              <td className="px-3 py-1.5 font-mono text-xs text-ink-muted whitespace-nowrap align-top w-1/3">{k}</td>
              <td className="px-3 py-1.5 font-mono text-xs text-ink break-all">{renderValue(v)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function renderValue(v: unknown): ReactNode {
  if (v === null || v === undefined) return <span className="text-ink-faint">null</span>
  if (typeof v === 'boolean') return v ? 'true' : 'false'
  if (typeof v === 'object') return <pre className="whitespace-pre-wrap">{JSON.stringify(v, null, 2)}</pre>
  return String(v)
}
