import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { CheckCircle2, Download, Plus, Trash2, XCircle } from 'lucide-react'
import { qk, useEnrollmentStatus, useInvalidate, useInvitations } from '../api/queries'
import { api, downloadFile, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { EnrollmentInvitation, InvitationCreated } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Alert } from '../components/ui/Alert'
import { Badge } from '../components/ui/Badge'
import { Table } from '../components/ui/Table'
import { Toggle } from '../components/ui/Checkbox'
import { Modal } from '../components/ui/Modal'
import { Field, FormGrid } from '../components/ui/Field'
import { Input } from '../components/ui/Input'
import { CodeBlock, CopyButton } from '../components/ui/CopyButton'
import { RelativeTime } from '../components/ui/RelativeTime'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { LoadingBlock } from '../components/ui/Spinner'
import { useToast } from '../components/ui/Toast'
import { absoluteTime } from '../lib/format'

const CONFIGURATOR_HELP =
  "For iPhone and iPad: Apple Configurator → Prepare → Manual Configuration → Supervise → Enroll in MDM Server → New server, paste this URL. Macs can't be supervised by Configurator without Apple Business Manager; enroll them with the profile link or the agent installer."

function Check({ ok, label, detail }: { ok: boolean; label: string; detail?: string }) {
  return (
    <div className="flex items-start gap-2.5">
      {ok ? <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-good" /> : <XCircle className="mt-0.5 size-4 shrink-0 text-critical" />}
      <div>
        <p className="text-sm text-ink">{label}</p>
        {detail && <p className="text-xs text-ink-muted">{detail}</p>}
      </div>
    </div>
  )
}

export function EnrollmentPage() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const status = useEnrollmentStatus()
  const invitations = useInvitations()
  const [createOpen, setCreateOpen] = useState(false)
  const [created, setCreated] = useState<InvitationCreated | null>(null)
  const [del, setDel] = useState<EnrollmentInvitation | null>(null)
  const [downloading, setDownloading] = useState(false)
  const s = status.data

  const downloadProfile = async () => {
    setDownloading(true)
    try {
      await downloadFile('/api/v1/enrollment/profile', { filename: 'vibemdm-enroll.mobileconfig' })
    } catch (e) {
      toast.error('Could not generate enrollment profile', errorMessage(e))
    } finally {
      setDownloading(false)
    }
  }

  const toggleInv = async (inv: EnrollmentInvitation, enabled: boolean) => {
    try {
      await api.patch(`/api/v1/enrollment/invitations/${inv.id}`, { enabled })
      await invalidate(qk.invitations, qk.enrollmentStatus)
    } catch (e) {
      toast.error('Update failed', errorMessage(e))
    }
  }
  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/enrollment/invitations/${del.id}`)
      toast.success('Invitation deleted')
      await invalidate(qk.invitations, qk.enrollmentStatus)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader
        title="Enrollment"
        description="Enroll Macs in MDM with a profile, and install the agent with a one-line command."
        actions={
          <Button variant="primary" icon={<Download />} loading={downloading} onClick={downloadProfile} disabled={!s?.apns_configured}>
            Download enrollment profile
          </Button>
        }
      />
      {status.isPending && <LoadingBlock />}
      {status.error && <Alert tone="critical">{errorMessage(status.error)}</Alert>}
      {s && (
        <div className="flex flex-col gap-6">
          {s.issues.length > 0 && (
            <Alert
              tone="warning"
              title="Enrollment is not ready"
              actions={
                <Link to="/settings">
                  <Button size="sm">Open Settings</Button>
                </Link>
              }
            >
              <ul className="list-disc pl-4 space-y-0.5">
                {s.issues.map((i) => (
                  <li key={i}>{i}</li>
                ))}
              </ul>
            </Alert>
          )}
          <div className="grid gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader title="Readiness" description="Everything a Mac needs to enroll." />
              <CardBody className="flex flex-col gap-3">
                <Check ok={!!s.server_url && s.server_url.startsWith('https://')} label="Public HTTPS server URL" detail={s.server_url || 'Not set'} />
                <Check ok={s.apns_configured} label="APNs push certificate" detail={s.apns_configured ? `Topic: ${(s.topics ?? []).join(', ')}` : 'Upload one under Settings → APNs'} />
                <Check ok={s.scep_ok} label="SCEP certificate authority" detail="Issues device identity certificates" />
                <Check ok={Object.values(s.agent_binaries).some(Boolean)} label="Agent binaries" detail={Object.entries(s.agent_binaries).map(([k, v]) => `${k}: ${v ? 'present' : 'missing'}`).join(' · ')} />
                <Check ok={true} label={s.open_enrollment ? 'Open enrollment is on' : 'Open enrollment is off'} detail={s.open_enrollment ? 'Any Mac with the profile or install command can enroll.' : 'Agents need the enrollment secret or an invitation token.'} />
              </CardBody>
            </Card>
            <Card>
              <CardHeader title="Enroll a Mac" description="Two steps: install the MDM profile, then install the agent." />
              <CardBody className="flex flex-col gap-5">
                <div>
                  <p className="text-sm font-medium text-ink">1. MDM enrollment profile</p>
                  <p className="mt-0.5 text-xs text-ink-muted">Download the profile and open it on the Mac, or share the enrollment URL. The user approves it in System Settings → Profiles.</p>
                  <div className="mt-2 flex items-center gap-2">
                    <Input readOnly mono value={s.enroll_url} className="flex-1" />
                    <CopyButton text={s.enroll_url} />
                  </div>
                </div>
                <div>
                  <p className="text-sm font-medium text-ink">2. Install the agent</p>
                  <p className="mt-0.5 text-xs text-ink-muted">Run this in Terminal on the Mac. It downloads the agent for the Mac's architecture and enrolls with the shared secret.</p>
                  {isAdmin && s.install_command ? (
                    <CodeBlock text={s.install_command} className="mt-2" filename="Terminal" />
                  ) : (
                    <Alert tone="info" className="mt-2">
                      The install command includes the enrollment secret and is only shown to administrators. Create an invitation below to get a scoped command.
                    </Alert>
                  )}
                </div>
                {isAdmin && s.configurator_url && (
                  <div className="border-t border-line pt-5">
                    <p className="text-sm font-medium text-ink">Apple Configurator URL</p>
                    <p className="mt-0.5 text-xs text-ink-muted">{CONFIGURATOR_HELP}</p>
                    <div className="mt-2 flex items-center gap-2">
                      <Input readOnly mono value={s.configurator_url} className="flex-1" />
                      <CopyButton text={s.configurator_url} />
                    </div>
                  </div>
                )}
              </CardBody>
            </Card>
          </div>

          <Card>
            <CardHeader
              title="Enrollment invitations"
              description="Shareable tokens with their own usage limits and expiry. Useful for contractors or one-off enrollments without exposing the global secret."
              actions={
                isAdmin && (
                  <Button variant="primary" icon={<Plus />} onClick={() => setCreateOpen(true)}>
                    New invitation
                  </Button>
                )
              }
            />
            <Table<EnrollmentInvitation>
              rows={invitations.data?.items}
              loading={invitations.isPending}
              rowKey={(i) => i.id}
              emptyTitle="No invitations yet"
              emptyDescription="Create an invitation to get a unique enrollment link and install command."
              columns={[
                { key: 'name', header: 'Name', render: (i) => <span className="font-medium text-ink">{i.name || 'Untitled'}</span> },
                { key: 'prefix', header: 'Token', render: (i) => <span className="font-mono text-xs text-ink-muted">{i.token_prefix}…</span> },
                {
                  key: 'uses',
                  header: 'Uses',
                  render: (i) => (
                    <span className="tabular-nums">
                      {i.uses}
                      {i.max_uses != null ? ` / ${i.max_uses}` : ''}
                    </span>
                  ),
                },
                {
                  key: 'expires',
                  header: 'Expires',
                  render: (i) => {
                    const expired = i.expires_at && new Date(i.expires_at) < new Date()
                    return i.expires_at ? (
                      <span className={expired ? 'text-critical' : 'text-ink-muted'} title={absoluteTime(i.expires_at)}>
                        {expired ? 'Expired ' : ''}
                        <RelativeTime value={i.expires_at} />
                      </span>
                    ) : (
                      <span className="text-ink-muted">Never</span>
                    )
                  },
                },
                {
                  key: 'enabled',
                  header: 'Enabled',
                  render: (i) => (isAdmin ? <Toggle size="sm" checked={i.enabled} onChange={(v) => toggleInv(i, v)} /> : <Badge tone={i.enabled ? 'good' : 'neutral'}>{i.enabled ? 'Enabled' : 'Disabled'}</Badge>),
                },
                { key: 'created', header: 'Created', render: (i) => <RelativeTime value={i.created_at} className="text-ink-muted" /> },
                {
                  key: 'actions',
                  header: '',
                  align: 'right',
                  render: (i) => (isAdmin ? <Button size="xs" variant="ghost" icon={<Trash2 />} aria-label="Delete" onClick={() => setDel(i)} /> : null),
                },
              ]}
            />
          </Card>
        </div>
      )}
      <CreateInvitationDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(r) => {
          setCreateOpen(false)
          setCreated(r)
        }}
      />
      <Modal open={!!created} onClose={() => setCreated(null)} title="Invitation created" description="Copy these now. The full token is not shown again." size="lg" footer={<Button variant="primary" onClick={() => setCreated(null)}>Done</Button>}>
        {created && (
          <div className="flex flex-col gap-4">
            <Field label="Token">
              <div className="flex items-center gap-2">
                <Input readOnly mono value={created.token} className="flex-1" />
                <CopyButton text={created.token} />
              </div>
            </Field>
            <Field label="Enrollment URL" hint="Open on the Mac to download the MDM profile.">
              <div className="flex items-center gap-2">
                <Input readOnly mono value={created.enroll_url} className="flex-1" />
                <CopyButton text={created.enroll_url} />
              </div>
            </Field>
            <Field label="Agent install command">
              <CodeBlock text={created.install_command} filename="Terminal" />
            </Field>
            {created.configurator_url && (
              <Field label="Apple Configurator URL" hint={CONFIGURATOR_HELP}>
                <div className="flex items-center gap-2">
                  <Input readOnly mono value={created.configurator_url} className="flex-1" />
                  <CopyButton text={created.configurator_url} />
                </div>
              </Field>
            )}
          </div>
        )}
      </Modal>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete invitation "${del?.name || del?.token_prefix}"?`} description="Links and install commands using this token stop working immediately." confirmLabel="Delete invitation" />
    </>
  )
}

function CreateInvitationDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (r: InvitationCreated) => void }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const [name, setName] = useState('')
  const [maxUses, setMaxUses] = useState('')
  const [days, setDays] = useState('7')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const r = await api.post<InvitationCreated>('/api/v1/enrollment/invitations', {
        name,
        max_uses: maxUses ? Number(maxUses) : null,
        expires_days: days ? Number(days) : null,
      })
      await invalidate(qk.invitations, qk.enrollmentStatus)
      setName('')
      setMaxUses('')
      setDays('7')
      onCreated(r)
    } catch (err) {
      toast.error('Could not create invitation', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open={open} onClose={onClose} title="New enrollment invitation" size="sm">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field label="Name" hint="For your reference, e.g. “Contractor laptops”.">
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </Field>
        <FormGrid cols={2}>
          <Field label="Maximum uses" hint="Leave blank for unlimited.">
            <Input type="number" min={1} value={maxUses} onChange={(e) => setMaxUses(e.target.value)} placeholder="Unlimited" />
          </Field>
          <Field label="Expires in (days)" hint="Leave blank to never expire.">
            <Input type="number" min={1} value={days} onChange={(e) => setDays(e.target.value)} placeholder="Never" />
          </Field>
        </FormGrid>
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={busy}>
            Create invitation
          </Button>
        </div>
      </form>
    </Modal>
  )
}
