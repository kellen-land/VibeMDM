import { Fragment, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowUp, ChevronDown, ChevronRight } from 'lucide-react'
import { absoluteTime, parseDate, timeWithMs } from '../../lib/format'
import { cn } from '../../lib/cn'
import { badgeToneClass, type BadgeTone } from '../ui/Badge'
import { Button } from '../ui/Button'
import { CodeBlock } from '../ui/CopyButton'
import { EmptyState } from '../ui/EmptyState'
import { Spinner } from '../ui/Spinner'
import { Highlight } from '../activity/CommandSearch'
import type { LogStream } from './useLogStream'

export function normalizeLevel(level: string): 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' {
  const l = level.toUpperCase()
  if (l === 'DEBUG') return 'DEBUG'
  if (l === 'WARN' || l === 'WARNING') return 'WARN'
  if (l === 'ERROR') return 'ERROR'
  return 'INFO'
}

const levelTone: Record<ReturnType<typeof normalizeLevel>, BadgeTone> = { DEBUG: 'neutral', INFO: 'accent', WARN: 'warning', ERROR: 'critical' }

export function LevelBadge({ level }: { level: string }) {
  const l = normalizeLevel(level)
  return (
    <span className={cn('inline-flex w-[46px] justify-center rounded border px-1 font-mono text-[10px] font-semibold leading-4 tracking-wide', badgeToneClass[levelTone[l]], l === 'DEBUG' && 'opacity-75')}>
      {l}
    </span>
  )
}

