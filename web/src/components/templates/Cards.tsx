import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, X } from 'lucide-react'
import { Card } from '../ui/Card'
import { cn } from '../../lib/cn'

/** One key figure (Polaris "metrics card"): label, value, a short note, optionally a link. */
export function MetricsCard({ label, value, note, to, tone, icon }: { label: string; value: ReactNode; note?: ReactNode; to?: string; tone?: 'good' | 'warning' | 'critical'; icon?: ReactNode }) {
  const body = (
    <Card className={cn('h-full px-4 py-3.5 transition-colors', to && 'hover:border-line-strong')}>
      <p className="flex items-center gap-1.5 text-xs text-ink-muted [&>svg]:size-3.5 [&>svg]:text-violet-600 dark:[&>svg]:text-violet-400 [&>svg]:glow-violet">
        {icon}
        {label}
      </p>
      <p className={cn('mt-1 text-2xl font-semibold tabular-nums tracking-tight text-ink', tone === 'good' && 'text-good', tone === 'warning' && 'text-warning', tone === 'critical' && 'text-critical')}>{value}</p>
      {note && <p className="mt-0.5 text-xs text-ink-muted">{note}</p>}
    </Card>
  )
  return to ? (
    <Link to={to} className="block rounded-xl focus-ring">
      {body}
    </Link>
  ) : (
    body
  )
}

/** Points to one next step (Polaris "callout card"), e.g. a missing push certificate. */
export function CalloutCard({ title, children, action, actionTo, icon, onDismiss }: { title: string; children?: ReactNode; action: string; actionTo: string; icon?: ReactNode; onDismiss?: () => void }) {
  return (
    <Card className="flex items-start gap-4 px-4 py-4">
      {icon && <div className="glow-under-violet flex size-9 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-violet-600 to-accent-600 text-white [&>svg]:size-5">{icon}</div>}
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold text-ink">{title}</p>
        {children && <div className="mt-0.5 text-sm text-ink-muted">{children}</div>}
        <Link to={actionTo} className="mt-2 inline-flex items-center gap-1 text-sm font-medium text-accent-700 hover:underline dark:text-accent-300">
          {action}
          <ArrowRight className="size-3.5" />
        </Link>
      </div>
      {onDismiss && (
        <button type="button" onClick={onDismiss} className="text-ink-faint hover:text-ink focus-ring rounded" aria-label="Dismiss">
          <X className="size-4" />
        </button>
      )}
    </Card>
  )
}
