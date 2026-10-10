import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { Eye, RefreshCw, Undo2, Wrench, X } from 'lucide-react'
import { useHealthChecks, useHealthFinding } from '../../api/queries'
import { errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { HealthEvent } from '../../api/types'
import { Badge } from '../ui/Badge'
import { Button, IconButton } from '../ui/Button'
import { Alert } from '../ui/Alert'
import { LoadingBlock } from '../ui/Spinner'
import { RelativeTime } from '../ui/RelativeTime'
import { EmptyState } from '../ui/EmptyState'
import { absoluteTime, titleCase } from '../../lib/format'
import { EventIcon, FactsList, ObjectName, RemediationList, SeverityBadge, SnoozeMenu, StateBadge, eventMeta, objectTypeName, useHealthActions } from './health'

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="border-t border-line px-5 py-4">
      <h3 className="micro-label mb-2.5">{title}</h3>
      {children}
    </section>
  )
}

export function HealthTimeline({ events }: { events: HealthEvent[] }) {
  if (events.length === 0) return <EmptyState compact title="No events yet" />
  return (
    <ol className="relative flex flex-col">
      {events.map((e, i) => {
        const m = eventMeta(e.event)
        return (
          <li key={e.id} className="relative flex gap-3 pb-3.5 last:pb-0">
            {i < events.length - 1 && <span className="absolute left-[13px] top-7 bottom-0 w-px bg-line" aria-hidden="true" />}
            <EventIcon event={e.event} />
            <div className="min-w-0 flex-1 pt-0.5">
              <div className="flex flex-wrap items-baseline gap-x-2">
                <span className="text-[13px] font-medium text-ink">{m.label}</span>
                <RelativeTime value={e.at} className="text-xs text-ink-muted" />
                {e.actor && <span className="text-xs text-ink-faint">by {e.actor}</span>}
              </div>
              {e.message && <p className="mt-0.5 text-xs text-ink-muted break-words">{e.message}</p>}
            </div>
          </li>
        )
      })}
    </ol>
  )
}

