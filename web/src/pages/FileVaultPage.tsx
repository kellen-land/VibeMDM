import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { KeyRound, RefreshCw, ShieldCheck, Wrench } from 'lucide-react'
import { qk, useFileVault } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { normalizeScope, emptyScope, type BuiltinProfileView, type FileVaultEnforceOptions, type FileVaultOverview, type FileVaultRow, type FileVaultUpdate, type Scope } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Tabs } from '../components/ui/Tabs'
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
import { shortDate } from '../lib/format'
import { useEditMode } from '../components/editor/useEditMode'
import { ObjectBar } from '../components/editor/ObjectBar'

const defaultEnforceOptions = (): FileVaultEnforceOptions => ({ DeferAtLogout: true, DeferDontAskAtLogout: false, MaxBypassAttempts: 0, ShowRecoveryKey: false })

/** Remediation applies to Macs with FileVault on and no escrowed key. */
export function needsRemediation(fv: { filevault_enabled?: boolean | null; escrowed: boolean }): boolean {
  return fv.filevault_enabled === true && !fv.escrowed
}

/** One sentence instead of a row of tiles: says whether anything needs attention. */
function fileVaultSummary(d: FileVaultOverview): string {
  const s = d.summary
  if (s.total === 0) return 'No computers enrolled yet.'
  const parts = [`${s.escrowed} of ${s.total} ${s.total === 1 ? 'Mac has' : 'Macs have'} an escrowed recovery key.`]
  if (s.missing_key > 0) parts.push(`${s.missing_key} ${s.missing_key === 1 ? 'needs' : 'need'} a new key.`)
  if (!d.escrow_profile.deployed) parts.push('Escrow is off: turn it on under Settings.')
  return parts.join(' ')
}

export function FileVaultPage() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const qc = useQueryClient()
  const q = useFileVault()
  const d = q.data
  const [checking, setChecking] = useState(false)
  const [filter, setFilter] = useState('')
  const [view, setView] = useState<'computers' | 'settings'>('computers')
  const [saving, setSaving] = useState(false)

  // Settings tab: escrow profile, enforcement profile and auto-remediation edit and save together.
  const [form, setForm] = useState<FileVaultForm>(() => fileVaultForm(d))
  const load = useCallback(() => {
    if (d) setForm(fileVaultForm(d))
  }, [d])
  const ed = useEditMode({ isNew: false, canEdit: isAdmin, snapshot: form, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])

  /** PUT /filevault with only the parts that changed; it returns the refreshed overview, written straight into the cache. */
  const save = async () => {
    if (!d) return false
    const saved = fileVaultForm(d)
    const body: FileVaultUpdate = {}
    if (JSON.stringify(form.escrow) !== JSON.stringify(saved.escrow)) {
      if (form.escrow.enabled && !d.escrow_certificate) {
        toast.error('Escrow profile cannot be deployed', 'The escrow identity is not loaded on the server.')
        return false
      }
      body.escrow = form.escrow
    }
    if (JSON.stringify(form.enforce) !== JSON.stringify(saved.enforce)) body.enforce = form.enforce
    if (form.autoRemediate !== saved.autoRemediate) body.auto_remediate = form.autoRemediate
    if (!body.escrow && !body.enforce && body.auto_remediate === undefined) return true
    setSaving(true)
    try {
      const r = await api.put<FileVaultOverview>('/api/v1/filevault', body)
      qc.setQueryData(qk.filevault, r)
      toast.success('FileVault settings saved')
      return true
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
      return false
    } finally {
      setSaving(false)
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
        description={fileVaultSummary(d)}
        actions={
          isAdmin && (
            <Button icon={<RefreshCw />} loading={checking} onClick={checkNow}>
              Check now
            </Button>
          )
        }
      />
      <Tabs
        items={[
          { key: 'computers', label: 'Computers' },
          { key: 'settings', label: 'Settings' },
        ]}
        value={view}
        onChange={(k) => setView(k as 'computers' | 'settings')}
        className="mb-5"
      />
      <div className="flex flex-col gap-6">
        {view === 'settings' && (
          <>
        <EscrowProfileCard view={d.escrow_profile} cert={d.escrow_certificate} readOnly={ed.readOnly} value={form.escrow} onChange={(escrow) => setForm((f) => ({ ...f, escrow }))} />
        <EnforceProfileCard view={d.enforce_profile} readOnly={ed.readOnly} value={form.enforce} onChange={(enforce) => setForm((f) => ({ ...f, enforce }))} />

        <Card>
          <CardHeader title="Auto-remediation" actions={d.auto_remediate ? <Badge tone="good" dot>On</Badge> : <Badge>Off</Badge>} />
          <CardBody>
            <Toggle
              checked={form.autoRemediate}
              disabled={ed.readOnly}
              onChange={(v) => setForm((f) => ({ ...f, autoRemediate: v }))}
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

          </>
        )}

        {view === 'computers' && (
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
        )}
      </div>
      {view === 'settings' && isAdmin ? <ObjectBar ed={ed} isNew={false} listPath="/filevault" noun="FileVault settings" saving={saving} onSave={save} noDone /> : ed.guard}
    </>
  )
}