function attrText(v: unknown): string {
  if (v === null || v === undefined) return String(v)
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

function truncate(s: string, n: number): string {
  return s.length > n ? `${s.slice(0, n)}…` : s
}

const MAX_CHIPS = 8

function AttrChips({ attrs, q }: { attrs: Record<string, unknown> | null; q: string }) {
  const entries = Object.entries(attrs ?? {})
  if (entries.length === 0) return null
  const shown = entries.slice(0, MAX_CHIPS)
  return (
    <span className="mt-0.5 flex flex-wrap gap-1">
      {shown.map(([k, v]) => (
        <span key={k} className="inline-flex max-w-[28rem] items-baseline rounded border border-line bg-surface-sunken px-1 font-mono text-[11px] leading-4">
          <span className="text-ink-faint">{k}=</span>
          <span className="truncate text-ink-muted">
            <Highlight text={truncate(attrText(v), 120)} q={q} />
          </span>
        </span>
      ))}
      {entries.length > MAX_CHIPS && <span className="font-mono text-[11px] leading-4 text-ink-faint">+{entries.length - MAX_CHIPS} more</span>}
    </span>
  )
}

function isToday(d: Date): boolean {
  const n = new Date()
  return d.getFullYear() === n.getFullYear() && d.getMonth() === n.getMonth() && d.getDate() === n.getDate()
}

function TimeCell({ at }: { at: string }) {
  const d = parseDate(at)
  return (
    <span className="whitespace-nowrap font-mono text-[11.5px] tabular-nums text-ink-muted" title={absoluteTime(at)}>
      {d && !isToday(d) && <span className="mr-1 text-ink-faint">{d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}</span>}
      {timeWithMs(at)}
    </span>
  )
}

/**
 * Dense, newest-first log table in its own scroll container. When the viewer scrolls away from the top, live-tail
 * prepends keep the visible rows in place and a "N new" pill offers a jump back.
 */
export function LogTable({ stream, q, showComputer, live, height = '68vh' }: { stream: LogStream; q: string; showComputer: boolean; live: boolean; height?: string }) {
  const { rows, loading, error, hasOlder, loadingOlder, loadOlder, prependSeq, prepended } = stream
  const scrollRef = useRef<HTMLDivElement>(null)
  const lastHeight = useRef(0)
  const pinnedRef = useRef(true)
  const [newCount, setNewCount] = useState(0)
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set())

  // Keep the viewport stable when rows are prepended while the user is reading further down.
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el || prependSeq === 0) return
    if (!pinnedRef.current) {
      el.scrollTop += el.scrollHeight - lastHeight.current
      setNewCount((n) => n + prepended)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [prependSeq])
  useLayoutEffect(() => {
    if (scrollRef.current) lastHeight.current = scrollRef.current.scrollHeight
  })
  useEffect(() => {
    if (!live) setNewCount(0)
  }, [live])

  const onScroll = () => {
    const el = scrollRef.current
    if (!el) return
    const pinned = el.scrollTop < 16
    pinnedRef.current = pinned
    if (pinned) setNewCount(0)
  }
  const jumpTop = () => {
    scrollRef.current?.scrollTo({ top: 0, behavior: 'smooth' })
    setNewCount(0)
  }
  const toggle = (id: number) =>
    setExpanded((s) => {
      const n = new Set(s)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })

  const cols = showComputer ? 6 : 5
  return (
    <div className="relative">
      {newCount > 0 && (
        <button
          type="button"
          onClick={jumpTop}
          className="absolute left-1/2 top-9 z-20 inline-flex -translate-x-1/2 items-center gap-1.5 rounded-full border border-accent-500/40 bg-surface-raised px-3 py-1 text-xs font-medium text-ink shadow-lg hover:bg-surface-sunken focus-ring"
        >
          <ArrowUp className="size-3.5 text-accent-400" />
          {newCount.toLocaleString()} new
        </button>
      )}
      <div ref={scrollRef} onScroll={onScroll} className="overflow-auto [overflow-anchor:none]" style={{ maxHeight: height }}>
        <table className="w-full border-collapse text-[12.5px]">
          <thead className="sticky top-0 z-10 bg-surface">
            <tr className="border-b border-line text-left">
              <th className="w-6 py-2 pl-3" aria-label="Expand" />
              <th className="px-2 py-2 micro-label whitespace-nowrap">Time</th>
              <th className="px-2 py-2 micro-label">Level</th>
              <th className="px-2 py-2 micro-label">Component</th>
              {showComputer && <th className="px-2 py-2 micro-label">Computer</th>}
              <th className="px-2 py-2 pr-4 micro-label">Message</th>
            </tr>
          </thead>
          <tbody>
            {loading && (
              <tr>
                <td colSpan={cols} className="py-12 text-center text-ink-muted">
                  <span className="inline-flex items-center gap-2">
                    <Spinner className="size-4" /> Loading…
                  </span>
                </td>
              </tr>
            )}
            {!loading && error && rows.length === 0 && (
              <tr>
                <td colSpan={cols} className="py-10 text-center text-sm text-critical">
                  {error}
                </td>
              </tr>
            )}
            {!loading && !error && rows.length === 0 && (
              <tr>
                <td colSpan={cols}>
                  <EmptyState compact title="No log entries match" description={live ? 'Live tail is on; new entries appear here as they arrive.' : 'Try widening the time range, lowering the minimum level or clearing the search.'} />
                </td>
              </tr>
            )}
            {rows.map((r) => {
              const open = expanded.has(r.id)
              const lvl = normalizeLevel(r.level)
              return (
                <Fragment key={r.id}>
                  <tr
                    onClick={() => toggle(r.id)}
                    className={cn(
                      'cursor-pointer border-b border-line/70 align-top hover:bg-surface-sunken/60',
                      lvl === 'ERROR' && 'bg-critical/[0.04]',
                      lvl === 'WARN' && 'bg-warning/[0.04]',
                      open && 'bg-surface-sunken/60',
                    )}
                  >
                    <td className="py-1 pl-3 text-ink-faint">{open ? <ChevronDown className="mt-0.5 size-3.5" /> : <ChevronRight className="mt-0.5 size-3.5" />}</td>
                    <td className="px-2 py-1">
                      <TimeCell at={r.at} />
                    </td>
                    <td className="px-2 py-1">
                      <LevelBadge level={r.level} />
                    </td>
                    <td className="px-2 py-1">
                      {r.component ? (
                        <span className="inline-block max-w-[11rem] truncate rounded bg-line/50 px-1.5 font-mono text-[11px] leading-4 text-ink-muted" title={r.component}>
                          {r.component}
                        </span>
                      ) : (
                        <span className="text-ink-faint">—</span>
                      )}
                    </td>
                    {showComputer && (
                      <td className="max-w-[12rem] truncate px-2 py-1">
                        {r.computer_id ? (
                          <Link to={`/computers/${r.computer_id}/logs`} onClick={(e) => e.stopPropagation()} className="text-ink hover:underline">
                            {r.computer_name || r.computer_id.slice(0, 8)}
                          </Link>
                        ) : (
                          <span className="text-ink-faint">—</span>
                        )}
                      </td>
                    )}
                    <td className="min-w-[20rem] px-2 py-1 pr-4">
                      <span className={cn('block break-words font-mono text-[12px] leading-[18px]', lvl === 'DEBUG' ? 'text-ink-muted' : 'text-ink')}>
                        <Highlight text={r.msg} q={q} />
                      </span>
                      {!open && <AttrChips attrs={r.attrs} q={q} />}
                    </td>
                  </tr>
                  {open && (
                    <tr className="border-b border-line bg-surface-sunken/40">
                      <td />
                      <td colSpan={cols - 1} className="px-2 py-2 pr-4">
                        <div className="mb-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-muted">
                          <span>{absoluteTime(r.at)}</span>
                          <span className="font-mono">#{r.id}</span>
                          {r.computer_name && <span>{r.computer_name}</span>}
                        </div>
                        <CodeBlock text={JSON.stringify({ at: r.at, level: r.level, component: r.component, msg: r.msg, attrs: r.attrs ?? {} }, null, 2)} filename="entry.json" maxHeight="20rem" />
                      </td>
                    </tr>
                  )}
                </Fragment>
              )
            })}
          </tbody>
        </table>
        {!loading && rows.length > 0 && (
          <div className="flex items-center justify-between gap-3 border-t border-line px-4 py-2 text-xs text-ink-muted">
            <span className="tabular-nums">
              {rows.length.toLocaleString()} {rows.length === 1 ? 'entry' : 'entries'} loaded{error ? ` · ${error}` : ''}
            </span>
            {hasOlder ? (
              <Button size="xs" variant="ghost" loading={loadingOlder} onClick={loadOlder}>
                Load older
              </Button>
            ) : (
              <span className="text-ink-faint">End of results</span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
