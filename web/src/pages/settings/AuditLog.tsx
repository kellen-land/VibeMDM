import { Fragment, useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useAudit } from '../../api/queries'
import { errorMessage } from '../../api/client'
import type { AuditEntry } from '../../api/types'
import { Card, CardHeader } from '../../components/ui/Card'
import { Badge } from '../../components/ui/Badge'
import { Pagination } from '../../components/ui/Pagination'
import { EmptyState } from '../../components/ui/EmptyState'
import { Spinner } from '../../components/ui/Spinner'
import { absoluteTime } from '../../lib/format'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { cn } from '../../lib/cn'

function actionTone(action: string): 'neutral' | 'good' | 'critical' | 'warning' | 'accent' {
  if (action.endsWith('.delete') || action.includes('flush') || action.includes('regenerate') || action.includes('rotate')) return 'critical'
  if (action.endsWith('.create') || action.endsWith('.upload') || action === 'auth.login') return 'good'
  if (action.includes('command') || action.includes('task') || action.includes('deploy')) return 'accent'
  if (action.includes('password')) return 'warning'
  return 'neutral'
}

export function AuditLog() {
  const [page, setPage] = useState({ limit: 100, offset: 0 })
  const q = useAudit(page)
  const [open, setOpen] = useState<Set<number>>(new Set())
  const toggle = (id: number) =>
    setOpen((s) => {
      const n = new Set(s)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  const rows = q.data?.items
  return (
    <Card>
      <CardHeader title="Audit log" description="Who did what, and when. Expand a row for details." />
      {q.isPending && (
        <div className="flex items-center justify-center gap-2 py-12 text-sm text-ink-muted">
          <Spinner className="size-4" /> Loading…
        </div>
      )}
      {q.error && <p className="px-5 py-4 text-sm text-critical">{errorMessage(q.error)}</p>}
      {rows && rows.length === 0 && <EmptyState title="No audit entries yet" compact />}
      {rows && rows.length > 0 && (
        <div className="overflow-x-auto">
          <table /* ui-allow: raw-table (expandable rows) */ className="w-full text-sm">
            <thead>
              <tr className="border-b border-line text-left text-xs uppercase tracking-wide text-ink-muted">
                <th className="w-8 pl-4 py-2.5" />
                <th className="px-3 py-2.5 font-medium">When</th>
                <th className="px-3 py-2.5 font-medium">Actor</th>
                <th className="px-3 py-2.5 font-medium">Action</th>
                <th className="px-3 py-2.5 font-medium">Target</th>
                <th className="px-3 py-2.5 font-medium pr-5">Summary</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((a: AuditEntry) => {
                const isOpen = open.has(a.id)
                const hasDetails = a.details && Object.keys(a.details).length > 0
                return (
                  <Fragment key={a.id}>
                    <tr onClick={() => hasDetails && toggle(a.id)} className={cn('border-b border-line', hasDetails && 'cursor-pointer hover:bg-surface-sunken/70', isOpen && 'bg-surface-sunken/50')}>
                      <td className="pl-4 py-2 text-ink-faint">{hasDetails ? isOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" /> : null}</td>
                      <td className="px-3 py-2 whitespace-nowrap text-ink-muted" title={absoluteTime(a.at)}>
                        <RelativeTime value={a.at} />
                      </td>
                      <td className="px-3 py-2 font-medium text-ink">{a.actor || 'system'}</td>
                      <td className="px-3 py-2">
                        <Badge tone={actionTone(a.action)} className="font-mono">
                          {a.action}
                        </Badge>
                      </td>
                      <td className="px-3 py-2 text-xs text-ink-muted font-mono">
                        {a.target_type}
                        {a.target_id ? ` ${a.target_id.slice(0, 8)}` : ''}
                      </td>
                      <td className="px-3 py-2 pr-5 text-xs text-ink-muted truncate max-w-md">{summarize(a.details)}</td>
                    </tr>
                    {isOpen && (
                      <tr className="border-b border-line bg-surface-sunken/30">
                        <td colSpan={6} className="px-5 py-3">
                          <pre className="max-h-64 overflow-auto rounded-lg border border-line bg-surface-sunken p-3 text-xs font-mono text-ink">{JSON.stringify({ id: a.id, at: a.at, target_id: a.target_id, ...a.details }, null, 2)}</pre>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      <Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} pageSizes={[50, 100, 200]} />
    </Card>
  )
}

function summarize(d: Record<string, unknown> | null): string {
  if (!d) return ''
  return Object.entries(d)
    .filter(([, v]) => typeof v !== 'object')
    .map(([k, v]) => `${k}: ${String(v)}`)
    .join(' · ')
}
