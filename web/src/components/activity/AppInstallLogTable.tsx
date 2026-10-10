import { Fragment, useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown, ChevronRight } from 'lucide-react'
import type { AppInstallLog } from '../../api/types'
import { Badge, StatusBadge } from '../ui/Badge'
import { RelativeTime } from '../ui/RelativeTime'
import { EmptyState } from '../ui/EmptyState'
import { Spinner } from '../ui/Spinner'
import { titleCase } from '../../lib/format'
import { cn } from '../../lib/cn'

/** Expandable table of Installomator runs, mirroring PolicyLogTable. */
export function AppInstallLogTable({
  logs,
  loading,
  showApp = true,
  showComputer = true,
  emptyTitle = 'No app installs recorded',
}: {
  logs: AppInstallLog[] | undefined
  loading?: boolean
  showApp?: boolean
  showComputer?: boolean
  emptyTitle?: string
}) {
  const [open, setOpen] = useState<Set<string>>(new Set())
  const toggle = (id: string) =>
    setOpen((s) => {
      const n = new Set(s)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  if (loading && !logs) {
    return (
      <div className="flex items-center justify-center gap-2 py-12 text-sm text-ink-muted">
        <Spinner className="size-4" /> Loading…
      </div>
    )
  }
  if (!logs || logs.length === 0) return <EmptyState title={emptyTitle} description="Runs appear once a scoped agent installs or updates an app from the catalog." compact />
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-line text-left text-xs uppercase tracking-wide text-ink-muted">
            <th className="w-8 pl-4 py-2.5" />
            {showApp && <th className="px-3 py-2.5 font-medium">App</th>}
            {showComputer && <th className="px-3 py-2.5 font-medium">Computer</th>}
            <th className="px-3 py-2.5 font-medium">Trigger</th>
            <th className="px-3 py-2.5 font-medium">Status</th>
            <th className="px-3 py-2.5 font-medium">Started</th>
            <th className="px-3 py-2.5 font-medium pr-5">Completed</th>
          </tr>
        </thead>
        <tbody>
          {logs.map((l) => {
            const isOpen = open.has(l.id)
            return (
              <Fragment key={l.id}>
                <tr onClick={() => toggle(l.id)} className={cn('cursor-pointer border-b border-line transition-colors hover:bg-surface-sunken/70', isOpen && 'bg-surface-sunken/50')}>
                  <td className="pl-4 py-2.5 text-ink-faint">{isOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}</td>
                  {showApp && (
                    <td className="px-3 py-2.5">
                      <Link to={`/apps/${l.app_id}`} onClick={(e) => e.stopPropagation()} className="font-medium text-ink hover:underline">
                        {l.app_name || 'Deleted app'}
                      </Link>
                    </td>
                  )}
                  {showComputer && (
                    <td className="px-3 py-2.5">
                      <Link to={`/computers/${l.computer_id}`} onClick={(e) => e.stopPropagation()} className="text-ink hover:underline">
                        {l.computer_name || 'Deleted computer'}
                      </Link>
                    </td>
                  )}
                  <td className="px-3 py-2.5">
                    <Badge>{titleCase(l.trigger || 'scope')}</Badge>
                  </td>
                  <td className="px-3 py-2.5">
                    <StatusBadge status={l.status} />
                  </td>
                  <td className="px-3 py-2.5 text-ink-muted">
                    <RelativeTime value={l.created_at} />
                  </td>
                  <td className="px-3 py-2.5 pr-5 text-ink-muted">
                    <RelativeTime value={l.completed_at} emptyLabel="—" />
                  </td>
                </tr>
                {isOpen && (
                  <tr className="border-b border-line bg-surface-sunken/30">
                    <td colSpan={5 + (showApp ? 1 : 0) + (showComputer ? 1 : 0)} className="px-5 py-4">
                      <div className="flex flex-col gap-2">
                        <span className="text-xs text-ink-muted font-mono">Run ID: {l.id}</span>
                        <pre className="max-h-96 overflow-auto rounded-lg border border-line bg-surface-sunken p-3 text-xs font-mono whitespace-pre-wrap text-ink">
                          {l.log || <span className="text-ink-faint">{l.status === 'pending' || l.status === 'running' ? 'Waiting for the agent to report output.' : '(no output recorded)'}</span>}
                        </pre>
                      </div>
                    </td>
                  </tr>
                )}
              </Fragment>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
