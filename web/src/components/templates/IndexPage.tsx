import { useMemo, useState, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Plus, X } from 'lucide-react'
import { PageHeader } from '../ui/PageHeader'
import { Card } from '../ui/Card'
import { Table, type Column, type SortState } from '../ui/Table'
import { Button } from '../ui/Button'
import { SearchInput } from '../ui/SearchInput'
import { Select } from '../ui/Input'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { useToast } from '../ui/Toast'
import { useAuth } from '../../auth/AuthContext'
import { errorMessage } from '../../api/client'
import { shortcutHint } from '../../lib/shortcuts'
import { cn } from '../../lib/cn'

/** A column, optionally sortable by a value. */
export interface IndexColumn<T> extends Column<T> {
  sortValue?: (row: T) => string | number | boolean | null | undefined
}

export interface IndexFilter<T> {
  key: string
  label: string
  options: { value: string; label: string }[]
  match: (row: T, value: string) => boolean
}

export interface BulkAction<T> {
  label: string
  icon?: ReactNode
  destructive?: boolean
  /** Ask first (always for destructive actions). */
  confirm?: { title: (n: number) => string; description?: string; confirmLabel?: string }
  /** Runs for every selected row; failures are counted and reported. */
  run: (row: T) => Promise<unknown>
  /** Toast after it finished, e.g. n => `${n} policies deleted`. */
  done: (n: number) => string
  /** Admins only (default true). */
  adminOnly?: boolean
}

export interface IndexPageProps<T> {
  title: ReactNode
  description?: ReactNode
  /** The one primary action: New. Shown to admins; ⌃N opens it. */
  newTo?: string
  newLabel?: string
  /** New opens a dialog instead of a page (e.g. Upload package). */
  onNew?: () => void
  /** Other page actions (secondary buttons), placed before New. */
  secondaryActions?: ReactNode
  /** Between the header and the list, e.g. tabs that switch between related lists. */
  above?: ReactNode
  rows: T[] | undefined
  loading?: boolean
  error?: unknown
  rowKey: (row: T) => string
  columns: IndexColumn<T>[]
  /** Opens the row's Details page. */
  rowHref?: (row: T) => string
  /** For objects edited in a dialog rather than on a Details page. */
  onRowClick?: (row: T) => void
  rowClassName?: (row: T) => string | undefined
  search?: { placeholder: string; text: (row: T) => string }
  filters?: IndexFilter<T>[]
  defaultSort?: SortState
  bulkActions?: BulkAction<T>[]
  /** Called after bulk actions so the page can refresh its queries. */
  onChanged?: () => Promise<unknown> | void
  empty: { title: string; description?: string; icon?: ReactNode }
  /** Under the list, e.g. attribution. */
  footer?: ReactNode
}

/**
 * The Index template (Polaris "resource index"): a title, one primary New action, search and
 * filters, a sortable table whose rows open the object's Details page, and bulk actions on
 * selected rows. Every list in the console uses it so they all behave the same.
 */
