import { useState } from 'react'
import type { UpdateState } from '../../api/types'
import { updateStateMeta, updateStates } from '../updates/updateState'
import { EmptyState } from '../ui/EmptyState'
import { cn } from '../../lib/cn'

/**
 * Part-to-whole bar of Macs by software update state. Status colors (good / warning / critical) only for states
 * that mean good or bad; the three neutral states use gray steps plus a hatch. A 2px surface gap separates segments,
 * each segment has a hover tooltip, and the legend below (icon + label + count) carries identity so color is never alone.
 */
export function ComplianceBar({ counts, active, onSelect }: { counts: Partial<Record<UpdateState, number>>; active?: UpdateState | ''; onSelect?: (s: UpdateState | '') => void }) {
  const [hover, setHover] = useState<UpdateState | null>(null)
  const segs = updateStates.map((s) => ({ s, n: counts[s] ?? 0 })).filter((x) => x.n > 0)
  const total = segs.reduce((a, x) => a + x.n, 0)
  if (total === 0) return <EmptyState title="No computers yet" description="Update compliance appears once Macs are enrolled." compact />
  const targeted = (counts.up_to_date ?? 0) + (counts.pending ?? 0) + (counts.failed ?? 0)
  const pct = (n: number) => (n / total) * 100
  const fmtPct = (n: number) => {
    const p = pct(n)
    return p > 0 && p < 1 ? '<1%' : `${Math.round(p)}%`
  }
  const hv = hover ? segs.find((x) => x.s === hover) : null
  const hoverLeft = hv ? segs.slice(0, segs.indexOf(hv)).reduce((a, x) => a + pct(x.n), 0) + pct(hv.n) / 2 : 0

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-baseline gap-3">
        <p className="text-[30px] font-semibold tracking-tight text-ink leading-9 tabular-nums">{targeted ? `${Math.round(((counts.up_to_date ?? 0) / targeted) * 100)}%` : '—'}</p>
        <p className="text-xs text-ink-muted">
          of targeted Macs are up to date <span className="tabular-nums">({(counts.up_to_date ?? 0).toLocaleString()} of {targeted.toLocaleString()})</span>
        </p>
      </div>
      <div className="relative">
        <div className="flex h-4 w-full gap-[2px] overflow-hidden rounded-full" role="img" aria-label={segs.map((x) => `${updateStateMeta[x.s].label}: ${x.n}`).join(', ')} onMouseLeave={() => setHover(null)}>
          {segs.map((x) => (
            <button
              key={x.s}
              type="button"
              tabIndex={-1}
              aria-hidden="true"
              onMouseEnter={() => setHover(x.s)}
              onClick={() => onSelect?.(active === x.s ? '' : x.s)}
              className={cn('h-full min-w-[3px] transition-opacity', updateStateMeta[x.s].fill, hover && hover !== x.s && 'opacity-55', onSelect && 'cursor-pointer')}
              style={{ flexGrow: x.n, flexBasis: 0 }}
            />
          ))}
        </div>
        {hv && (
          <div
            className="pointer-events-none absolute bottom-full z-10 mb-2 -translate-x-1/2 whitespace-nowrap rounded-md border border-line-strong/60 bg-surface-raised px-2.5 py-1.5 text-xs shadow-lg"
            style={{ left: `clamp(60px, ${hoverLeft}%, calc(100% - 60px))` }}
            role="tooltip"
          >
            <span className="font-medium text-ink">{updateStateMeta[hv.s].label}</span>
            <span className="ml-2 tabular-nums text-ink-muted">
              {hv.n.toLocaleString()} · {fmtPct(hv.n)}
            </span>
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-x-5 gap-y-2">
        {updateStates.map((s) => {
          const m = updateStateMeta[s]
          const n = counts[s] ?? 0
          const isActive = active === s
          return (
            <button
              key={s}
              type="button"
              onClick={() => onSelect?.(isActive ? '' : s)}
              onMouseEnter={() => n > 0 && setHover(s)}
              onMouseLeave={() => setHover(null)}
              aria-pressed={isActive}
              className={cn('inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-xs focus-ring', isActive ? 'bg-accent-500/12 text-ink' : 'text-ink-muted hover:text-ink', n === 0 && 'opacity-55')}
            >
              <span className={cn('size-2.5 rounded-sm', m.fill)} aria-hidden="true" />
              <m.icon className={cn('size-3.5', m.ink)} aria-hidden="true" />
              {m.label}
              <span className="tabular-nums text-ink">{n.toLocaleString()}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
