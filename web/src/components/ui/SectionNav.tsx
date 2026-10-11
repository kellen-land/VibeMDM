import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'
import { Select } from './Input'

export interface SectionNavItem {
  key: string
  label: string
  count?: number
  icon?: ReactNode
}

export interface SectionNavGroup {
  heading?: string
  items: SectionNavItem[]
}

/**
 * A short vertical list of sections beside the content (a select on small screens), the way
 * Jamf Pro nests settings and computer records. One section is shown at a time.
 */
export function SectionNav({ groups, value, onChange, label, className }: { groups: SectionNavGroup[]; value: string; onChange: (key: string) => void; label: string; className?: string }) {
  return (
    <nav aria-label={label} className={cn('lg:w-52 lg:shrink-0', className)}>
      <Select className="lg:hidden" value={value} onChange={(e) => onChange(e.target.value)} aria-label={label}>
        {groups.map((g, gi) =>
          g.heading ? (
            <optgroup key={g.heading} label={g.heading}>
              {g.items.map((i) => (
                <option key={i.key} value={i.key}>
                  {i.label}
                </option>
              ))}
            </optgroup>
          ) : (
            g.items.map((i) => (
              <option key={`${gi}-${i.key}`} value={i.key}>
                {i.label}
              </option>
            ))
          ),
        )}
      </Select>
      <div className="hidden space-y-4 lg:block">
        {groups.map((g, gi) => (
          <div key={g.heading ?? gi}>
            {g.heading && <p className="px-2 pb-1 micro-label text-ink-faint">{g.heading}</p>}
            {g.items.map((i) => (
              <button
                key={i.key}
                type="button"
                onClick={() => onChange(i.key)}
                aria-current={i.key === value ? 'page' : undefined}
                className={cn(
                  'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] focus-ring',
                  i.key === value ? 'bg-accent-500/12 font-medium text-ink' : 'text-ink-muted hover:bg-line/40 hover:text-ink',
                )}
              >
                {i.icon && <span className="[&>svg]:size-4 text-ink-faint">{i.icon}</span>}
                <span className="truncate">{i.label}</span>
                {i.count !== undefined && i.count > 0 && <span className="ml-auto text-[11px] tabular-nums text-ink-faint">{i.count}</span>}
              </button>
            ))}
          </div>
        ))}
      </div>
    </nav>
  )
}
