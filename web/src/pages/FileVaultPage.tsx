import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { HelpCircle, KeyRound, LockKeyholeOpen, RefreshCw, Save, ShieldCheck, ShieldAlert, Wrench } from 'lucide-react'
import { qk, useFileVault } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { normalizeScope, emptyScope, type BuiltinProfileView, type FileVaultEnforceOptions, type FileVaultOverview, type FileVaultRow, type FileVaultUpdate, type Scope } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardFooter, CardHeader } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Badge, BoolBadge, StatusBadge, statusTone } from '../components/ui/Badge'
import { Toggle } from '../components/ui/Checkbox'
import { Field, FormGrid } from '../components/ui/Field'
import { Input } from '../components/ui/Input'
import { Alert } from '../components/ui/Alert'
import { Table } from '../components/ui/Table'
import { RelativeTime } from '../components/ui/RelativeTime'
import { LoadingBlock } from '../components/ui/Spinner'
import { SearchInput } from '../components/ui/SearchInput'
import { useToast } from '../components/ui/Toast'
import { ScopeEditor, ScopeSummary } from '../components/pickers/ScopeEditor'
import { StatTile } from './DashboardPage'
import { shortDate } from '../lib/format'

const defaultEnforceOptions = (): FileVaultEnforceOptions => ({ DeferAtLogout: true, DeferDontAskAtLogout: false, MaxBypassAttempts: 0, ShowRecoveryKey: false })

/** Remediation applies to Macs with FileVault on and no escrowed key. */
export function needsRemediation(fv: { filevault_enabled?: boolean | null; escrowed: boolean }): boolean {
  return fv.filevault_enabled === true && !fv.escrowed
}

