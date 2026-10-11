import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Save } from 'lucide-react'
import { qk, useLAPS } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { emptyScope, normalizeScope, type LAPSConfig, type LAPSOverview, type LAPSStatus } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardFooter, CardHeader } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Badge, type BadgeTone } from '../components/ui/Badge'
import { Toggle } from '../components/ui/Checkbox'
import { Field } from '../components/ui/Field'
import { Input, Select } from '../components/ui/Input'
import { Alert } from '../components/ui/Alert'
import { Table } from '../components/ui/Table'
import { RelativeTime } from '../components/ui/RelativeTime'
import { LoadingBlock } from '../components/ui/Spinner'
import { useToast } from '../components/ui/Toast'
import { ScopeEditor, ScopeSummary } from '../components/pickers/ScopeEditor'
import { LapsRevealButton } from '../components/LapsReveal'

const statusLabel: Record<string, { label: string; tone: BadgeTone }> = {
  ok: { label: 'Current', tone: 'good' },
  due: { label: 'Rotation due', tone: 'info' },
  pending: { label: 'Updating', tone: 'info' },

  missing: { label: 'Waiting for first password', tone: 'warning' },
  failing: { label: 'Needs attention', tone: 'critical' },
}

export function LapsStatusBadge({ status }: { status?: string }) {
  const s = statusLabel[status ?? ''] ?? { label: 'Not managed', tone: 'neutral' as BadgeTone }
  return (
    <Badge tone={s.tone} dot>
      {s.label}
    </Badge>
  )
}

/** A single sentence that says whether anything needs attention. */
function summaryLine(d: LAPSOverview): string {
  const s = d.summary
  if (!d.config.enabled) return 'Off. Turn it on below to give every Mac its own random administrator password.'
  if (s.in_scope === 0) return 'On, but no Macs with the agent are in scope yet.'
  const bad = s.failing + s.missing
  if (bad > 0) return `${s.ok} of ${s.in_scope} Macs are current. ${bad} ${bad === 1 ? 'needs' : 'need'} attention.`
  return `All ${s.in_scope} ${s.in_scope === 1 ? 'Mac is' : 'Macs are'} current.`
}

export function LapsPage() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const qc = useQueryClient()
  const q = useLAPS()
  const d = q.data

  const save = async (cfg: LAPSConfig) => {
    try {
      const r = await api.put<LAPSOverview>('/api/v1/laps', cfg)
      qc.setQueryData(qk.laps, r)
      toast.success('Saved')
      return true
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
      return false
    }
  }

  const rotate = async (r: LAPSStatus) => {
    try {
      await api.post(`/api/v1/computers/${r.computer_id}/laps/rotate`)
      toast.success('Rotation requested', `${r.computer_name} will set a new password shortly.`)
      await qc.invalidateQueries({ queryKey: qk.laps })
    } catch (e) {
      toast.error('Could not request a rotation', errorMessage(e))
    }
  }

  if (q.isPending) return <LoadingBlock />
  if (q.error || !d) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'Local admin passwords unavailable'}</Alert>

  return (
    <>
      <PageHeader title="Local admin passwords" description={summaryLine(d)} />
      <div className="flex flex-col gap-6">
        {d.config.enabled && (d.computers?.length ?? 0) > 0 && (
          <Card>
            <Table<LAPSStatus>
              rows={d.computers ?? []}
              rowKey={(r) => r.computer_id ?? ''}
              columns={[
                {
                  key: 'name',
                  header: 'Computer',
                  render: (r) => (
                    <Link to={`/computers/${r.computer_id}/security`} className="font-medium text-ink hover:underline">
                      {r.computer_name || r.serial_number}
                    </Link>
                  ),
                },
                {
                  key: 'status',
                  header: 'Status',
                  render: (r) => (
                    <span title={r.last_error || undefined}>
                      <LapsStatusBadge status={r.rotation_status} />
                    </span>
                  ),
                },
                { key: 'at', header: 'Last changed', render: (r) => <RelativeTime value={r.escrowed_at} emptyLabel="—" className="text-ink-muted" /> },
                {
                  key: 'actions',
                  header: '',
                  align: 'right',
                  render: (r) =>
                    isAdmin && (
                      <span className="inline-flex items-center gap-1">
                        {r.has_password && <LapsRevealButton computerId={r.computer_id!} computerName={r.computer_name} onDone={() => qc.invalidateQueries({ queryKey: qk.laps })} />}
                        <Button size="xs" variant="ghost" icon={<RefreshCw />} onClick={() => rotate(r)}>
                          Rotate
                        </Button>
                      </span>
                    ),
                },
              ]}
            />
          </Card>
        )}
        <PolicyCard cfg={d.config} readOnly={!isAdmin} onSave={save} />
      </div>
    </>
  )
}

