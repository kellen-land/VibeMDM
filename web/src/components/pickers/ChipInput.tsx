import { useState, type KeyboardEvent, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { Input } from '../ui/Input'
import { cn } from '../../lib/cn'

/** Free-text multi-value input: type a value and press Enter (or comma) to add it as a chip. */
export function ChipInput({
  value,
  onChange,
  disabled,
  placeholder = 'Type a value and press Enter',
  icon,
  className,
  mono,
}: {
  value: string[]
  onChange: (v: string[]) => void
  disabled?: boolean
  placeholder?: string
  icon?: ReactNode
  className?: string
  mono?: boolean
}) {
  const [draft, setDraft] = useState('')
  const commit = () => {
    const parts = draft
      .split(/[,\s]+/)
      .map((s) => s.trim())
      .filter(Boolean)
    if (parts.length === 0) return
    const next = [...value]
    for (const p of parts) if (!next.includes(p)) next.push(p)
    onChange(next)
    setDraft('')
  }
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      commit()
    } else if (e.key === 'Backspace' && draft === '' && value.length > 0) {
      onChange(value.slice(0, -1))
    }
  }
  return (
    <div className={cn('flex flex-col gap-2', className)}>
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {value.map((v) => (
            <span key={v} className={cn('inline-flex items-center gap-1.5 rounded-full border border-line bg-surface-sunken pl-2.5 pr-1 py-0.5 text-xs text-ink', mono && 'font-mono')}>
              {icon && <span className="text-ink-faint [&>svg]:size-3">{icon}</span>}
              <span className="max-w-[220px] truncate">{v}</span>
              {!disabled && (
                <button type="button" onClick={() => onChange(value.filter((x) => x !== v))} className="rounded-full p-0.5 text-ink-faint hover:bg-line hover:text-ink" aria-label="Remove">
                  <X className="size-3" />
                </button>
              )}
            </span>
          ))}
        </div>
      )}
      {!disabled && <Input value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={onKey} onBlur={commit} placeholder={placeholder} mono={mono} autoComplete="off" spellCheck={false} />}
    </div>
  )
}
