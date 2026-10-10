import { cn } from '../../lib/cn'

export function ProgressBar({ value, className, tone = 'accent' }: { value: number; className?: string; tone?: 'accent' | 'good' | 'critical' }) {
  const pct = Math.max(0, Math.min(100, value))
  return (
    <div className={cn('h-2 w-full overflow-hidden rounded-full bg-accent-100 dark:bg-accent-950', className)} role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
      <div className={cn('h-full rounded-full transition-[width]', tone === 'good' ? 'bg-good' : tone === 'critical' ? 'bg-critical' : 'bg-accent-600')} style={{ width: `${pct}%` }} />
    </div>
  )
}
