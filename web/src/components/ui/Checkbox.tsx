import { type InputHTMLAttributes, type ReactNode, useId } from 'react'
import { cn } from '../../lib/cn'

export interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label?: ReactNode
  description?: ReactNode
}

export function Checkbox({ label, description, className, id, ...rest }: CheckboxProps) {
  const auto = useId()
  const cid = id ?? auto
  return (
    <label htmlFor={cid} className={cn('flex items-start gap-2.5 cursor-pointer select-none', rest.disabled && 'opacity-60 cursor-not-allowed', className)}>
      <input id={cid} type="checkbox" className="mt-0.5 size-4 shrink-0 rounded border-line-strong text-accent-600 accent-accent-600 focus-ring" {...rest} />
      {(label || description) && (
        <span className="flex flex-col">
          {label && <span className="text-sm text-ink">{label}</span>}
          {description && <span className="text-xs text-ink-muted">{description}</span>}
        </span>
      )}
    </label>
  )
}

export interface ToggleProps {
  checked: boolean
  onChange: (v: boolean) => void
  label?: ReactNode
  description?: ReactNode
  disabled?: boolean
  size?: 'sm' | 'md'
  className?: string
}

export function Toggle({ checked, onChange, label, description, disabled, size = 'md', className }: ToggleProps) {
  const dims = size === 'sm' ? { track: 'h-5 w-9', knob: 'size-4', on: 'translate-x-4' } : { track: 'h-6 w-11', knob: 'size-5', on: 'translate-x-5' }
  return (
    <label className={cn('inline-flex items-start gap-3 select-none', disabled ? 'opacity-60 cursor-not-allowed' : 'cursor-pointer', className)}>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={(e) => {
          e.stopPropagation()
          onChange(!checked)
        }}
        className={cn(
          'relative inline-flex shrink-0 items-center rounded-full border-2 border-transparent transition-colors focus-ring',
          dims.track,
          checked ? 'bg-accent-600' : 'bg-line-control',
        )}
      >
        <span className={cn('inline-block rounded-full bg-white shadow transition-transform', dims.knob, checked ? dims.on : 'translate-x-0')} />
      </button>
      {(label || description) && (
        <span className="flex flex-col pt-px">
          {label && <span className="text-sm text-ink">{label}</span>}
          {description && <span className="text-xs text-ink-muted">{description}</span>}
        </span>
      )}
    </label>
  )
}