export function IndexPage<T>(p: IndexPageProps<T>) {
  const { isAdmin } = useAuth()
  const navigate = useNavigate()
  const toast = useToast()
  const [q, setQ] = useState('')
  const [filterValues, setFilterValues] = useState<Record<string, string>>({})
  const [sort, setSort] = useState<SortState | undefined>(p.defaultSort)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [pending, setPending] = useState<BulkAction<T> | null>(null)
  const [running, setRunning] = useState(false)

  const bulk = (p.bulkActions ?? []).filter((b) => isAdmin || b.adminOnly === false)
  const needle = q.trim().toLowerCase()
  const shown = useMemo(() => {
    let rows = (p.rows ?? []).filter((r) => !needle || (p.search?.text(r) ?? '').toLowerCase().includes(needle))
    for (const f of p.filters ?? []) {
      const v = filterValues[f.key]
      if (v) rows = rows.filter((r) => f.match(r, v))
    }
    const col = sort && p.columns.find((c) => c.key === sort.key)
    if (sort && col?.sortValue) {
      const get = col.sortValue
      rows = [...rows].sort((a, b) => {
        const x = get(a) ?? ''
        const y = get(b) ?? ''
        const d = typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y), undefined, { numeric: true, sensitivity: 'base' })
        return sort.dir === 'asc' ? d : -d
      })
    }
    return rows
  }, [p.rows, p.search, p.filters, p.columns, needle, filterValues, sort])

  const visibleKeys = shown.map(p.rowKey)
  const chosen = shown.filter((r) => selected.has(p.rowKey(r)))
  const allChosen = visibleKeys.length > 0 && visibleKeys.every((k) => selected.has(k))
  const filtered = !!needle || Object.values(filterValues).some(Boolean)

  const toggleAll = () => setSelected(allChosen ? new Set() : new Set(visibleKeys))
  const toggle = (k: string) =>
    setSelected((s) => {
      const n = new Set(s)
      if (n.has(k)) n.delete(k)
      else n.add(k)
      return n
    })

  const runBulk = async (a: BulkAction<T>) => {
    setRunning(true)
    let ok = 0
    const failures: string[] = []
    for (const r of chosen) {
      try {
        await a.run(r)
        ok++
      } catch (e) {
        failures.push(errorMessage(e))
      }
    }
    setRunning(false)
    setPending(null)
    setSelected(new Set())
    await p.onChanged?.()
    if (ok) toast.success(a.done(ok))
    if (failures.length) toast.error(`${failures.length} could not be changed`, failures[0])
  }

  const columns: Column<T>[] = [
    ...(bulk.length
      ? [
          {
            key: '__select',
            header: <input type="checkbox" aria-label="Select all" checked={allChosen} onChange={toggleAll} className="size-4 accent-accent-600" />,
            width: '40px',
            render: (r: T) => (
              <span onClick={(e) => e.stopPropagation()}>
                <input type="checkbox" aria-label="Select" checked={selected.has(p.rowKey(r))} onChange={() => toggle(p.rowKey(r))} className="size-4 accent-accent-600" />
              </span>
            ),
          },
        ]
      : []),
    ...p.columns.map((c) => ({ ...c, sortable: !!c.sortValue })),
  ]

  const newButton = (size?: 'sm') =>
    isAdmin &&
    (p.onNew ? (
      <Button variant="primary" size={size} icon={<Plus />} onClick={p.onNew} data-shortcut="new" title={shortcutHint('new', p.newLabel ?? 'New')}>
        {p.newLabel ?? 'New'}
      </Button>
    ) : (
      p.newTo && (
        <Link to={p.newTo} data-shortcut="new" title={shortcutHint('new', p.newLabel ?? 'New')}>
          <Button variant="primary" size={size} icon={<Plus />}>
            {p.newLabel ?? 'New'}
          </Button>
        </Link>
      )
    ))

  return (
    <>
      <PageHeader
        title={p.title}
        description={p.description}
        actions={
          (p.secondaryActions || p.newTo || p.onNew) && (
            <>
              {p.secondaryActions}
              {newButton()}
            </>
          )
        }
      />
      {p.above}
      <Card>
        {chosen.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2 border-b border-line bg-accent-500/[0.06] px-4 py-2.5">
            <span className="mr-2 text-sm font-medium text-ink tabular-nums">{chosen.length} selected</span>
            {bulk.map((b) => (
              <Button
                key={b.label}
                size="sm"
                variant={b.destructive ? 'danger-outline' : 'secondary'}
                icon={b.icon}
                loading={running && pending === b}
                onClick={() => (b.destructive || b.confirm ? setPending(b) : (setPending(b), void runBulk(b)))}
              >
                {b.label}
              </Button>
            ))}
            <Button size="sm" variant="ghost" icon={<X />} className="ml-auto" onClick={() => setSelected(new Set())}>
              Clear selection
            </Button>
          </div>
        ) : (
          (p.search || p.filters?.length) && (
            <div className="flex flex-wrap items-center gap-3 border-b border-line px-4 py-3">
              {p.search && <SearchInput value={q} onChange={setQ} placeholder={p.search.placeholder} className="w-full sm:w-72" />}
              {p.filters?.map((f) => (
                <div key={f.key} className="w-full shrink-0 sm:w-44">
                  <Select
                    value={filterValues[f.key] ?? ''}
                    onChange={(e) => setFilterValues((v) => ({ ...v, [f.key]: e.target.value }))}
                    aria-label={f.label}
                    className={cn(filterValues[f.key] && 'border-accent-500/60')}
                    options={[{ value: '', label: f.label }, ...f.options]}
                  />
                </div>
              ))}
              {filtered && (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setQ('')
                    setFilterValues({})
                  }}
                >
                  Clear filters
                </Button>
              )}
              <span className="ml-auto text-xs text-ink-muted tabular-nums">
                {filtered ? `${shown.length} of ${p.rows?.length ?? 0}` : `${p.rows?.length ?? 0}`}
              </span>
            </div>
          )
        )}
        <Table<T>
          rows={shown}
          loading={p.loading}
          error={p.error ? errorMessage(p.error) : null}
          rowKey={p.rowKey}
          onRowClick={p.rowHref ? (r) => navigate(p.rowHref!(r)) : p.onRowClick}
          rowClassName={p.rowClassName}
          selected={(r) => selected.has(p.rowKey(r))}
          sort={sort}
          onSort={setSort}
          emptyTitle={filtered ? 'Nothing matches' : p.empty.title}
          emptyDescription={filtered ? 'Try another search or clear the filters.' : p.empty.description}
          emptyAction={filtered ? undefined : newButton('sm')}
          columns={columns}
        />
      </Card>
      {p.footer}
      <ConfirmDialog
        open={!!pending && (pending.destructive || !!pending.confirm)}
        onClose={() => !running && setPending(null)}
        onConfirm={() => (pending ? runBulk(pending) : undefined)}
        destructive={pending?.destructive}
        loading={running}
        title={pending?.confirm?.title(chosen.length) ?? `${pending?.label} ${chosen.length} selected?`}
        description={pending?.confirm?.description}
        confirmLabel={pending?.confirm?.confirmLabel ?? pending?.label}
      />
    </>
  )
}
