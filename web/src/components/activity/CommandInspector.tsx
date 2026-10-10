import { useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import {
  ArrowDownToLine,
  Ban,
  BellOff,
  BellRing,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock,
  Eye,
  EyeOff,
  ExternalLink,
  Inbox,
  Plug,
  ShieldAlert,
  XCircle,
  type LucideIcon,
} from 'lucide-react'
import { useCommandInspection } from '../../api/queries'
import { errorMessage } from '../../api/client'
import type { CommandEvent, MDMCommand } from '../../api/types'
import { Badge, StatusBadge, badgeToneClass, type BadgeTone } from '../ui/Badge'
import { Button } from '../ui/Button'
import { Modal } from '../ui/Modal'
import { Tabs } from '../ui/Tabs'
import { LoadingBlock } from '../ui/Spinner'
import { Alert } from '../ui/Alert'
import { CodeBlock, CopyButton } from '../ui/CopyButton'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { EmptyState } from '../ui/EmptyState'
import { RelativeTime } from '../ui/RelativeTime'
import { CodeEditor } from '../CodeEditor'
import { absoluteTime, timeWithMs } from '../../lib/format'
import { cn } from '../../lib/cn'

/** Status shown for a command row: inactive Pending commands were cancelled by an admin. */
export function commandStatus(c: Pick<MDMCommand, 'active' | 'status'>): string {
  return c.active || c.status !== 'Pending' ? c.status : 'Cancelled'
}

interface EventStyle {
  tone: BadgeTone
  icon: LucideIcon
  label: string
}

const eventStyles: Record<string, EventStyle> = {
  queued: { tone: 'neutral', icon: Inbox, label: 'Queued' },
  push_sent: { tone: 'accent', icon: BellRing, label: 'Push sent' },
  push_failed: { tone: 'warning', icon: BellOff, label: 'Push failed' },
  connected: { tone: 'info', icon: Plug, label: 'Connected' },
  delivered: { tone: 'accent', icon: ArrowDownToLine, label: 'Delivered' },
  acknowledged: { tone: 'good', icon: CheckCircle2, label: 'Acknowledged' },
  error: { tone: 'critical', icon: XCircle, label: 'Error' },
  not_now: { tone: 'warning', icon: Clock, label: 'Not now' },
  cancelled: { tone: 'neutral', icon: Ban, label: 'Cancelled' },
}

export function eventStyle(event: string): EventStyle {
  return eventStyles[event] ?? { tone: 'neutral', icon: Inbox, label: event.replace(/_/g, ' ') }
}

export function EventBadge({ event }: { event: string }) {
  const s = eventStyle(event)
  const Icon = s.icon
  return (
    <Badge tone={s.tone} icon={<Icon />} className={event === 'cancelled' ? 'opacity-70' : undefined}>
      {s.label}
    </Badge>
  )
}

/** Vertical timeline of a command's events, oldest first. */
export function CommandTimeline({ events }: { events: CommandEvent[] }) {
  if (events.length === 0) return <EmptyState compact title="No events recorded" description="Events are recorded for commands queued after the inspector was enabled, and kept for 90 days." />
  return (
    <ol className="relative flex flex-col">
      {events.map((e, i) => (
        <TimelineItem key={e.id} e={e} last={i === events.length - 1} />
      ))}
    </ol>
  )
}

function TimelineItem({ e, last }: { e: CommandEvent; last: boolean }) {
  const [open, setOpen] = useState(false)
  const s = eventStyle(e.event)
  const Icon = s.icon
  const hasDetails = !!e.details && Object.keys(e.details).length > 0
  return (
    <li className="relative flex gap-3 pb-4 last:pb-0">
      {!last && <span className="absolute left-[13px] top-7 bottom-0 w-px bg-line" aria-hidden="true" />}
      <span className={cn('relative z-[1] flex size-7 shrink-0 items-center justify-center rounded-full border [&>svg]:size-3.5', badgeToneClass[s.tone], e.event === 'cancelled' && 'opacity-70')}>
        <Icon />
      </span>
      <div className="min-w-0 flex-1 pt-0.5">
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <span className="text-sm font-medium text-ink">{s.label}</span>
          <time dateTime={e.at} title={absoluteTime(e.at)} className="font-mono text-xs tabular-nums text-ink-muted">
            {timeWithMs(e.at)}
          </time>
        </div>
        <p className="mt-0.5 text-sm text-ink-muted">{e.description}</p>
        {hasDetails && (
          <div className="mt-1">
            <button type="button" onClick={() => setOpen((o) => !o)} className="inline-flex items-center gap-1 text-xs text-ink-muted hover:text-ink focus-ring rounded">
              {open ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
              Details
            </button>
            {open && <CodeBlock className="mt-1.5" text={JSON.stringify(e.details, null, 2)} maxHeight="14rem" />}
          </div>
        )}
      </div>
    </li>
  )
}

/** Collapsible study aid: how an Apple MDM command moves (docs/inspector.md). */
export function HowCommandTravels({ className }: { className?: string }) {
  const steps: { step: string; events: string[]; what: ReactNode }[] = [
    { step: '1', events: ['queued'], what: <>The server stored the command in the device&apos;s queue (NanoMDM <code className="font-mono text-xs">enrollment_queue</code>).</> },
    {
      step: '2',
      events: ['push_sent', 'push_failed'],
      what: (
        <>
          The server sent an APNs push containing only the device&apos;s <em>PushMagic</em>: &ldquo;come check in&rdquo;. APNs never carries the command itself.
        </>
      ),
    },
    { step: '3', events: ['delivered'], what: <>The device connected to <code className="font-mono text-xs">/mdm</code> (reporting <code className="font-mono text-xs">Idle</code> or a previous result) and the server answered with the next queued command plist.</> },
    { step: '4', events: ['acknowledged', 'error', 'not_now'], what: <>The device reported the result in its next request. <code className="font-mono text-xs">NotNow</code> means &ldquo;busy or locked, try again later&rdquo; and the command stays queued.</> },
    { step: '—', events: ['cancelled'], what: 'An administrator deactivated the command before it was delivered.' },
  ]
  return (
    <details className={cn('group rounded-lg border border-line bg-surface-sunken/40', className)}>
      <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-sm font-medium text-ink select-none [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-4 text-ink-faint transition-transform group-open:rotate-90" />
        How a command travels
      </summary>
      <div className="border-t border-line px-3 py-3">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left">
                <th className="micro-label pb-2 pr-3 font-normal">Step</th>
                <th className="micro-label pb-2 pr-3 font-normal">Event</th>
                <th className="micro-label pb-2 font-normal">What happened</th>
              </tr>
            </thead>
            <tbody>
              {steps.map((s) => (
                <tr key={s.step} className="border-t border-line align-top">
                  <td className="py-2 pr-3 tabular-nums text-ink-muted">{s.step}</td>
                  <td className="py-2 pr-3">
                    <div className="flex flex-wrap gap-1">
                      {s.events.map((ev) => (
                        <EventBadge key={ev} event={ev} />
                      ))}
                    </div>
                  </td>
                  <td className="py-2 text-ink-muted">{s.what}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mt-3 text-xs text-ink-muted">
          A device keeps connecting until its queue is empty: each response carries the next command, and an empty response ends the session. That is why several delivered/acknowledged pairs often appear within a second.
        </p>
      </div>
    </details>
  )
}

/**
 * The inspector body: header, summaries, timeline, and request/response plists.
 * Raw payloads are fetched only after an audited confirm and are dropped from the cache on unmount.
 */
export function CommandInspector({ uuid, showOpenPage }: { uuid: string; showOpenPage?: boolean }) {
  const [reveal, setReveal] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [plistTab, setPlistTab] = useState<'request' | 'response'>('request')
  const q = useCommandInspection(uuid, reveal)
  const d = q.data

  if (q.isPending) return <LoadingBlock />
  if (q.error || !d) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'Command not found'}</Alert>
  const c = d.command
  const events = [...(d.events ?? [])].sort((a, b) => a.at.localeCompare(b.at) || a.id - b.id)
  const plist = plistTab === 'request' ? c.command : c.result

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-lg font-semibold text-ink">{c.request_type}</h2>
          <StatusBadge status={commandStatus(c)} />
          <Badge>{c.kind || 'management'}</Badge>
          {d.redacted ? (
            <Badge tone="warning" icon={<EyeOff />}>
              Redacted
            </Badge>
          ) : (
            <Badge tone="critical" icon={<ShieldAlert />}>
              Raw payloads (audited)
            </Badge>
          )}
          <span className="ml-auto flex items-center gap-2">
            {d.can_reveal &&
              (d.redacted ? (
                <Button size="sm" icon={<Eye />} onClick={() => setConfirmOpen(true)}>
                  Reveal raw payloads
                </Button>
              ) : (
                <Button size="sm" variant="ghost" icon={<EyeOff />} onClick={() => setReveal(false)}>
                  Hide raw payloads
                </Button>
              ))}
            {showOpenPage && (
              <Link to={`/commands/${c.command_uuid}`} className="inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-[13px] text-ink-muted hover:bg-line/40 hover:text-ink focus-ring">
                <ExternalLink className="size-4" /> Full page
              </Link>
            )}
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-muted">
          {c.computer_id && (
            <span>
              Computer{' '}
              <Link to={`/computers/${c.computer_id}`} className="text-ink hover:underline">
                {c.computer_name || c.computer_id}
              </Link>
            </span>
          )}
          <span className="inline-flex items-center gap-1">
            UUID <span className="font-mono text-ink">{c.command_uuid}</span>
            <CopyButton text={c.command_uuid} size="xs" variant="ghost" label="" aria-label="Copy command UUID" />
          </span>
          <span>Sent by {c.created_by || 'system'}</span>
          <span>
            Created <RelativeTime value={c.created_at} />
          </span>
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="rounded-lg border border-line px-3 py-2.5">
          <p className="micro-label">Asks the device to</p>
          <p className="mt-1 text-sm text-ink">{d.summary || '—'}</p>
        </div>
        <div className="rounded-lg border border-line px-3 py-2.5">
          <p className="micro-label">Result</p>
          <p className="mt-1 text-sm text-ink">{d.result_summary || '—'}</p>
        </div>
      </div>

      <section>
        <h3 className="mb-3 text-sm font-semibold text-ink">Timeline</h3>
        <CommandTimeline events={events} />
      </section>

      <HowCommandTravels />

      <section className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Tabs
            variant="pills"
            value={plistTab}
            onChange={setPlistTab}
            items={[
              { key: 'request', label: 'Request plist' },
              { key: 'response', label: 'Response plist' },
            ]}
          />
          {d.redacted && <p className="text-xs text-ink-muted">Sensitive values are replaced with placeholders such as [redacted 6 bytes].</p>}
        </div>
        {plist ? (
          <CodeEditor key={`${plistTab}-${d.redacted}`} language="xml" value={plist} readOnly editable={false} height="360px" />
        ) : (
          <EmptyState compact title={plistTab === 'request' ? 'Request not available' : 'No response yet'} />
        )}
      </section>

      <ConfirmDialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        onConfirm={() => {
          setReveal(true)
          setConfirmOpen(false)
        }}
        title="Reveal raw payloads?"
        description="The unredacted request and response may contain PINs, passwords, tokens and profile payloads. This reveal is written to the audit log under your name."
        confirmLabel="Reveal"
      />
    </div>
  )
}

/** Modal wrapper used from command tables and the computer timeline. */
export function CommandInspectorModal({ uuid, onClose }: { uuid: string | null; onClose: () => void }) {
  return (
    <Modal open={!!uuid} onClose={onClose} title="Command inspector" size="xl">
      {uuid && <CommandInspector key={uuid} uuid={uuid} showOpenPage />}
    </Modal>
  )
}