function PolicyCard({ cfg, readOnly, onSave }: { cfg: LAPSConfig; readOnly: boolean; onSave: (c: LAPSConfig) => Promise<boolean> }) {
  const seed = JSON.stringify(cfg)
  const [f, setF] = useState<LAPSConfig>(() => ({ ...cfg, scope: normalizeScope(cfg.scope ?? emptyScope()) }))
  const [saving, setSaving] = useState(false)
  useEffect(() => {
    setF({ ...cfg, scope: normalizeScope(cfg.scope ?? emptyScope()) })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seed])
  const set = (p: Partial<LAPSConfig>) => setF((x) => ({ ...x, ...p }))
  const num = (v: string, fallback: number) => (Number.isFinite(parseInt(v, 10)) ? parseInt(v, 10) : fallback)

  return (
    <Card>
      <CardHeader title="Settings" />
      <CardBody className="flex flex-col gap-5">
        <Toggle
          checked={f.enabled}
          onChange={(v) => set({ enabled: v })}
          disabled={readOnly}
          label="Manage a local administrator password on each Mac"
          description="The agent sets a random password, keeps it encrypted in VibeMDM, and changes it after anyone views it."
        />
        {f.enabled && (readOnly ? <ScopeSummary scope={f.scope} /> : <ScopeEditor value={f.scope} onChange={(scope) => set({ scope })} />)}
        {f.enabled && (
          <details className="group rounded-lg border border-line">
            <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium text-ink-muted hover:text-ink">More options</summary>
            <div className="flex flex-col gap-5 border-t border-line p-4">
              <Field label="Rotate after viewing (minutes)" hint="A password always changes after someone views it. This is how long you have to use it first.">
                <Input type="number" min={1} max={10080} value={f.rotate_after_use_minutes} onChange={(e) => set({ rotate_after_use_minutes: num(e.target.value, 60) })} disabled={readOnly} className="max-w-32" />
              </Field>
              <div className="flex flex-col gap-3">
                <Toggle checked={f.auto_rotate} onChange={(v) => set({ auto_rotate: v })} disabled={readOnly} label="Also rotate on a schedule" description="Off means a password that nobody views is never changed, as Jamf Pro does by default." />
                {f.auto_rotate && (
                  <Field label="Every (days)">
                    <Input type="number" min={1} max={3650} value={f.rotation_days} onChange={(e) => set({ rotation_days: num(e.target.value, 90) })} disabled={readOnly} className="max-w-32" />
                  </Field>
                )}
              </div>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <Field label="Password style">
                  <Select
                    value={f.password_format}
                    onChange={(e) => {
                      const v = e.target.value as LAPSConfig['password_format']
                      set({ password_format: v, password_length: v === 'jamf' ? 30 : 20 })
                    }}
                    disabled={readOnly}
                    options={[
                      { value: 'jamf', label: 'Easy to type (A–Z, 2–9, dashes)' },
                      { value: 'complex', label: 'Complex (all character types)' },
                    ]}
                  />
                </Field>
                <Field label="Length">
                  <Input type="number" min={14} max={64} value={f.password_length} onChange={(e) => set({ password_length: num(e.target.value, 30) })} disabled={readOnly} />
                </Field>
              </div>
              <Field label="Account name" hint="Changing it manages a new account; existing accounts are left on the Macs.">
                <Input value={f.account_name} onChange={(e) => set({ account_name: e.target.value })} disabled={readOnly} mono />
              </Field>
              <Toggle checked={f.create_account} onChange={(v) => set({ create_account: v })} disabled={readOnly} label="Create the account if it is missing" />
              <Toggle checked={f.hide_account} onChange={(v) => set({ hide_account: v })} disabled={readOnly} label="Hide the account from the login window" />
            </div>
          </details>
        )}
      </CardBody>
      {!readOnly && (
        <CardFooter>
          <Button
            variant="primary"
            icon={<Save />}
            loading={saving}
            onClick={async () => {
              setSaving(true)
              await onSave(f)
              setSaving(false)
            }}
          >
            Save
          </Button>
        </CardFooter>
      )}
    </Card>
  )
}
