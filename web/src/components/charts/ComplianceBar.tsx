import { useState, type ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import type { UpdateState } from '../../api/types'
import { updateStateMeta, updateStates } from '../updates/updateState'
import { EmptyState } from '../ui/EmptyState'
import { cn } from '../../lib/cn'

export interface BarSegment<K extends string> {
  key: K
  label: string
  n: number
  /** Fill class for the bar segment and legend swatch. */
  fill: string
  /** Icon colour class. */
  ink: string
  icon: LucideIcon
}

/**
 * Part-to-whole bar. A 2px surface gap separates segments, each segment has a hover tooltip, and the legend
 * below (icon + label + count) carries identity so colour is never alone. Selecting a segment or legend item
 * calls onSelect (toggles back to '').
 */
export function SegmentBar<K extends string>({
  segments,
  headline,
  active,
  onSelect,
  emptyTitle = 'Nothing to show',
  emptyDescription,
}: {
  segments: BarSegment<K>[]
  headline?: ReactNode
  active?: K | ''
  onSelect?: (k: K | '') => void
  emptyTitle?: ReactNode
  emptyDescription?: ReactNode
}) {
  const [hover, setHover] = useState<K | null>(null)
  const segs = segments.filter((x) => x.n > 0)
  const total = segs.reduce((a, x) => a + x.n, 0)
  if (total === 0) return <EmptyState title={emptyTitle} description={emptyDescription} compact />
  const pct = (n: number) => (n / total) * 100
  const fmtPct = (n: number) => {
    const p = pct(n)
    return p > 0 && p < 1 ? '<1%' : `${Math.round(p)}%`
  }
  const hv = hover ? segs.find((x) => x.key === hover) : null
  const hoverLeft = hv ? segs.slice(0, segs.indexOf(hv)).reduce((a, x) => a + pct(x.n), 0) + pct(hv.n) / 2 : 0

  return (
    <div className="flex flex-col gap-4">
      {headline}
      <div className="relative">
        <div className="flex h-4 w-full gap-[2px] overflow-hidden rounded-full" role="img" aria-label={segs.map((x) => `${x.label}: ${x.n}`).join(', ')} onMouseLeave={() => setHover(null)}>
          {segs.map((x) => (
            <button
              key={x.key}
              type="button"
              tabIndex={-1}
              aria-hidden="true"
              onMouseEnter={() => setHover(x.key)}
              onClick={() => onSelect?.(active === x.key ? '' : x.key)}
              className={cn('h-full min-w-[3px] transition-opacity', x.fill, hover && hover !== x.key && 'opacity-55', onSelect && 'cursor-pointer')}
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
            <span className="font-medium text-ink">{hv.label}</span>
            <span className="ml-2 tabular-nums text-ink-muted">
              {hv.n.toLocaleString()} · {fmtPct(hv.n)}
            </span>
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-x-5 gap-y-2">
        {segments.map((x) => {
          const isActive = active === x.key
          const Icon = x.icon
          return (
            <button
              key={x.key}
              type="button"
              onClick={() => onSelect?.(isActive ? '' : x.key)}
              onMouseEnter={() => x.n > 0 && setHover(x.key)}
              onMouseLeave={() => setHover(null)}
              aria-pressed={onSelect ? isActive : undefined}
              className={cn(
                'inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-xs focus-ring',
                isActive ? 'bg-accent-500/12 text-ink' : 'text-ink-muted hover:text-ink',
                x.n === 0 && 'opacity-55',
                !onSelect && 'cursor-default',
              )}
            >
              <span className={cn('size-2.5 rounded-sm', x.fill)} aria-hidden="true" />
              <Icon className={cn('size-3.5', x.ink)} aria-hidden="true" />
              {x.label}
              <span className="tabular-nums text-ink">{x.n.toLocaleString()}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

/** Big percentage plus a muted explanation, used above a SegmentBar. */
export function BarHeadline({ value, children }: { value: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-baseline gap-3">
      <p className="text-[30px] font-semibold tracking-tight text-ink leading-9 tabular-nums">{value}</p>
      <p className="text-xs text-ink-muted">{children}</p>
    </div>
  )
}

/**
 * Macs by software update state. Status colours (good / warning / critical) only for states that mean good or
 * bad; the three neutral states use gray steps plus a hatch.
 */
export function ComplianceBar({ counts, active, onSelect }: { counts: Partial<Record<UpdateState, number>>; active?: UpdateState | ''; onSelect?: (s: UpdateState | '') => void }) {
  const targeted = (counts.up_to_date ?? 0) + (counts.pending ?? 0) + (counts.failed ?? 0)
  const segments = updateStates.map((s) => ({ key: s, label: updateStateMeta[s].label, n: counts[s] ?? 0, fill: updateStateMeta[s].fill, ink: updateStateMeta[s].ink, icon: updateStateMeta[s].icon }))
  return (
    <SegmentBar
      segments={segments}
      active={active}
      onSelect={onSelect}
      emptyTitle="No computers yet"
      emptyDescription="Update compliance appears once Macs are enrolled."
      headline={
        <BarHeadline value={targeted ? `${Math.round(((counts.up_to_date ?? 0) / targeted) * 100)}%` : '—'}>
          of targeted Macs are up to date{' '}
          <span className="tabular-nums">
            ({(counts.up_to_date ?? 0).toLocaleString()} of {targeted.toLocaleString()})
          </span>
        </BarHeadline>
      }
    />
  )
}
