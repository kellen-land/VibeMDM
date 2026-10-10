import { useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { BookOpen, CheckCircle2, ChevronRight, CircleDashed, RefreshCw } from 'lucide-react'
import type { ComputerDDM, DDMDeclaration } from '../../api/types'
import { api, errorMessage } from '../../api/client'
import { qk, useComputerDDM, useInvalidate } from '../../api/queries'
import { useAuth } from '../../auth/AuthContext'
import { CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Alert } from '../../components/ui/Alert'
import { CodeBlock } from '../../components/ui/CopyButton'
import { DescriptionList } from '../../components/ui/DescriptionList'
import { EmptyState } from '../../components/ui/EmptyState'
import { LoadingBlock } from '../../components/ui/Spinner'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'
import { failureReasonLabel, formatLocalDeadline, installStateLabel, pendingVersionLabel, SupervisionCallout, UpdateStateBadge } from '../../components/updates/updateState'
import { absoluteTime } from '../../lib/format'
import { cn } from '../../lib/cn'

/** One entry of a device's per-declaration status ("valid" is a string: valid | invalid | unknown). */
interface DeclStatus {
  identifier?: string
  active?: boolean
  valid?: string | boolean
  'server-token'?: string
  reasons?: { code?: string; description?: string; details?: unknown }[]
}

type Obj = Record<string, unknown>
const isObj = (v: unknown): v is Obj => !!v && typeof v === 'object' && !Array.isArray(v)

/**
 * Index the device-reported declaration status by identifier. The server stores the report's top-level
 * "Declarations"; Apple also nests the same lists under StatusItems.management.declarations, so read both.
 */
function declarationStatusIndex(d: ComputerDDM): Map<string, DeclStatus & { kind: string }> {
  const out = new Map<string, DeclStatus & { kind: string }>()
  const sources: unknown[] = [d.state?.declarations_status]
  const mgmt = d.state?.status?.management
  if (isObj(mgmt)) sources.push(mgmt.declarations)
  for (const src of sources) {
    if (!isObj(src)) continue
    for (const kind of ['activations', 'configurations', 'assets', 'management']) {
      const list = src[kind]
      if (!Array.isArray(list)) continue
      for (const item of list) {
        if (isObj(item) && typeof item.identifier === 'string' && !out.has(item.identifier)) out.set(item.identifier, { ...(item as DeclStatus), kind })
      }
    }
  }
  return out
}

export function ComputerUpdatesTab({ id }: { id: string }) {
  const q = useComputerDDM(id)
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [syncing, setSyncing] = useState(false)

  const syncNow = async () => {
    setSyncing(true)
    try {
      await api.post(`/api/v1/computers/${id}/ddm/sync`)
      toast.success('DeclarativeManagement command queued', 'The Mac will check in and fetch any changed declarations.')
      await invalidate(qk.computerDDM(id), qk.computerCommands(id))
    } catch (e) {
      toast.error('Sync failed', errorMessage(e))
    } finally {
      setSyncing(false)
    }
  }

  if (q.isPending) return <LoadingBlock />
  if (q.error || !q.data) return <Alert tone="critical" className="m-4">{q.error ? errorMessage(q.error) : 'DDM state unavailable'}</Alert>
  const d = q.data
  const s = d.summary
  const st = d.state
  const declarations = d.declarations ?? []
  const statusIdx = declarationStatusIndex(d)
  const inSync = !!st?.served_token && st.served_token === d.declarations_token
  const failure = failureReasonLabel(s.failure_reason)

  return (
    <div className="divide-y divide-line">
      <section>
        <CardHeader title="Software update" description="Effective plans for this Mac and what it last reported." actions={<UpdateStateBadge state={s.state} />} className="border-b-0" />
        <CardBody className="pt-1 flex flex-col gap-4">
          <DescriptionList
            cols={3}
            items={[
              { label: 'macOS', value: s.os_version ? `${s.os_version}${s.os_build ? ` (${s.os_build})` : ''}` : '' },
              { label: 'Enforcement plan', value: s.enforcement_plan },
              { label: 'Target', value: s.target_os_version ? `macOS ${s.target_os_version}` : '' },
              { label: 'Deadline (Mac local time)', value: formatLocalDeadline(s.deadline) },
              { label: 'Settings plan', value: s.settings_plan },
              { label: 'Install state', value: installStateLabel(s.install_state) },
              { label: 'Pending version', value: pendingVersionLabel(s.pending_version) },
              { label: 'Last status', value: <RelativeTime value={s.last_status_at} /> },
            ]}
          />
          {failure && (
            <Alert tone="critical" title="The Mac reported an update failure">
              <span className="font-mono text-xs">{failure}</span>
            </Alert>
          )}
        </CardBody>
      </section>

      <section>
        <CardHeader
          title="Sync status"
          description="Compares the DeclarationsToken for the current declaration set with the token this Mac last fetched."
          actions={
            isAdmin && (
              <Button size="sm" icon={<RefreshCw />} loading={syncing} onClick={syncNow} disabled={!s.mdm_enrolled} title={s.mdm_enrolled ? 'Send the DeclarativeManagement command' : 'Not MDM enrolled'}>
                Sync now
              </Button>
            )
          }
          className="border-b-0"
        />
        <CardBody className="pt-1">
          <div className="mb-4">
            {inSync ? (
              <Badge tone="good" icon={<CheckCircle2 />} size="md">
                In sync
              </Badge>
            ) : (
              <Badge tone="neutral" icon={<CircleDashed />} size="md">
                {st?.served_token ? 'Waiting for the device to fetch the new set' : 'The device has not fetched declarations yet'}
              </Badge>
            )}
          </div>
          <DescriptionList
            cols={2}
            items={[
              { label: 'Current DeclarationsToken', value: d.declarations_token, mono: true },
              { label: 'Token the device last fetched', value: st?.served_token, mono: true },
              { label: 'Last sync (declarations fetched)', value: <RelativeTime value={st?.last_sync_at} /> },
              { label: 'Last status report', value: <RelativeTime value={st?.last_status_at} /> },
              { label: 'Last sync request sent', value: st?.sync_sent_at ? <><RelativeTime value={st.sync_sent_at} /> {st.sync_sent_token && <span className="ml-1 font-mono text-xs text-ink-muted">for {st.sync_sent_token}</span>}</> : '' },
            ]}
          />
          {st?.errors && st.errors.length > 0 && (
            <div className="mt-4">
              <p className="micro-label mb-1.5">Errors reported by the device</p>
              <CodeBlock text={JSON.stringify(st.errors, null, 2)} filename="Errors" maxHeight="12rem" />
            </div>
          )}
        </CardBody>
      </section>

      <section>
        <CardHeader title={`Declarations (${declarations.length})`} description="Exactly as served to this Mac, with the validity the device reported for each." className="border-b-0" />
        <CardBody className="pt-1">
          {declarations.length === 0 ? (
            <EmptyState compact title="No declarations" description="This Mac has no declarations yet." />
          ) : (
            <ul className="flex flex-col gap-2">
              {declarations.map((decl) => (
                <DeclarationItem key={decl.Identifier} decl={decl} status={statusIdx.get(decl.Identifier)} />
              ))}
            </ul>
          )}
          <SupervisionCallout className="mt-4" />
        </CardBody>
      </section>

      <section>
        <CardHeader title="Status reports" description="The merged status this Mac has reported, and its latest raw reports (newest first)." className="border-b-0" />
        <CardBody className="pt-1 flex flex-col gap-5">
          <div>
            <p className="micro-label mb-1.5">Current status</p>
            {st?.status && Object.keys(st.status).length > 0 ? (
              <div className="rounded-lg border border-line bg-surface-sunken/50 p-3">
                <KeyTree value={st.status} />
              </div>
            ) : (
              <p className="text-sm text-ink-faint">No status reported yet.</p>
            )}
          </div>
          <div>
            <p className="micro-label mb-1.5">Recent reports</p>
            {(d.recent_reports ?? []).length === 0 ? (
              <p className="text-sm text-ink-faint">No reports received yet.</p>
            ) : (
              <ul className="flex flex-col gap-1.5">
                {(d.recent_reports ?? []).map((r, i) => (
                  <Expandable
                    key={`${r.at}-${i}`}
                    summary={
                      <>
                        <RelativeTime value={r.at} className="text-ink" />
                        <span className="text-xs text-ink-muted">{absoluteTime(r.at)}</span>
                        <span className="ml-auto text-xs text-ink-faint">{reportSummary(r.report)}</span>
                      </>
                    }
                  >
                    <CodeBlock text={JSON.stringify(r.report, null, 2)} filename="status report" maxHeight="24rem" />
                  </Expandable>
                ))}
              </ul>
            )}
          </div>
        </CardBody>
      </section>

      <section>
        <CardBody>
          <HowDDMWorks />
        </CardBody>
      </section>
    </div>
  )
}

function reportSummary(r: Obj): string {
  const parts: string[] = []
  if (isObj(r.StatusItems)) parts.push(`${Object.keys(r.StatusItems).join(', ')}`)
  if (isObj(r.Declarations)) parts.push('declarations')
  if (Array.isArray(r.Errors) && r.Errors.length) parts.push(`${r.Errors.length} error${r.Errors.length === 1 ? '' : 's'}`)
  return parts.join(' · ')
}

function Expandable({ summary, children, className }: { summary: ReactNode; children: ReactNode; className?: string }) {
  const [open, setOpen] = useState(false)
  return (
    <li className={cn('rounded-lg border border-line', className)}>
      <button type="button" onClick={() => setOpen((o) => !o)} aria-expanded={open} className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm focus-ring rounded-lg">
        <ChevronRight className={cn('size-4 shrink-0 text-ink-faint transition-transform', open && 'rotate-90')} aria-hidden="true" />
        {summary}
      </button>
      {open && <div className="border-t border-line p-3">{children}</div>}
    </li>
  )
}

function DeclarationItem({ decl, status }: { decl: DDMDeclaration; status?: DeclStatus & { kind: string } }) {
  const valid = status?.valid
  const validTone = valid === 'valid' || valid === true ? 'good' : valid === 'invalid' || valid === false ? 'critical' : 'neutral'
  const validLabel = valid === true ? 'valid' : valid === false ? 'invalid' : valid || 'unknown'
  const tokenMatch = status?.['server-token'] !== undefined ? status['server-token'] === decl.ServerToken : null
  return (
    <Expandable
      summary={
        <span className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1">
          <span className="min-w-0">
            <span className="block truncate font-mono text-xs text-ink">{decl.Type}</span>
            <span className="block truncate font-mono text-[11px] text-ink-muted">{decl.Identifier}</span>
          </span>
          <span className="ml-auto flex flex-wrap items-center gap-1.5">
            {status ? (
              <>
                <Badge tone={validTone} dot>
                  {validLabel}
                </Badge>
                <Badge tone={status.active ? 'good' : 'neutral'} dot>
                  {status.active ? 'active' : 'inactive'}
                </Badge>
                {tokenMatch === false && <Badge tone="warning" title="The device reported an older server token for this declaration">older version</Badge>}
              </>
            ) : (
              <Badge>Not reported</Badge>
            )}
          </span>
        </span>
      }
    >
      <div className="flex flex-col gap-3">
        <DescriptionList
          cols={2}
          items={[
            { label: 'ServerToken', value: decl.ServerToken, mono: true },
            { label: 'Device-reported token', value: status?.['server-token'], mono: true },
          ]}
        />
        {status?.reasons && status.reasons.length > 0 && (
          <div>
            <p className="micro-label mb-1.5">Reasons</p>
            <ul className="flex flex-col gap-1">
              {status.reasons.map((r, i) => (
                <li key={i} className="rounded-md border border-critical/25 bg-critical/6 px-2.5 py-1.5 text-xs">
                  {r.code && <span className="font-mono text-ink">{r.code}</span>}
                  {r.description && <span className="ml-2 text-ink-muted">{r.description}</span>}
                  {r.details !== undefined && <pre className="mt-1 whitespace-pre-wrap font-mono text-[11px] text-ink-muted">{JSON.stringify(r.details, null, 2)}</pre>}
                </li>
              ))}
            </ul>
          </div>
        )}
        <CodeBlock text={JSON.stringify(decl.Payload ?? {}, null, 2)} filename="Payload" maxHeight="20rem" />
      </div>
    </Expandable>
  )
}

/** Readable nested key/value view of the merged status (e.g. softwareupdate → install-state). */
function KeyTree({ value, depth = 0 }: { value: unknown; depth?: number }) {
  if (isObj(value)) {
    const entries = Object.entries(value).sort(([a], [b]) => a.localeCompare(b))
    if (entries.length === 0) return <span className="font-mono text-xs text-ink-faint">{'{}'}</span>
    return (
      <dl className={cn('flex flex-col gap-1', depth > 0 && 'mt-1 border-l border-line pl-3')}>
        {entries.map(([k, v]) => (
          <div key={k} className={cn(isObj(v) || (Array.isArray(v) && v.some(isObj)) ? 'block' : 'flex flex-wrap items-baseline gap-x-3')}>
            <dt className="font-mono text-xs text-ink-muted">{k}</dt>
            <dd className="min-w-0">
              <KeyTree value={v} depth={depth + 1} />
            </dd>
          </div>
        ))}
      </dl>
    )
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return <span className="font-mono text-xs text-ink-faint">[]</span>
    if (!value.some(isObj)) return <span className="font-mono text-xs text-ink break-all">{value.map((x) => String(x)).join(', ')}</span>
    return (
      <ol className="mt-1 flex flex-col gap-1.5 border-l border-line pl-3">
        {value.map((x, i) => (
          <li key={i}>
            <span className="font-mono text-[11px] text-ink-faint">[{i}]</span>
            <KeyTree value={x} depth={depth + 1} />
          </li>
        ))}
      </ol>
    )
  }
  if (value === null || value === undefined) return <span className="font-mono text-xs text-ink-faint">null</span>
  if (typeof value === 'boolean') return <span className={cn('font-mono text-xs', value ? 'text-ink' : 'text-ink-muted')}>{String(value)}</span>
  return <span className="font-mono text-xs text-ink break-all">{String(value)}</span>
}

function HowDDMWorks() {
  return (
    <details className="group rounded-lg border border-line">
      <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2.5 text-sm font-medium text-ink focus-ring rounded-lg">
        <ChevronRight className="size-4 text-ink-faint transition-transform group-open:rotate-90" aria-hidden="true" />
        <BookOpen className="size-4 text-accent-400" aria-hidden="true" />
        How Declarative Device Management works
      </summary>
      <div className="border-t border-line px-4 py-4 text-[13px] leading-relaxed text-ink-muted">
        <ol className="list-decimal space-y-3 pl-5">
          <li>
            The server sends the MDM command <span className="font-mono text-ink">DeclarativeManagement</span> with a sync token. That enables DDM on the device, or asks it to re-sync.
            The <strong className="text-ink">Sync now</strong> button above sends it.
          </li>
          <li>
            The device sends DeclarativeManagement <strong className="text-ink">check-ins</strong> to <span className="font-mono">/mdm</span>:
            <ul className="mt-1.5 list-disc space-y-1 pl-5">
              <li>
                <span className="font-mono text-ink">tokens</span> returns the <span className="font-mono">DeclarationsToken</span>, which changes whenever the declaration set changes. If it matches what the
                device already has, it stops here.
              </li>
              <li>
                <span className="font-mono text-ink">declaration-items</span> returns the manifest: identifiers and server tokens for activations, configurations, assets and management declarations.
              </li>
              <li>
                <span className="font-mono text-ink">declaration/&lt;kind&gt;/&lt;identifier&gt;</span> returns each declaration the device doesn&apos;t already have (its server token changed).
              </li>
              <li>
                <span className="font-mono text-ink">status</span> is a JSON report from the device: subscribed status items, plus whether each declaration is <strong className="text-ink">valid</strong> and{' '}
                <strong className="text-ink">active</strong>.
              </li>
            </ul>
          </li>
          <li>
            Activations (<span className="font-mono">com.apple.activation.simple</span>) turn configurations on. VibeMDM pairs every configuration with one.
          </li>
        </ol>
        <p className="mt-4">
          VibeMDM subscribes every Mac to OS version, build, model and software update status (<span className="font-mono">softwareupdate.install-state</span>,{' '}
          <span className="font-mono">pending-version</span>, <span className="font-mono">failure-reason</span>, <span className="font-mono">install-reason</span>). It re-checks every 30 minutes and after any plan
          change, then sends a sync only to Macs whose declaration set changed.
        </p>
        <div className="mt-4 rounded-md border border-line bg-surface-sunken/50 p-3">
          <p className="micro-label mb-1.5">Study notes</p>
          <ul className="list-disc space-y-1 pl-5">
            <li>DDM is device-driven: the server publishes state, and the device decides when to fetch it and reports back asynchronously. MDM commands are server-queued and pulled on push.</li>
            <li>The status channel replaces polling commands like DeviceInformation for many values.</li>
            <li>Server tokens let the device skip declarations it already has. The DeclarationsToken lets it skip the whole sync.</li>
          </ul>
        </div>
        <p className="mt-3 text-xs">
          Manage plans under{' '}
          <Link to="/updates/plans" className="text-ink hover:underline">
            Software Updates
          </Link>
          .
        </p>
      </div>
    </details>
  )
}
