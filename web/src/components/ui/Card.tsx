import type { HTMLAttributes, ReactNode } from 'react'
import { cn } from '../../lib/cn'

export function Card({ className, children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div data-card="" className={cn('rounded-xl border border-line bg-surface', className)} {...rest}>
      {children}
    </div>
  )
}

export function CardHeader({
  title,
  description,
  actions,
  className,
}: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  className?: string
}) {
  return (
    <div className={cn('flex items-start justify-between gap-4 px-4 py-3 border-b border-line', className)}>
      <div className="min-w-0">
        <h2 className="text-[13px] font-semibold text-ink leading-5">{title}</h2>
        {description && <p className="text-xs text-ink-muted mt-0.5">{description}</p>}
      </div>
      {actions && <div className="flex items-center gap-2 shrink-0">{actions}</div>}
    </div>
  )
}

export function CardBody({ className, children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn('px-4 py-4', className)} {...rest}>
      {children}
    </div>
  )
}

export function CardFooter({ className, children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn('px-4 py-3 border-t border-line flex items-center justify-end gap-2 bg-surface-sunken/40 rounded-b-xl', className)} {...rest}>
      {children}
    </div>
  )
}
