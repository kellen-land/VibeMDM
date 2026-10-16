import type { HTMLAttributes, ReactNode } from 'react'
import { cn } from '../../lib/cn'

export type BadgeTone = 'neutral' | 'accent' | 'good' | 'warning' | 'serious' | 'critical' | 'info'

/* Low-chrome tinted chips: subtle background, hairline border, legible ink. */
export const badgeToneClass: Record<BadgeTone, string> = {
  neutral: 'bg-line/40 text-ink-muted border-line',
  accent: 'bg-accent-500/12 text-accent-700 border-accent-500/25 dark:text-accent-300',
  info: 'bg-accent-500/10 text-accent-700 border-accent-500/20 dark:text-accent-300',
  good: 'bg-good/12 text-[#166534] border-good/25 dark:text-[#5fd98a]',
  warning: 'bg-warning/14 text-[#8a5a0a] border-warning/30 dark:text-[#f0c070]',
  serious: 'bg-serious/14 text-[#9a4420] border-serious/30 dark:text-[#f6a683]',
  critical: 'bg-critical/12 text-[#b22424] border-critical/28 dark:text-[#f28b8b]',
}

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone
  dot?: boolean
  icon?: ReactNode
  size?: 'sm' | 'md'
}

export function Badge({ tone = 'neutral', dot, icon, size = 'sm', className, children, ...rest }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md border font-medium whitespace-nowrap tabular-nums',
        size === 'sm' ? 'px-1.5 py-0.5 text-[11px] leading-4' : 'px-2 py-0.5 text-xs',
        badgeToneClass[tone],
        className,
      )}
      {...rest}
    >
      {dot && <span className={cn('size-1.5 rounded-full', dotColor[tone])} aria-hidden="true" />}
      {icon && <span className="[&>svg]:size-3">{icon}</span>}
      {children}
    </span>
  )
}

const dotColor: Record<BadgeTone, string> = {
  neutral: 'bg-ink-faint',
  accent: 'bg-accent-500',
  info: 'bg-accent-400',
  good: 'bg-good',
  warning: 'bg-warning',
  serious: 'bg-serious',
  critical: 'bg-critical',
}

export function statusTone(status: string | null | undefined): BadgeTone {
  switch ((status ?? '').toLowerCase()) {
    case 'completed':
    case 'acknowledged':
    case 'installed':
    case 'enabled':
    case 'online':
    case 'ok':
      return 'good'
    case 'failed':
    case 'error':
    case 'commandformaterror':
      return 'critical'
    case 'pending':
    case 'queued':
    case 'sent':
    case 'running':
    case 'removing':
      return 'info'
    case 'notnow':
    case 'skipped':
      return 'warning'
    case 'cancelled':
    case 'removed':
    case 'disabled':
      return 'neutral'
    default:
      return 'neutral'
  }
}

export function StatusBadge({ status, className }: { status: string | null | undefined; className?: string }) {
  const s = status ?? 'unknown'
  const label = s === 'NotNow' ? 'Not now' : s === 'CommandFormatError' ? 'Format error' : s.replace(/_/g, ' ').replace(/^\w/, (c) => c.toUpperCase())
  return (
    <Badge tone={statusTone(s)} dot className={className}>
      {label}
    </Badge>
  )
}

export function BoolBadge({ value, yes = 'Yes', no = 'No', unknown = 'Unknown', invert }: { value: boolean | null | undefined; yes?: string; no?: string; unknown?: string; invert?: boolean }) {
  if (value === null || value === undefined) return <Badge tone="neutral">{unknown}</Badge>
  void invert
  return (
    <Badge tone={value ? 'good' : 'warning'} dot>
      {value ? yes : no}
    </Badge>
  )
}
