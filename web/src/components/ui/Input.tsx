import { forwardRef, type InputHTMLAttributes, type SelectHTMLAttributes, type TextareaHTMLAttributes } from 'react'
import { cn } from '../../lib/cn'

export const inputClass =
  'block w-full rounded-md border border-line-control bg-surface-sunken px-2.5 text-[13px] text-ink placeholder:text-ink-faint shadow-none focus-ring disabled:opacity-60 disabled:bg-surface-sunken read-only:bg-surface-sunken/70'

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  mono?: boolean
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input({ className, mono, ...rest }, ref) {
  return <input ref={ref} className={cn(inputClass, 'h-[34px]', mono && 'font-mono', className)} {...rest} />
})

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  mono?: boolean
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea({ className, mono, rows = 4, ...rest }, ref) {
  return <textarea ref={ref} rows={rows} className={cn(inputClass, 'py-2 leading-relaxed', mono && 'font-mono text-xs', className)} {...rest} />
})

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  options?: { value: string; label: string; disabled?: boolean }[]
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select({ className, options, children, ...rest }, ref) {
  return (
    <select ref={ref} className={cn(inputClass, 'h-[34px] pr-8 appearance-none bg-no-repeat bg-[right_0.55rem_center] bg-[length:13px]', className)} style={{ backgroundImage: chevron }} {...rest}>
      {options?.map((o) => (
        <option key={o.value} value={o.value} disabled={o.disabled}>
          {o.label}
        </option>
      ))}
      {children}
    </select>
  )
})

const chevron = `url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='13' height='13' viewBox='0 0 24 24' fill='none' stroke='%23847e9e' stroke-width='2.5' stroke-linecap='round' stroke-linejoin='round'><polyline points='6 9 12 15 18 9'/></svg>")`
