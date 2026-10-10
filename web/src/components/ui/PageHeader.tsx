import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import { cn } from '../../lib/cn'

export interface Crumb {
  label: ReactNode
  to?: string
}

export function PageHeader({
  title,
  description,
  actions,
  crumbs,
  className,
  meta,
}: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  crumbs?: Crumb[]
  className?: string
  meta?: ReactNode
}) {
  return (
    <div className={cn('mb-6 flex flex-col gap-3', className)}>
      {crumbs && crumbs.length > 0 && (
        <nav className="flex items-center gap-1 text-xs text-ink-muted" aria-label="Breadcrumb">
          {crumbs.map((c, i) => (
            <span key={i} className="inline-flex items-center gap-1">
              {i > 0 && <ChevronRight className="size-3 text-ink-faint" />}
              {c.to ? (
                <Link to={c.to} className="hover:text-ink">
                  {c.label}
                </Link>
              ) : (
                <span className="text-ink">{c.label}</span>
              )}
            </span>
          ))}
        </nav>
      )}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-lg font-semibold tracking-tight text-ink truncate">{title}</h1>
          {description && <p className="mt-1 text-sm text-ink-muted">{description}</p>}
          {meta && <div className="mt-2 flex flex-wrap items-center gap-2">{meta}</div>}
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </div>
  )
}
