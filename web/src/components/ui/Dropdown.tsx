import { useEffect, useRef, useState, type ReactNode } from 'react'
import { cn } from '../../lib/cn'

export interface DropdownItem {
  label: ReactNode
  icon?: ReactNode
  onSelect?: () => void
  danger?: boolean
  disabled?: boolean
  separator?: boolean
}

export function Dropdown({ trigger, items, align = 'left', className, menuClassName }: { trigger: ReactNode; items: DropdownItem[]; align?: 'left' | 'right' | 'up'; className?: string; menuClassName?: string }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])
  return (
    <div ref={ref} className={cn('relative inline-block', className)}>
      <div onClick={() => setOpen((o) => !o)}>{trigger}</div>
      {open && (
        <div
          role="menu"
          className={cn(
            'absolute z-40 min-w-48 rounded-xl border border-line bg-surface-raised p-1 shadow-xl animate-fade-in',
            align === 'right' ? 'right-0 mt-1' : align === 'up' ? 'left-0 bottom-full mb-1' : 'left-0 mt-1',
            menuClassName,
          )}
        >
          {items.map((it, i) =>
            it.separator ? (
              <div key={i} className="my-1 h-px bg-line" />
            ) : (
              <button
                key={i}
                type="button"
                role="menuitem"
                disabled={it.disabled}
                onClick={() => {
                  setOpen(false)
                  it.onSelect?.()
                }}
                className={cn(
                  'flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-sm disabled:opacity-50',
                  it.danger ? 'text-critical hover:bg-critical/10' : 'text-ink hover:bg-line/50',
                )}
              >
                {it.icon && <span className="text-ink-muted [&>svg]:size-4">{it.icon}</span>}
                {it.label}
              </button>
            ),
          )}
        </div>
      )}
    </div>
  )
}
