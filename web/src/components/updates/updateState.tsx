import type { ReactNode } from 'react'
import { AlertCircle, CheckCircle2, CircleDashed, CircleOff, Clock, HelpCircle, type LucideIcon } from 'lucide-react'
import type { UpdateState } from '../../api/types'
import { Badge, type BadgeTone } from '../ui/Badge'
import { Alert } from '../ui/Alert'
import { cn } from '../../lib/cn'

export interface UpdateStateMeta {
  label: string
  tone: BadgeTone
  icon: LucideIcon
  /** Legend wording, from api.updateRowFor. */
  meaning: string
  /** Fill for the compliance bar / tile accent. Status colors only for states that mean good/bad. */
  fill: string
  /** Icon color class. */
  ink: string
}

/** Order used everywhere: worst-to-best attention first, then the neutral states. */
export const updateStates: UpdateState[] = ['failed', 'pending', 'up_to_date', 'not_targeted', 'no_ddm', 'unsupported']

export const updateStateMeta: Record<UpdateState, UpdateStateMeta> = {
  up_to_date: { label: 'Up to date', tone: 'good', icon: CheckCircle2, meaning: 'At or above the enforcement target version.', fill: 'bg-good', ink: 'text-good' },
  pending: { label: 'Pending', tone: 'warning', icon: Clock, meaning: 'Below the target version; the enforcement declaration has been delivered.', fill: 'bg-warning', ink: 'text-warning' },
  failed: { label: 'Failed', tone: 'critical', icon: AlertCircle, meaning: 'The Mac reported a software update failure reason.', fill: 'bg-critical', ink: 'text-critical' },
  not_targeted: { label: 'Not targeted', tone: 'neutral', icon: CircleDashed, meaning: 'No enabled enforcement plan has this Mac in scope.', fill: 'bg-ink-faint', ink: 'text-ink-faint' },
  no_ddm: { label: 'No DDM', tone: 'neutral', icon: HelpCircle, meaning: 'No DDM status report yet, or the Mac is not MDM enrolled.', fill: 'bg-line-strong', ink: 'text-ink-faint' },
  unsupported: { label: 'Unsupported', tone: 'neutral', icon: CircleOff, meaning: 'macOS older than 14, which cannot receive update enforcement declarations.', fill: 'hatch-neutral', ink: 'text-ink-faint' },
}

export function UpdateStateBadge({ state, className }: { state: UpdateState | string; className?: string }) {
  const m = updateStateMeta[state as UpdateState]
  if (!m) return <Badge className={className}>{state || 'Unknown'}</Badge>
  const Icon = m.icon
  return (
    <Badge tone={m.tone} icon={<Icon />} className={className}>
      {m.label}
    </Badge>
  )
}

/** Turn a DDM local date-time ("2026-11-01T17:00:00", no zone) into readable text without shifting zones. */
export function formatLocalDeadline(v: string | null | undefined): string {
  if (!v) return ''
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(v)
  if (!m) return v
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]), Number(m[4]), Number(m[5]), Number(m[6] ?? 0))
  return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

/** softwareupdate.pending-version is {"os-version", "build-version"}; fall back to any string values. */
export function pendingVersionLabel(pv: Record<string, unknown> | null | undefined): string {
  if (!pv || Object.keys(pv).length === 0) return ''
  const v = pv['os-version'] ?? pv['version']
  const b = pv['build-version'] ?? pv['build']
  if (typeof v === 'string' || typeof b === 'string') return [v, b ? `(${String(b)})` : ''].filter(Boolean).join(' ')
  return Object.values(pv).filter((x) => typeof x === 'string' || typeof x === 'number').join(' ')
}

/** softwareupdate.failure-reason is {"reason", "count", "timestamp"}. */
export function failureReasonLabel(fr: Record<string, unknown> | null | undefined): string {
  if (!fr || Object.keys(fr).length === 0) return ''
  const reason = typeof fr.reason === 'string' ? fr.reason : ''
  const count = typeof fr.count === 'number' && fr.count > 1 ? ` (${fr.count}×)` : ''
  return reason ? reason + count : JSON.stringify(fr)
}

export function installStateLabel(s: string): string {
  if (!s) return ''
  return s.replace(/-/g, ' ').replace(/^\w/, (c) => c.toUpperCase())
}

export function StateLegend({ className }: { className?: string }) {
  return (
    <dl className={cn('grid gap-x-6 gap-y-2.5 sm:grid-cols-2 lg:grid-cols-3', className)}>
      {updateStates.map((s) => {
        const m = updateStateMeta[s]
        return (
          <div key={s} className="flex items-start gap-2">
            <m.icon className={cn('mt-0.5 size-4 shrink-0', m.ink)} aria-hidden="true" />
            <div>
              <dt className="text-[13px] font-medium text-ink">{m.label}</dt>
              <dd className="text-xs text-ink-muted">{m.meaning}</dd>
            </div>
          </div>
        )
      })}
    </dl>
  )
}

export function SupervisionCallout({ className, children }: { className?: string; children?: ReactNode }) {
  return (
    <Alert tone="info" title="Check declaration status on Macs enrolled without Apple Business Manager" className={className}>
      Apple&apos;s schema lists the software update declarations as allowed on <span className="font-mono">supervised</span> enrollments on macOS. That most likely means device
      enrollment rather than strictly Automated Device Enrollment, but it hasn&apos;t been confirmed yet. Each Mac&apos;s status report says whether the enforcement declaration
      is <strong>valid</strong> and <strong>active</strong>; open a computer&apos;s Updates tab to confirm it was accepted.
      {children}
    </Alert>
  )
}
