import { CheckCircle2, XCircle } from 'lucide-react'
import { EmptyState } from '../ui/EmptyState'

/** Status-colored part-to-whole bar for policy outcomes (good vs critical), with icon + label so color never carries meaning alone. */
export function OutcomeBar({ completed, failed, period = 'last 7 days' }: { completed: number; failed: number; period?: string }) {
  const total = completed + failed
  if (total === 0) return <EmptyState title="No policy runs recorded" description={`Nothing has run in the ${period}.`} compact />
  const pc = Math.round((completed / total) * 100)
  const pf = 100 - pc
  return (
    <div className="flex flex-col gap-4">
      <div className="flex h-4 w-full overflow-hidden rounded-full bg-line/40" role="img" aria-label={`${completed} completed, ${failed} failed`}>
        {completed > 0 && <div className="h-full bg-good" style={{ width: `${pc}%` }} title={`Completed: ${completed}`} />}
        {completed > 0 && failed > 0 && <div className="h-full w-0.5 bg-surface-raised" />}
        {failed > 0 && <div className="h-full bg-critical" style={{ width: `${pf}%` }} title={`Failed: ${failed}`} />}
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="flex items-start gap-2.5">
          <CheckCircle2 className="mt-0.5 size-4 text-good" />
          <div>
            <p className="text-2xl font-semibold text-ink leading-7">{completed.toLocaleString()}</p>
            <p className="text-xs text-ink-muted">
              Completed <span className="tabular-nums">({pc}%)</span>
            </p>
          </div>
        </div>
        <div className="flex items-start gap-2.5">
          <XCircle className="mt-0.5 size-4 text-critical" />
          <div>
            <p className="text-2xl font-semibold text-ink leading-7">{failed.toLocaleString()}</p>
            <p className="text-xs text-ink-muted">
              Failed <span className="tabular-nums">({pf}%)</span>
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