export function FileVaultPage() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const qc = useQueryClient()
  const q = useFileVault()
  const d = q.data
  const [checking, setChecking] = useState(false)
  const [filter, setFilter] = useState('')

  /** PUT /filevault returns the refreshed overview; write it straight into the cache. */
  const update = async (body: FileVaultUpdate, success: string) => {
    try {
      const r = await api.put<FileVaultOverview>('/api/v1/filevault', body)
      qc.setQueryData(qk.filevault, r)
      toast.success(success)
      return r
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
      return null
    }
  }

  const checkNow = async () => {
    setChecking(true)
    try {
      const r = await api.post<{ queued: number }>('/api/v1/filevault/check')
      toast.success(`SecurityInfo queued for ${r.queued} ${r.queued === 1 ? 'Mac' : 'Macs'}`, r.queued ? 'Keys are collected as each Mac checks in.' : 'No Macs are missing a key.')
    } catch (e) {
      toast.error('Check failed', errorMessage(e))
    } finally {
      setChecking(false)
    }
  }

  const remediate = async (r: FileVaultRow) => {
    try {
      await api.post(`/api/v1/computers/${r.computer_id}/filevault/remediate`)
      toast.success('Escrow Buddy remediation queued', `${r.computer_name || r.serial_number} will generate a new key at the next login.`)
      await qc.invalidateQueries({ queryKey: qk.filevault })
    } catch (e) {
      toast.error('Remediation failed', errorMessage(e))
    }
  }

  if (q.isPending) return <LoadingBlock />
  if (q.error || !d) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'FileVault overview unavailable'}</Alert>

  const s = filter.trim().toLowerCase()
  const rows = (d.computers ?? []).filter((r) => !s || r.computer_name.toLowerCase().includes(s) || r.serial_number.toLowerCase().includes(s))

  return (
    <>
      <PageHeader
        title="FileVault"
        description="Escrow each Mac's FileVault personal recovery key, enforce FileVault, and repair Macs that are missing a key."
        actions={
          isAdmin && (
            <Button icon={<RefreshCw />} loading={checking} onClick={checkNow}>
              Check now
            </Button>
          )
        }
      />
      <div className="flex flex-col gap-6">
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <StatTile label="Escrowed" value={d.summary.escrowed} icon={ShieldCheck} />
          <StatTile label="Missing key" value={d.summary.missing_key} icon={ShieldAlert} tone="warn" />
          <StatTile label="Not encrypted" value={d.summary.not_encrypted} icon={LockKeyholeOpen} />
          <StatTile label="Unknown" value={d.summary.unknown} icon={HelpCircle} />
        </div>

        <EscrowProfileCard view={d.escrow_profile} cert={d.escrow_certificate} readOnly={!isAdmin} onSave={(enabled, scope) => update({ escrow: { enabled, scope } }, enabled ? 'Escrow profile saved' : 'Escrow profile turned off')} />
        <EnforceProfileCard view={d.enforce_profile} readOnly={!isAdmin} onSave={(enabled, scope, options) => update({ enforce: { enabled, scope, options } }, enabled ? 'Enforcement profile saved' : 'Enforcement profile turned off')} />

        <Card>
          <CardHeader title="Auto-remediation" actions={d.auto_remediate ? <Badge tone="good" dot>On</Badge> : <Badge>Off</Badge>} />
          <CardBody>
            <Toggle
              checked={d.auto_remediate}
              disabled={!isAdmin}
              onChange={(v) => update({ auto_remediate: v }, v ? 'Auto-remediation turned on' : 'Auto-remediation turned off')}
              label="Automatically remediate missing keys"
              description="Uses Escrow Buddy to generate a new key at the next login for Macs with FileVault on, the escrow profile installed, and no escrowed key (at most once per day)."
            />
          </CardBody>
          <CardFooter className="justify-start">
            <span className="text-xs text-ink-muted">
              Remediation uses{' '}
              <a href="https://github.com/macadmins/escrow-buddy" target="_blank" rel="noreferrer" className="text-ink hover:underline">
                Escrow Buddy
              </a>{' '}
              (Apache-2.0)
            </span>
          </CardFooter>
        </Card>

        <Card>
          <CardHeader
            title="Computers"
            description={`${d.summary.total.toLocaleString()} computers. Keys are never shown here; open a computer's Security tab to reveal one.`}
            actions={<SearchInput value={filter} onChange={setFilter} placeholder="Filter by name or serial…" className="w-64" />}
          />
          <Table<FileVaultRow>
            rows={rows}
            rowKey={(r) => r.computer_id}
            emptyTitle={s ? 'No computers match' : 'No computers enrolled'}
            columns={[
              {
                key: 'name',
                header: 'Computer',
                render: (r) => (
                  <Link to={`/computers/${r.computer_id}/security`} className="font-medium text-ink hover:underline">
                    {r.computer_name || r.serial_number || r.computer_id}
                  </Link>
                ),
              },
              { key: 'serial', header: 'Serial', render: (r) => <span className="font-mono text-xs text-ink-muted">{r.serial_number || '—'}</span> },
              { key: 'fv', header: 'FileVault', render: (r) => <BoolBadge value={r.filevault_enabled} yes="On" no="Off" /> },
              { key: 'profile', header: 'Escrow profile', render: (r) => (r.escrow_profile_status ? <StatusBadge status={r.escrow_profile_status} /> : <span className="text-ink-faint">Not scoped</span>) },
              {
                key: 'escrowed',
                header: 'Escrowed',
                render: (r) =>
                  r.escrowed ? (
                    <span className="inline-flex items-center gap-2">
                      <Badge tone="good" dot>
                        Escrowed
                      </Badge>
                      <span className="font-mono text-xs text-ink-muted" title={r.fingerprint}>
                        {r.fingerprint}
                      </span>
                    </span>
                  ) : needsRemediation(r) ? (
                    <Badge tone="warning" dot>
                      Missing key
                    </Badge>
                  ) : (
                    <Badge>No key</Badge>
                  ),
              },
              { key: 'at', header: 'Escrowed at', render: (r) => <RelativeTime value={r.escrowed_at} emptyLabel="—" className="text-ink-muted" /> },
              { key: 'rotated', header: 'Rotated', align: 'right', render: (r) => <span className="tabular-nums text-ink-muted">{r.rotated_count}</span> },
              {
                key: 'actions',
                header: '',
                align: 'right',
                render: (r) =>
                  isAdmin && needsRemediation(r) ? (
                    <Button size="xs" variant="ghost" icon={<Wrench />} onClick={() => remediate(r)} title="Run Escrow Buddy so a new key is generated at the next login">
                      Remediate
                    </Button>
                  ) : null,
              },
            ]}
          />
        </Card>
      </div>
    </>
  )
}