interface FileVaultForm {
  escrow: { enabled: boolean; scope: Scope }
  enforce: { enabled: boolean; scope: Scope; options: FileVaultEnforceOptions }
  autoRemediate: boolean
}

/** The editable FileVault settings as the server has them. */
function fileVaultForm(d: FileVaultOverview | undefined): FileVaultForm {
  const scopeOf = (v?: BuiltinProfileView) => (v?.profile ? normalizeScope(v.profile.scope) : emptyScope())
  return {
    escrow: { enabled: !!d?.escrow_profile.deployed, scope: scopeOf(d?.escrow_profile) },
    enforce: { enabled: !!d?.enforce_profile.deployed, scope: scopeOf(d?.enforce_profile), options: d?.enforce_profile.options ?? defaultEnforceOptions() },
    autoRemediate: !!d?.auto_remediate,
  }
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
  value,
  onChange,
}: {
  view: BuiltinProfileView
  cert?: { subject: string; not_after: string }
  readOnly: boolean
  value: FileVaultForm['escrow']
  onChange: (v: FileVaultForm['escrow']) => void
}) {
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
        <Toggle checked={value.enabled} onChange={(enabled) => onChange({ ...value, enabled })} disabled={readOnly} label="Deploy the escrow profile" description="Turning this off keeps the profile record but removes it from every Mac." />
        {value.enabled && (readOnly ? <ScopeSummary scope={value.scope} /> : <ScopeEditor value={value.scope} onChange={(scope) => onChange({ ...value, scope })} />)}
        <div>
          <p className="micro-label mb-1.5">Deployment status</p>
          <DeploymentCounts status={view.status} />
        </div>
      </CardBody>
      {view.profile && (
        <CardFooter className="justify-start">
          <Link to={`/profiles/${view.profile.id}`} className="text-xs text-ink-muted hover:text-ink hover:underline">
            View profile
          </Link>
        </CardFooter>
      )}
    </Card>
  )
}

function EnforceProfileCard({ view, readOnly, value, onChange }: { view: BuiltinProfileView; readOnly: boolean; value: FileVaultForm['enforce']; onChange: (v: FileVaultForm['enforce']) => void }) {
  const opts = value.options
  const setOpt = (patch: Partial<FileVaultEnforceOptions>) => onChange({ ...value, options: { ...opts, ...patch } })
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
        <Toggle checked={value.enabled} onChange={(enabled) => onChange({ ...value, enabled })} disabled={readOnly} label="Deploy the enforcement profile" />
        {value.enabled && (
          <>
            {readOnly ? <ScopeSummary scope={value.scope} /> : <ScopeEditor value={value.scope} onChange={(scope) => onChange({ ...value, scope })} />}
            <div className="flex flex-col gap-4 rounded-lg border border-line p-4">
              <p className="micro-label">Options</p>
              {!readOnly && <p className="-mt-2 text-xs text-ink-muted">{view.options ? 'Loaded from the deployed enforcement profile. Saving replaces it with the values shown here.' : 'No enforcement profile exists yet; these are the defaults it will be created with.'}</p>}
              <FormGrid cols={2}>
                <Toggle checked={opts.DeferAtLogout} onChange={(v) => setOpt({ DeferAtLogout: v })} disabled={readOnly} label="Prompt at logout" description="Ask the user to enable FileVault when they log out. When off, FileVault is enabled at the next login without a logout prompt." />
                <Toggle checked={opts.DeferDontAskAtLogout} onChange={(v) => setOpt({ DeferDontAskAtLogout: v })} disabled={readOnly || !opts.DeferAtLogout} label="Don't ask at logout" description="Skip the logout prompt and require FileVault at the next login instead." />
                <Toggle checked={opts.ShowRecoveryKey} onChange={(v) => setOpt({ ShowRecoveryKey: v })} disabled={readOnly} label="Show recovery key to the user" description="Display the new personal recovery key when FileVault is enabled." />
                <Field label="Maximum bypass attempts" hint="Logins the user may skip before FileVault is required. -1 allows unlimited deferrals; 0 requires it at the next login.">
                  <Input type="number" min={-1} max={9999} value={opts.MaxBypassAttempts} onChange={(e) => setOpt({ MaxBypassAttempts: Math.max(-1, Math.min(9999, Math.trunc(Number(e.target.value) || 0))) })} disabled={readOnly} className="max-w-32" />
                </Field>
              </FormGrid>
            </div>
          </>
        )}
        <div>
          <p className="micro-label mb-1.5">Deployment status</p>
          <DeploymentCounts status={view.status} />
        </div>
      </CardBody>
      {view.profile && (
        <CardFooter className="justify-start">
          <Link to={`/profiles/${view.profile.id}`} className="text-xs text-ink-muted hover:text-ink hover:underline">
            View profile
          </Link>
        </CardFooter>
      )}
    </Card>
  )
}
