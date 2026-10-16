import { type ReactNode, useId, cloneElement, isValidElement } from 'react'
import { cn } from '../../lib/cn'

export interface FieldProps {
  label?: ReactNode
  hint?: ReactNode
  error?: ReactNode
  required?: boolean
  className?: string
  children: ReactNode
  inline?: boolean
}

export function Field({ label, hint, error, required, className, children, inline }: FieldProps) {
  const id = useId()
  const child = isValidElement<{ id?: string }>(children) && !children.props.id ? cloneElement(children, { id }) : children
  return (
    <div className={cn(inline ? 'grid grid-cols-1 sm:grid-cols-[180px_1fr] gap-x-4 gap-y-1 items-start' : 'flex flex-col gap-1.5', className)}>
      {label && (
        <label htmlFor={id} className={cn('text-sm font-medium text-ink', inline && 'sm:pt-2')}>
          {label}
          {required && (
            <>
              <span className="text-critical ml-0.5" aria-hidden="true">
                *
              </span>
              <span className="sr-only"> (required)</span>
            </>
          )}
        </label>
      )}
      <div className="flex flex-col gap-1 min-w-0">
        {child}
        {hint && !error && <p className="text-xs text-ink-muted">{hint}</p>}
        {error && <p className="text-xs text-critical">{error}</p>}
      </div>
    </div>
  )
}

export function FormGrid({ children, className, cols = 2 }: { children: ReactNode; className?: string; cols?: 1 | 2 | 3 }) {
  return <div className={cn('grid gap-4', cols === 1 ? 'grid-cols-1' : cols === 2 ? 'grid-cols-1 md:grid-cols-2' : 'grid-cols-1 md:grid-cols-3', className)}>{children}</div>
}

export function FormSection({ title, description, children, className }: { title: ReactNode; description?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn('flex flex-col gap-4', className)}>
      <div>
        <h3 className="text-sm font-semibold text-ink">{title}</h3>
        {description && <p className="text-xs text-ink-muted mt-0.5">{description}</p>}
      </div>
      {children}
    </section>
  )
}
