import { absoluteTime, relativeTime } from '../../lib/format'
import { useNow } from '../../lib/hooks'
import { cn } from '../../lib/cn'

export function RelativeTime({ value, className, emptyLabel = 'never' }: { value: string | Date | null | undefined; className?: string; emptyLabel?: string }) {
  const now = useNow()
  if (!value) return <span className={cn('text-ink-faint', className)}>{emptyLabel}</span>
  const abs = absoluteTime(value)
  if (!abs) return <span className={cn('text-ink-faint', className)}>{emptyLabel}</span>
  return (
    <time dateTime={typeof value === 'string' ? value : value.toISOString()} title={abs} className={cn('whitespace-nowrap', className)}>
      {relativeTime(value, now)}
    </time>
  )
}
