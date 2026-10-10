import { cn } from '../../lib/cn'

export function Spinner({ className }: { className?: string }) {
  return (
    <svg className={cn('animate-spin size-5 text-current', className)} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" />
      <path className="opacity-80" fill="currentColor" d="M4 12a8 8 0 018-8v3a5 5 0 00-5 5H4z" />
    </svg>
  )
}

export function LoadingBlock({ label = 'Loading…', className }: { label?: string; className?: string }) {
  return (
    <div className={cn('flex items-center justify-center gap-2 py-12 text-ink-muted text-sm', className)}>
      <Spinner className="size-4" />
      {label}
    </div>
  )
}
