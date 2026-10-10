import { Fragment, useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown, ChevronRight, XCircle } from 'lucide-react'
import type { AgentTask } from '../../api/types'
import { StatusBadge } from '../ui/Badge'
import { RelativeTime } from '../ui/RelativeTime'
import { EmptyState } from '../ui/EmptyState'
import { Spinner } from '../ui/Spinner'
import { Button } from '../ui/Button'
import { cn } from '../../lib/cn'
import { taskLabel } from '../../pages/DashboardPage'
import { absoluteTime } from '../../lib/format'

export function TaskTable({
  tasks,
  loading,
  showComputer = true,
  onCancel,
  canCancel,
  emptyTitle = 'No agent tasks',
}: {
  tasks: AgentTask[] | undefined
  loading?: boolean
  showComputer?: boolean
  onCancel?: (t: AgentTask) => void
  canCancel?: boolean
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
  if (loading && !tasks) {
    return (
      <div className="flex items-center justify-center gap-2 py-12 text-sm text-ink-muted">
        <Spinner className="size-4" /> Loading…
      </div>
    )
  }
  if (!tasks || tasks.length === 0) return <EmptyState title={emptyTitle} compact />
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-line text-left text-xs uppercase tracking-wide text-ink-muted">
            <th className="w-8 pl-4 py-2.5" />
            <th className="px-3 py-2.5 font-medium">Task</th>
            {showComputer && <th className="px-3 py-2.5 font-medium">Computer</th>}
            <th className="px-3 py-2.5 font-medium">Created by</th>
            <th className="px-3 py-2.5 font-medium">Status</th>
            <th className="px-3 py-2.5 font-medium">Created</th>
            <th className="px-3 py-2.5 font-medium">Completed</th>
            {onCancel && <th className="px-3 py-2.5 pr-5" />}
          </tr>
        </thead>
        <tbody>
          {tasks.map((t) => {
            const isOpen = open.has(t.id)
            const cancellable = t.status === 'queued' || t.status === 'sent'
            return (
              <Fragment key={t.id}>
                <tr onClick={() => toggle(t.id)} className={cn('cursor-pointer border-b border-line transition-colors hover:bg-surface-sunken/70', isOpen && 'bg-surface-sunken/50')}>
                  <td className="pl-4 py-2.5 text-ink-faint">{isOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}</td>
                  <td className="px-3 py-2.5 font-medium text-ink">{taskLabel(t)}</td>
                  {showComputer && (
                    <td className="px-3 py-2.5">
                      <Link to={`/computers/${t.computer_id}`} onClick={(e) => e.stopPropagation()} className="text-ink hover:underline">
                        {t.computer_name || 'Deleted computer'}
                      </Link>
                    </td>
                  )}
                  <td className="px-3 py-2.5 text-ink-muted">{t.created_by || 'system'}</td>
                  <td className="px-3 py-2.5">
                    <StatusBadge status={t.status} />
                  </td>
                  <td className="px-3 py-2.5 text-ink-muted">
                    <RelativeTime value={t.created_at} />
                  </td>
                  <td className="px-3 py-2.5 text-ink-muted">
                    <RelativeTime value={t.completed_at} emptyLabel="—" />
                  </td>
                  {onCancel && (
                    <td className="px-3 py-2.5 pr-5 text-right">
                      {cancellable && canCancel && (
                        <Button
                          size="xs"
                          variant="ghost"
                          icon={<XCircle />}
                          onClick={(e) => {
                            e.stopPropagation()
                            onCancel(t)
                          }}
                        >
                          Cancel
                        </Button>
                      )}
                    </td>
                  )}
                </tr>
                {isOpen && (
                  <tr className="border-b border-line bg-surface-sunken/30">
                    <td colSpan={6 + (showComputer ? 1 : 0) + (onCancel ? 1 : 0)} className="px-5 py-4">
                      <TaskDetail task={t} />
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

export function TaskDetail({ task }: { task: AgentTask }) {
  const r = task.result
  const payload = { ...(task.payload ?? {}) }
  // Hide bulky script contents / policy bodies from the payload summary.
  delete payload.contents
  delete payload.policy
  return (
    <div className="flex flex-col gap-3 text-xs">
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-ink-muted">
        <span className="font-mono">Task ID: {task.id}</span>
        {task.sent_at && <span>Sent: {absoluteTime(task.sent_at)}</span>}
        {r?.exit_code !== undefined && (
          <span>
            Exit code: <span className={cn('font-mono', r.exit_code === 0 ? 'text-ink' : 'text-critical')}>{r.exit_code}</span>
          </span>
        )}
      </div>
      {Object.keys(payload).length > 0 && (
        <div>
          <p className="mb-1 font-medium text-ink-muted">Payload</p>
          <pre className="max-h-40 overflow-auto rounded-lg border border-line bg-surface-sunken p-3 font-mono whitespace-pre-wrap text-ink">{JSON.stringify(payload, null, 2)}</pre>
        </div>
      )}
      {r?.error && (
        <div>
          <p className="mb-1 font-medium text-critical">Error</p>
          <pre className="rounded-lg border border-critical/30 bg-critical/5 p-3 font-mono whitespace-pre-wrap text-ink">{r.error}</pre>
        </div>
      )}
      {(r?.stdout !== undefined || r?.stderr !== undefined) && (
        <div className="grid gap-3 md:grid-cols-2">
          <div>
            <p className="mb-1 font-medium text-ink-muted">stdout</p>
            <pre className="max-h-72 overflow-auto rounded-lg border border-line bg-surface-sunken p-3 font-mono whitespace-pre-wrap text-ink">{r?.stdout || <span className="text-ink-faint">(empty)</span>}</pre>
          </div>
          <div>
            <p className="mb-1 font-medium text-ink-muted">stderr</p>
            <pre className="max-h-72 overflow-auto rounded-lg border border-line bg-surface-sunken p-3 font-mono whitespace-pre-wrap text-ink">{r?.stderr || <span className="text-ink-faint">(empty)</span>}</pre>
          </div>
        </div>
      )}
      {!r && <p className="text-ink-faint">{task.status === 'queued' ? 'Waiting for the agent to pick up this task.' : task.status === 'sent' ? 'Sent to the agent; awaiting result.' : 'No result recorded.'}</p>}
    </div>
  )
}