function DeploymentCounts({ status }: { status: Record<string, number> }) {
  const entries = Object.entries(status ?? {}).filter(([, n]) => n > 0)
  if (entries.length === 0) return <span className="text-xs text-ink-faint">No deployments yet</span>
  return (
    <div className="flex flex-wrap gap-1.5">
      {entries.map(([k, n]) => (
        <Badge key={k} tone={statusTone(k)} dot>
          {k.replace(/^\w/, (c) => c.toUpperCase())} · {n}
        </Badge>
      ))}
    </div>
  )
}

/** Local edit state for a built-in profile, re-seeded only when the server copy changes (not on every poll). */
function useBuiltinForm(view: BuiltinProfileView) {
  const seed = `${view.deployed}|${view.profile?.updated_at ?? ''}`
  const [enabled, setEnabled] = useState(view.deployed)
  const [scope, setScope] = useState<Scope>(() => (view.profile ? normalizeScope(view.profile.scope) : emptyScope()))
  useEffect(() => {
    setEnabled(view.deployed)
    setScope(view.profile ? normalizeScope(view.profile.scope) : emptyScope())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seed])
  return { enabled, setEnabled, scope, setScope }
}

function ProfileStateBadge({ view }: { view: BuiltinProfileView }) {
  return view.deployed ? (
    <Badge tone="good" dot>
      Deployed
    </Badge>
  ) : (
    <Badge>Off</Badge>
  )
}

function EscrowProfileCard({
  view,
  cert,
  readOnly,
  onSave,
}: {
  view: BuiltinProfileView
  cert?: { subject: string; not_after: string }
  readOnly: boolean
  onSave: (enabled: boolean, scope: Scope) => Promise<unknown>
}) {
  const f = useBuiltinForm(view)
  const [saving, setSaving] = useState(false)
  const save = async () => {
    setSaving(true)
    await onSave(f.enabled, f.scope)
    setSaving(false)
  }
  return (
    <Card>
      <CardHeader
        title={
          <span className="inline-flex items-center gap-2">
            <KeyRound className="size-4 text-ink-faint" /> Escrow profile
          </span>
        }
        description="Built-in FileVault Recovery Key Escrow profile (com.apple.security.FDERecoveryKeyEscrow)."
        actions={<ProfileStateBadge view={view} />}
      />
      <CardBody className="flex flex-col gap-5">
        <p className="text-sm text-ink-muted">
          With this profile installed, macOS encrypts each Mac&apos;s personal recovery key to VibeMDM&apos;s escrow certificate and returns it in the SecurityInfo response; the server decrypts and stores it sealed. macOS only escrows keys generated <strong className="text-ink">after</strong> the profile is installed, so Macs encrypted earlier show as missing a key until a new one is generated.
        </p>
        {cert ? (
          <p className="text-xs text-ink-muted">
            Escrow certificate <span className="font-mono text-ink">{cert.subject}</span> · expires {shortDate(cert.not_after)}
          </p>
        ) : (
          <Alert tone="warning">The escrow identity is not loaded on the server. The escrow profile cannot be deployed until it is.</Alert>
        )}
        <Toggle checked={f.enabled} onChange={f.setEnabled} disabled={readOnly} label="Deploy the escrow profile" description="Turning this off keeps the profile record but removes it from every Mac." />
        {f.enabled && (readOnly ? <ScopeSummary scope={f.scope} /> : <ScopeEditor value={f.scope} onChange={f.setScope} />)}
        <div>
          <p className="micro-label mb-1.5">Deployment status</p>
          <DeploymentCounts status={view.status} />
        </div>
      </CardBody>
      {!readOnly && (
        <CardFooter>
          {view.profile && (
            <Link to={`/profiles/${view.profile.id}`} className="mr-auto text-xs text-ink-muted hover:text-ink hover:underline">
              View profile
            </Link>
          )}
          <Button variant="primary" icon={<Save />} loading={saving} onClick={save} disabled={!cert && f.enabled}>
            Save
          </Button>
        </CardFooter>
      )}
    </Card>
  )
}

