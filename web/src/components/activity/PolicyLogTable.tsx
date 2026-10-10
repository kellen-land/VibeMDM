import { Fragment, useState } from 'react'
import { Link } from 'react-router-dom'
import { AppWindow, ChevronDown, ChevronRight, ClipboardList, FileCode, Package, RotateCcw, Terminal, type LucideIcon } from 'lucide-react'
import type { PolicyLog, PolicyStep } from '../../api/types'
import { Badge, StatusBadge, type BadgeTone } from '../ui/Badge'
import { RelativeTime } from '../ui/RelativeTime'
import { EmptyState } from '../ui/EmptyState'
import { Spinner } from '../ui/Spinner'
import { formatMs, titleCase } from '../../lib/format'
import { cn } from '../../lib/cn'

export function PolicyLogTable({
  logs,
  loading,
  showPolicy = true,
  showComputer = true,
  emptyTitle = 'No policy runs recorded',
}: {
  logs: PolicyLog[] | undefined
  loading?: boolean
  showPolicy?: boolean
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
  if (!logs || logs.length === 0) return <EmptyState title={emptyTitle} compact />
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-line text-left text-xs uppercase tracking-wide text-ink-muted">
            <th className="w-8 pl-4 py-2.5" />
            {showPolicy && <th className="px-3 py-2.5 font-medium">Policy</th>}
            {showComputer && <th className="px-3 py-2.5 font-medium">Computer</th>}
            <th className="px-3 py-2.5 font-medium">Trigger</th>
            <th className="px-3 py-2.5 font-medium">Status</th>
            <th className="px-3 py-2.5 font-medium">Steps</th>
            <th className="px-3 py-2.5 font-medium pr-5">When</th>
          </tr>
        </thead>
        <tbody>
          {logs.map((l) => {
            const isOpen = open.has(l.id)
            const steps = l.steps ?? []
            const failedSteps = steps.filter((s) => s.status === 'failed').length
            return (
              <Fragment key={l.id}>
                <tr onClick={() => toggle(l.id)} className={cn('cursor-pointer border-b border-line transition-colors hover:bg-surface-sunken/70', isOpen && 'bg-surface-sunken/50')}>
                  <td className="pl-4 py-2.5 text-ink-faint">{isOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}</td>
                  {showPolicy && (
                    <td className="px-3 py-2.5">
                      <Link to={`/policies/${l.policy_id}`} onClick={(e) => e.stopPropagation()} className="font-medium text-ink hover:underline">
                        {l.policy_name || 'Deleted policy'}
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
                    <Badge>{titleCase(l.trigger || 'manual')}</Badge>
                  </td>
                  <td className="px-3 py-2.5">
                    <StatusBadge status={l.status} />
                  </td>
                  <td className="px-3 py-2.5 text-xs text-ink-muted tabular-nums">
                    {steps.length} {failedSteps > 0 && <span className="text-critical">({failedSteps} failed)</span>}
                  </td>
                  <td className="px-3 py-2.5 pr-5 text-ink-muted">
                    <RelativeTime value={l.completed_at ?? l.started_at ?? l.created_at} />
                  </td>
                </tr>
                {isOpen && (
                  <tr className="border-b border-line bg-surface-sunken/30">
                    <td colSpan={5 + (showPolicy ? 1 : 0) + (showComputer ? 1 : 0)} className="px-5 py-4">
                      <PolicyLogDetail log={l} />
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

export function PolicyLogDetail({ log }: { log: PolicyLog }) {
  const steps = log.steps ?? []
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-ink-muted">
        <span>
          Started: <RelativeTime value={log.started_at} className="text-ink" />
        </span>
        <span>
          Completed: <RelativeTime value={log.completed_at} className="text-ink" />
        </span>
        <span className="font-mono">Log ID: {log.id}</span>
      </div>
      {steps.length === 0 ? (
        <p className="text-xs text-ink-faint">{log.status === 'pending' || log.status === 'running' ? 'Waiting for the agent to report results.' : 'No step details recorded.'}</p>
      ) : (
        <div className="flex flex-col gap-2">
          {steps.map((s, i) => (
            <StepRow key={i} step={s} />
          ))}
        </div>
      )}
      {log.log && (
        <details className="group">
          <summary className="cursor-pointer text-xs font-medium text-ink-muted hover:text-ink">Raw log</summary>
          <pre className="mt-2 max-h-72 overflow-auto rounded-lg border border-line bg-surface-sunken p-3 text-xs font-mono whitespace-pre-wrap text-ink">{log.log}</pre>
        </details>
      )}
    </div>
  )
}

const stepMeta: Record<string, { label: string; icon: LucideIcon; tone?: BadgeTone }> = {
  script: { label: 'Script', icon: FileCode },
  package: { label: 'Package', icon: Package },
  app: { label: 'Installomator', icon: AppWindow, tone: 'accent' },
  command: { label: 'Command', icon: Terminal },
  inventory: { label: 'Inventory', icon: ClipboardList },
  reboot: { label: 'Restart', icon: RotateCcw },
}

function StepTypeBadge({ type }: { type: string }) {
  const m = stepMeta[type]
  if (!m) return <Badge className="uppercase">{type}</Badge>
  return (
    <Badge tone={m.tone ?? 'neutral'} icon={<m.icon />}>
      {m.label}
    </Badge>
  )
}

function StepRow({ step }: { step: PolicyStep }) {
  const [open, setOpen] = useState(step.status === 'failed')
  return (
    <div className="rounded-lg border border-line bg-surface-raised">
      <button type="button" onClick={() => setOpen((o) => !o)} className="flex w-full items-center gap-3 px-3 py-2 text-left">
        <span className="text-ink-faint">{open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}</span>
        <StepTypeBadge type={step.type} />
        <span className="min-w-0 flex-1 truncate text-sm text-ink">{step.name || titleCase(step.type)}</span>
        <StatusBadge status={step.status} />
        <span className="text-xs text-ink-muted tabular-nums w-20 text-right">exit {step.exit_code}</span>
        <span className="text-xs text-ink-muted tabular-nums w-16 text-right">{formatMs(step.duration_ms)}</span>
      </button>
      {open && (
        <pre className="max-h-72 overflow-auto border-t border-line bg-surface-sunken px-3 py-2 text-xs font-mono whitespace-pre-wrap text-ink rounded-b-lg">{step.output || <span className="text-ink-faint">(no output)</span>}</pre>
      )}
    </div>
  )
}
