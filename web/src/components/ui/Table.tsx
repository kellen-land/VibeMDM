import type { ReactNode } from 'react'
import { ChevronDown, ChevronUp, ChevronsUpDown } from 'lucide-react'
import { cn } from '../../lib/cn'
import { Spinner } from './Spinner'
import { EmptyState } from './EmptyState'

export interface Column<T> {
  key: string
  header: ReactNode
  render: (row: T) => ReactNode
  sortable?: boolean
  className?: string
  headerClassName?: string
  width?: string
  align?: 'left' | 'right' | 'center'
}

export interface SortState {
  key: string
  dir: 'asc' | 'desc'
}

export interface TableProps<T> {
  columns: Column<T>[]
  rows: T[] | undefined
  rowKey: (row: T) => string
  loading?: boolean
  error?: string | null
  emptyTitle?: ReactNode
  emptyDescription?: ReactNode
  emptyAction?: ReactNode
  onRowClick?: (row: T) => void
  sort?: SortState
  onSort?: (s: SortState) => void
  dense?: boolean
  className?: string
  footer?: ReactNode
  rowClassName?: (row: T) => string | undefined
  selected?: (row: T) => boolean
}

export function Table<T>({
  columns,
  rows,
  rowKey,
  loading,
  error,
  emptyTitle = 'Nothing here yet',
  emptyDescription,
  emptyAction,
  onRowClick,
  sort,
  onSort,
  dense,
  className,
  footer,
  rowClassName,
  selected,
}: TableProps<T>) {
  const toggleSort = (c: Column<T>) => {
    if (!c.sortable || !onSort) return
    if (sort?.key === c.key) onSort({ key: c.key, dir: sort.dir === 'asc' ? 'desc' : 'asc' })
    else onSort({ key: c.key, dir: 'asc' })
  }
  const empty = !loading && !error && rows && rows.length === 0
  return (
    <div className={cn('w-full', className)}>
      <div className="overflow-x-auto">
        <table className="w-full text-sm border-collapse">
          <thead>
            <tr className="border-b border-line text-left">
              {columns.map((c) => (
                <th
                  key={c.key}
                  style={c.width ? { width: c.width } : undefined}
                  className={cn(
                    'px-3 py-2 micro-label whitespace-nowrap select-none first:pl-4 last:pr-4',
                    c.align === 'right' && 'text-right',
                    c.align === 'center' && 'text-center',
                    c.sortable && onSort && 'cursor-pointer hover:text-ink',
                    c.headerClassName,
                  )}
                  onClick={() => toggleSort(c)}
                  aria-sort={sort?.key === c.key ? (sort.dir === 'asc' ? 'ascending' : 'descending') : undefined}
                >
                  <span className="inline-flex items-center gap-1">
                    {c.header}
                    {c.sortable && onSort && (
                      <span className="text-ink-faint [&>svg]:size-3.5">
                        {sort?.key === c.key ? sort.dir === 'asc' ? <ChevronUp /> : <ChevronDown /> : <ChevronsUpDown className="opacity-50" />}
                      </span>
                    )}
                  </span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {loading && (!rows || rows.length === 0) && (
              <tr>
                <td colSpan={columns.length} className="py-12 text-center text-ink-muted">
                  <span className="inline-flex items-center gap-2">
                    <Spinner className="size-4" /> Loading…
                  </span>
                </td>
              </tr>
            )}
            {error && (
              <tr>
                <td colSpan={columns.length} className="py-10 text-center text-critical text-sm">
                  {error}
                </td>
              </tr>
            )}
            {empty && (
              <tr>
                <td colSpan={columns.length}>
                  <EmptyState title={emptyTitle} description={emptyDescription} action={emptyAction} compact />
                </td>
              </tr>
            )}
            {rows?.map((r) => {
              const sel = selected?.(r)
              return (
                <tr
                  key={rowKey(r)}
                  onClick={onRowClick ? () => onRowClick(r) : undefined}
                  className={cn(
                    'border-b border-line last:border-b-0 transition-colors',
                    onRowClick && 'cursor-pointer hover:bg-surface-sunken/60',
                    sel && 'bg-accent-50 dark:bg-accent-950/40',
                    loading && 'opacity-60',
                    rowClassName?.(r),
                  )}
                >
                  {columns.map((c) => (
                    <td
                      key={c.key}
                      className={cn(
                        'px-3 align-middle first:pl-4 last:pr-4 text-[13px]',
                        dense ? 'py-1.5' : 'py-2',
                        c.align === 'right' && 'text-right',
                        c.align === 'center' && 'text-center',
                        c.className,
                      )}
                    >
                      {c.render(r)}
                    </td>
                  ))}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      {footer}
    </div>
  )
}