/** Slide-over with everything about one finding: explanation, facts, fixes, state actions, and its history. */
export function FindingDrawer({ id, onClose }: { id: string | null; onClose: () => void }) {
  const { isAdmin } = useAuth()
  const q = useHealthFinding(id ?? undefined)
  const checks = useHealthChecks()
  const { setState, runCheck } = useHealthActions()
  const [busy, setBusy] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [id, onClose])

  if (!id) return null
  const d = q.data
  const f = d?.finding
  const check = f ? checks.data?.find((c) => c.id === f.check_id) : undefined
  const events = [...(d?.events ?? [])].sort((a, b) => b.at.localeCompare(a.at) || b.id - a.id)

  const act = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key)
    await fn()
    setBusy(null)
  }

  return createPortal(
    <div className="fixed inset-0 z-50 animate-fade-in" role="dialog" aria-modal="true" aria-label="Finding details">
      <div className="absolute inset-0 bg-black/45 backdrop-blur-[2px]" onClick={onClose} />
      <aside className="absolute inset-y-0 right-0 flex w-full max-w-xl flex-col border-l border-line-strong/60 bg-surface shadow-2xl">
        <div className="flex items-start justify-between gap-3 px-5 pt-4 pb-3">
          <div className="min-w-0">
            {f ? (
              <>
                <div className="flex flex-wrap items-center gap-1.5">
                  <SeverityBadge severity={f.severity} />
                  <StateBadge f={f} />
                  {f.category && <Badge>{titleCase(f.category)}</Badge>}
                </div>
                <p className="mt-2 text-xs text-ink-muted">{f.check_title || f.check_id}</p>
                <h2 className="mt-0.5 text-base font-semibold text-ink break-words">{f.summary}</h2>
                <p className="mt-1 text-xs text-ink-muted">
                  {objectTypeName(f.object_type)}: <ObjectName f={f} className="text-xs" />
                </p>
              </>
            ) : (
              <h2 className="text-base font-semibold text-ink">Finding</h2>
            )}
          </div>
          <IconButton label="Close" variant="ghost" onClick={onClose} className="-mr-2">
            <X />
          </IconButton>
        </div>

        <div className="flex-1 overflow-y-auto">
          {q.isPending && <LoadingBlock />}
          {q.error && (
            <div className="px-5 pb-4">
              <Alert tone="critical">{errorMessage(q.error)}</Alert>
            </div>
          )}
          {f && d && (
            <>
              {isAdmin && (
                <div className="flex flex-wrap items-center gap-2 px-5 pb-4">
                  {f.state === 'open' && (
                    <Button size="sm" icon={<Eye />} loading={busy === 'ack'} onClick={() => act('ack', () => setState(f, 'acknowledged'))}>
                      Acknowledge
                    </Button>
                  )}
                  {(f.state === 'open' || f.state === 'acknowledged') && <SnoozeMenu onSnooze={(h) => act('snooze', () => setState(f, 'snoozed', h))} disabled={busy === 'snooze'} />}
                  {(f.state === 'acknowledged' || f.state === 'snoozed') && (
                    <Button size="sm" icon={<Undo2 />} loading={busy === 'open'} onClick={() => act('open', () => setState(f, 'open'))}>
                      Reopen
                    </Button>
                  )}
                  {f.state !== 'resolved' && (
                    <Button size="sm" variant="ghost" icon={<RefreshCw />} loading={busy === 'run'} onClick={() => act('run', () => runCheck(f.check_id, f.check_title))} title="Run this check now. If the condition has cleared, the finding resolves immediately.">
                      Re-check now
                    </Button>
                  )}
                </div>
              )}

              {(f.detail || d.check_description) && (
                <Section title="What this means">
                  {f.detail && <p className="text-[13px] leading-relaxed text-ink whitespace-pre-line">{f.detail}</p>}
                  {d.check_description && <p className={f.detail ? 'mt-2 text-xs text-ink-muted' : 'text-[13px] text-ink-muted'}>{d.check_description}</p>}
                </Section>
              )}

              <Section title="How to fix">
                <RemediationList finding={f} canAct={isAdmin} autoHeal={check?.auto_heal} />
                {f.heal_attempts > 0 && (
                  <p className="mt-2.5 inline-flex items-start gap-1.5 text-xs text-ink-muted">
                    <Wrench className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
                    <span>
                      {f.heal_attempts} automatic {f.heal_attempts === 1 ? 'attempt' : 'attempts'}
                      {f.last_heal_at && (
                        <>
                          , last <RelativeTime value={f.last_heal_at} />
                        </>
                      )}
                      {f.last_heal_error && <span className="block text-critical">{f.last_heal_error}</span>}
                    </span>
                  </p>
                )}
              </Section>

              {f.facts && Object.keys(f.facts).length > 0 && (
                <Section title="Details">
                  <FactsList facts={f.facts} />
                </Section>
              )}

              <Section title="Lifecycle">
                <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-[13px] sm:grid-cols-3">
                  <div>
                    <dt className="text-xs text-ink-muted">First seen</dt>
                    <dd title={absoluteTime(f.first_seen)}>
                      <RelativeTime value={f.first_seen} />
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-ink-muted">Last seen</dt>
                    <dd>
                      <RelativeTime value={f.last_seen} />
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-ink-muted">Occurrences</dt>
                    <dd className="tabular-nums">{f.occurrences.toLocaleString()}</dd>
                  </div>
                  {f.resolved_at && (
                    <div>
                      <dt className="text-xs text-ink-muted">Resolved</dt>
                      <dd>
                        <RelativeTime value={f.resolved_at} />
                      </dd>
                    </div>
                  )}
                  {f.acknowledged_by && (
                    <div>
                      <dt className="text-xs text-ink-muted">Acknowledged by</dt>
                      <dd>{f.acknowledged_by}</dd>
                    </div>
                  )}
                </dl>
              </Section>

              <Section title="Timeline">
                <HealthTimeline events={events} />
              </Section>
            </>
          )}
        </div>
      </aside>
    </div>,
    document.body,
  )
}
