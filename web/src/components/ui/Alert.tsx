import type { ReactNode } from 'react'
import { AlertTriangle, CheckCircle2, Info, AlertCircle } from 'lucide-react'
import { cn } from '../../lib/cn'

export type AlertTone = 'info' | 'warning' | 'critical' | 'good'

const tones: Record<AlertTone, string> = {
  info: 'border-accent-500/25 bg-accent-500/8 text-ink dark:text-accent-100',
  warning: 'border-warning/50 bg-warning/10 text-[#5c3d00] dark:text-[#fad27a]',
  critical: 'border-critical/40 bg-critical/10 text-[#7a1f1f] dark:text-[#f3a0a0]',
  good: 'border-good/40 bg-good/10 text-[#0b4f0b] dark:text-[#8fe08f]',
}

export function Alert({ tone = 'info', title, children, className, actions }: { tone?: AlertTone; title?: ReactNode; children?: ReactNode; className?: string; actions?: ReactNode }) {
  const Icon = tone === 'warning' ? AlertTriangle : tone === 'critical' ? AlertCircle : tone === 'good' ? CheckCircle2 : Info
  return (
    <div className={cn('flex gap-3 rounded-xl border px-4 py-3 text-sm', tones[tone], className)} role={tone === 'critical' || tone === 'warning' ? 'alert' : 'status'}>
      <Icon className="mt-0.5 size-4 shrink-0" />
      <div className="min-w-0 flex-1">
        {title && <p className="font-medium">{title}</p>}
        {children && <div className={cn('text-[13px] leading-relaxed', title && 'mt-1 opacity-90')}>{children}</div>}
      </div>
      {actions && <div className="shrink-0 self-center">{actions}</div>}
    </div>
  )
}
