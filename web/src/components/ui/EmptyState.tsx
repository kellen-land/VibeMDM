import type { ReactNode } from 'react'
import { Inbox } from 'lucide-react'
import { cn } from '../../lib/cn'

export function EmptyState({
  title,
  description,
  action,
  icon,
  compact,
  className,
}: {
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
  icon?: ReactNode
  compact?: boolean
  className?: string
}) {
  return (
    <div className={cn('flex flex-col items-center justify-center text-center', compact ? 'py-10 px-4' : 'py-20 px-6', className)}>
      <div className="mb-3 flex size-10 items-center justify-center rounded-full bg-line/50 text-ink-faint [&>svg]:size-5">{icon ?? <Inbox />}</div>
      <p className="text-sm font-medium text-ink">{title}</p>
      {description && <p className="mt-1 max-w-sm text-xs text-ink-muted">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}
