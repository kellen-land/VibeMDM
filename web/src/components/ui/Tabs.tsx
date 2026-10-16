import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'

export interface TabItem<K extends string = string> {
  key: K
  label: ReactNode
  count?: number
  icon?: ReactNode
  disabled?: boolean
  /** Keyboard shortcut that opens this tab (e.g. "logs" for ⌃L). */
  shortcut?: string
}

export function Tabs<K extends string>({
  items,
  value,
  onChange,
  className,
  variant = 'underline',
}: {
  items: TabItem<K>[]
  value: K
  onChange: (k: K) => void
  className?: string
  variant?: 'underline' | 'pills'
}) {
  return (
    <div className={cn(variant === 'underline' ? 'border-b border-line' : '', className)} role="tablist">
      <div className={cn('flex gap-0.5 overflow-x-auto', variant === 'underline' ? '-mb-px' : 'rounded-lg bg-surface-sunken p-1 w-fit border border-line')}>
        {items.map((t) => {
          const active = t.key === value
          return (
            <button
              key={t.key}
              role="tab"
              type="button"
              aria-selected={active}
              disabled={t.disabled}
              data-shortcut={t.shortcut}
              onClick={() => onChange(t.key)}
              className={cn(
                'inline-flex items-center gap-1.5 whitespace-nowrap text-[13px] font-medium transition-colors focus-ring disabled:opacity-45',
                variant === 'underline'
                  ? cn('px-3 py-2.5 border-b-2', active ? 'border-accent-700 dark:border-accent-400 text-ink' : 'border-transparent text-ink-muted hover:text-ink hover:border-line-strong')
                  : cn('px-3 py-1.5 rounded-md', active ? 'bg-surface-raised text-ink shadow-sm' : 'text-ink-muted hover:text-ink'),
              )}
            >
              {t.icon && <span className="[&>svg]:size-4">{t.icon}</span>}
              {t.label}
              {t.count !== undefined && (
                <span className={cn('rounded px-1.5 text-[11px] leading-4 tabular-nums', active ? 'bg-accent-500/15 text-accent-700 dark:text-accent-300' : 'bg-line/60 text-ink-muted')}>
                  {t.count}
                </span>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}
