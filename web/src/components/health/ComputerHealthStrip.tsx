import { Link } from 'react-router-dom'
import { HeartPulse } from 'lucide-react'
import { useHealthFindings } from '../../api/queries'
import { SeverityIcon, sevMeta } from './health'
import { cn } from '../../lib/cn'

/** One line per open health finding for this Mac. Renders nothing when the Mac is healthy. */
export function ComputerHealthStrip({ computerId, className }: { computerId: string; className?: string }) {
  const q = useHealthFindings({ object_type: 'computer', object_id: computerId })
  const items = [...(q.data?.items ?? [])].sort((a, b) => sevMeta(a.severity).rank - sevMeta(b.severity).rank || Number(a.state === 'snoozed') - Number(b.state === 'snoozed'))
  if (items.length === 0) return null
  return (
    <div className={cn('rounded-xl border border-line bg-surface px-4 py-2.5', className)} aria-label="Health findings for this computer">
      <p className="micro-label mb-1.5 inline-flex items-center gap-1.5">
        <HeartPulse className="size-3.5" aria-hidden="true" />
        Health
      </p>
      <ul className="flex flex-col gap-1">
        {items.map((f) => (
          <li key={f.id} className={cn('flex items-center gap-2.5 text-[13px]', (f.state !== 'open' || f.severity === 'info') && 'opacity-70')}>
            <SeverityIcon severity={f.severity} />
            <span className="min-w-0 truncate text-ink">{f.summary}</span>
            {f.state !== 'open' && <span className="shrink-0 text-xs text-ink-faint">{f.state}</span>}
            <span className="hidden shrink-0 text-xs text-ink-faint md:inline">{f.check_title}</span>
            <Link to={`/health?finding=${encodeURIComponent(f.id)}`} className="ml-auto shrink-0 text-xs text-accent-700 hover:underline dark:text-accent-300">
              Details
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}