function EnforceProfileCard({ view, readOnly, onSave }: { view: BuiltinProfileView; readOnly: boolean; onSave: (enabled: boolean, scope: Scope, options: FileVaultEnforceOptions) => Promise<unknown> }) {
  const f = useBuiltinForm(view)
  const [opts, setOpts] = useState<FileVaultEnforceOptions>(defaultEnforceOptions)
  const [saving, setSaving] = useState(false)
  const setOpt = (patch: Partial<FileVaultEnforceOptions>) => setOpts((o) => ({ ...o, ...patch }))
  const save = async () => {
    setSaving(true)
    await onSave(f.enabled, f.scope, opts)
    setSaving(false)
  }
  return (
    <Card>
      <CardHeader
        title={
          <span className="inline-flex items-center gap-2">
            <ShieldCheck className="size-4 text-ink-faint" /> Enforce FileVault
          </span>
        }
        description="Optional built-in FileVault Enforcement profile (com.apple.MCX.FileVault2)."
        actions={<ProfileStateBadge view={view} />}
      />
      <CardBody className="flex flex-col gap-5">
        <p className="text-sm text-ink-muted">Turns FileVault on with a personal recovery key, deferred to the next login, so new keys are escrowed from the start. Scope it together with the escrow profile.</p>
        <Toggle checked={f.enabled} onChange={f.setEnabled} disabled={readOnly} label="Deploy the enforcement profile" />
        {f.enabled && (
          <>
            {readOnly ? <ScopeSummary scope={f.scope} /> : <ScopeEditor value={f.scope} onChange={f.setScope} />}
            {!readOnly && (
              <div className="flex flex-col gap-4 rounded-lg border border-line p-4">
                <p className="micro-label">Options</p>
                <p className="-mt-2 text-xs text-ink-muted">Options are not read back from the deployed profile; saving sends the values shown here.</p>
                <FormGrid cols={2}>
                  <Toggle checked={opts.DeferAtLogout} onChange={(v) => setOpt({ DeferAtLogout: v })} label="Prompt at logout" description="Ask the user to enable FileVault when they log out. When off, FileVault is enabled at the next login without a logout prompt." />
                  <Toggle checked={opts.DeferDontAskAtLogout} onChange={(v) => setOpt({ DeferDontAskAtLogout: v })} disabled={!opts.DeferAtLogout} label="Don't ask at logout" description="Skip the logout prompt and require FileVault at the next login instead." />
                  <Toggle checked={opts.ShowRecoveryKey} onChange={(v) => setOpt({ ShowRecoveryKey: v })} label="Show recovery key to the user" description="Display the new personal recovery key when FileVault is enabled." />
                  <Field label="Maximum bypass attempts" hint="Logins the user may skip before FileVault is required. -1 allows unlimited deferrals; 0 requires it at the next login.">
                    <Input type="number" min={-1} max={9999} value={opts.MaxBypassAttempts} onChange={(e) => setOpt({ MaxBypassAttempts: Math.max(-1, Math.min(9999, Math.trunc(Number(e.target.value) || 0))) })} className="max-w-32" />
                  </Field>
                </FormGrid>
              </div>
            )}
          </>
        )}
        <div>
          <p className="micro-label mb-1.5">Deployment status</p>
          <DeploymentCounts status={view.status} />
        </div>
      </CardBody>
      {!readOnly && (
        <CardFooter>
          {view.profile && (
            <Link to={`/profiles/${view.profile.id}`} className="mr-auto text-xs text-ink-muted hover:text-ink hover:underline">
              View profile
            </Link>
          )}
          <Button variant="primary" icon={<Save />} loading={saving} onClick={save}>
            Save
          </Button>
        </CardFooter>
      )}
    </Card>
  )
}
