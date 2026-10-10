import { useEffect, useState } from 'react'
import { Eye, EyeOff, RefreshCw, Save, X } from 'lucide-react'
import { qk, useInvalidate, useSettings } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { SettingsUpdate, SettingsView } from '../../api/types'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Textarea } from '../../components/ui/Input'
import { Toggle } from '../../components/ui/Checkbox'
import { Alert } from '../../components/ui/Alert'
import { Badge } from '../../components/ui/Badge'
import { LoadingBlock } from '../../components/ui/Spinner'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { CopyButton } from '../../components/ui/CopyButton'
import { useToast } from '../../components/ui/Toast'
import { statusKey } from '../../auth/AuthContext'

export function GeneralSettings() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const settings = useSettings()
  const [form, setForm] = useState<SettingsView | null>(null)
  const [certPem, setCertPem] = useState('')
  const [keyPem, setKeyPem] = useState('')
  const [clearSigning, setClearSigning] = useState(false)
  const [showSecret, setShowSecret] = useState(false)
  const [saving, setSaving] = useState(false)
  const [rotateOpen, setRotateOpen] = useState(false)
  const [rotating, setRotating] = useState(false)

  useEffect(() => {
    if (settings.data) setForm(settings.data)
  }, [settings.data])

  const save = async () => {
    if (!form) return
    setSaving(true)
    try {
      const body: SettingsUpdate = {
        org_name: form.org_name,
        server_url: form.server_url,
        agent_checkin_interval_seconds: form.agent_checkin_interval_seconds,
        inventory_interval_hours: form.inventory_interval_hours,
        enrollment_profile_name: form.enrollment_profile_name,
        enrollment_profile_identifier: form.enrollment_profile_identifier,
        open_enrollment: form.open_enrollment,
        remote_shell_enabled: form.remote_shell_enabled,
        remote_shell_idle_timeout_secs: form.remote_shell_idle_timeout_secs,
      }
      if (clearSigning) body.clear_signing = true
      else if (certPem.trim() && keyPem.trim()) {
        body.signing_cert_pem = certPem
        body.signing_key_pem = keyPem
      }
      await api.put('/api/v1/settings', body)
      await invalidate(qk.settings, statusKey, qk.enrollmentStatus)
      setCertPem('')
      setKeyPem('')
      setClearSigning(false)
      toast.success('Settings saved')
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  const rotate = async () => {
    setRotating(true)
    try {
      await api.post('/api/v1/settings/rotate-enrollment-secret')
      await invalidate(qk.settings, qk.enrollmentStatus)
      toast.success('Enrollment secret rotated', 'Existing install commands using the old secret no longer work.')
      setRotateOpen(false)
    } catch (e) {
      toast.error('Rotation failed', errorMessage(e))
    } finally {
      setRotating(false)
    }
  }

  if (settings.isPending || !form) return <LoadingBlock />
  if (settings.error) return <Alert tone="critical">{errorMessage(settings.error)}</Alert>
  const ro = !isAdmin
  const set = (patch: Partial<SettingsView>) => setForm({ ...form, ...patch })

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader title="Organization and server" />
        <CardBody>
          <FormGrid cols={2}>
            <Field label="Organization name" hint="Shown in the console and on enrollment profiles.">
              <Input value={form.org_name} onChange={(e) => set({ org_name: e.target.value })} disabled={ro} />
            </Field>
            <Field label="Server URL" hint="Public HTTPS URL devices use to reach this server. Must match your TLS certificate.">
              <Input value={form.server_url} onChange={(e) => set({ server_url: e.target.value })} disabled={ro} placeholder="https://mdm.example.com" mono />
            </Field>
            <Field label="Agent check-in interval (seconds)" hint="30–86400. How often the agent polls for tasks and triggers check-in policies.">
              <Input type="number" min={30} max={86400} value={form.agent_checkin_interval_seconds} onChange={(e) => set({ agent_checkin_interval_seconds: Number(e.target.value) || 0 })} disabled={ro} />
            </Field>
            <Field label="Inventory interval (hours)" hint="How often the agent submits a full inventory report.">
              <Input type="number" min={1} value={form.inventory_interval_hours} onChange={(e) => set({ inventory_interval_hours: Number(e.target.value) || 0 })} disabled={ro} />
            </Field>
          </FormGrid>
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title="Encryption at rest"
          description={
            form.encryption_at_rest
              ? 'Secrets (signing keys, escrowed FileVault keys, APNs/SCEP/escrow private keys) are sealed in the database.'
              : <>Secrets are stored unencrypted in the database. Set <code className="font-mono">VIBEMDM_ENCRYPTION_KEY</code> on the server and restart to seal them.</>
          }
          actions={
            form.encryption_at_rest ? (
              <Badge tone="good" dot>
                Enabled
              </Badge>
            ) : (
              <Badge tone="warning" dot>
                Disabled — set VIBEMDM_ENCRYPTION_KEY
              </Badge>
            )
          }
          className="border-b-0"
        />
      </Card>

      <Card>
        <CardHeader title="Enrollment" />
        <CardBody className="flex flex-col gap-5">
          <FormGrid cols={2}>
            <Field label="Enrollment profile name" hint="Display name of the MDM profile users see.">
              <Input value={form.enrollment_profile_name} onChange={(e) => set({ enrollment_profile_name: e.target.value })} disabled={ro} />
            </Field>
            <Field label="Enrollment profile identifier" hint="Reverse-DNS identifier. Changing it after devices enroll makes them appear unmanaged.">
              <Input value={form.enrollment_profile_identifier} onChange={(e) => set({ enrollment_profile_identifier: e.target.value })} disabled={ro} mono />
            </Field>
          </FormGrid>
          <Toggle checked={form.open_enrollment} onChange={(v) => set({ open_enrollment: v })} disabled={ro} label="Open enrollment" description="Allow any Mac to enroll with the profile or install command without an invitation token." />
          {isAdmin && (
            <Field label="Agent enrollment secret" hint="Embedded in the agent install command. Rotate it if it leaks.">
              <div className="flex flex-wrap items-center gap-2">
                <Input readOnly mono type={showSecret ? 'text' : 'password'} value={form.agent_enrollment_secret ?? ''} className="flex-1 min-w-[240px]" />
                <Button icon={showSecret ? <EyeOff /> : <Eye />} onClick={() => setShowSecret((s) => !s)} aria-label={showSecret ? 'Hide' : 'Show'} />
                <CopyButton text={form.agent_enrollment_secret ?? ''} />
                <Button variant="danger-outline" icon={<RefreshCw />} onClick={() => setRotateOpen(true)}>
                  Rotate
                </Button>
              </div>
            </Field>
          )}
        </CardBody>
      </Card>

      {isAdmin && (
        <Card>
          <CardHeader title="Remote shell" description="Interactive terminal access to online Macs from the console." actions={form.remote_shell_enabled ? <Badge tone="good" dot>Enabled</Badge> : <Badge>Disabled</Badge>} />
          <CardBody className="flex flex-col gap-5">
            <Alert tone="warning" title="High-privilege feature">
              Enabling remote shell grants administrators an interactive <strong>root</strong> shell to any agent-enrolled Mac that is currently online. Every keystroke and all output is recorded to a session transcript visible under Activity. Only enable this if your team is authorized to use it.
            </Alert>
            <Toggle
              checked={form.remote_shell_enabled}
              onChange={(v) => set({ remote_shell_enabled: v })}
              disabled={ro}
              label="Enable remote shell"
              description="Shows an Open Shell action on agent-enrolled computers for administrators."
            />
            <Field label="Idle timeout (seconds)" hint="Sessions with no input or output for this long are closed automatically. Minimum 30." className="max-w-xs">
              <Input
                type="number"
                min={30}
                value={form.remote_shell_idle_timeout_secs}
                onChange={(e) => set({ remote_shell_idle_timeout_secs: Math.max(30, Number(e.target.value) || 0) })}
                disabled={ro || !form.remote_shell_enabled}
              />
            </Field>
          </CardBody>
        </Card>
      )}

      <Card>
        <CardHeader
          title="Profile signing"
          description="Optionally sign configuration and enrollment profiles with a certificate so macOS shows them as Verified."
          actions={form.signing_configured ? <Badge tone="good" dot>Configured{form.signing_cert_subject ? ` · ${form.signing_cert_subject}` : ''}</Badge> : <Badge>Not configured</Badge>}
        />
        <CardBody className="flex flex-col gap-4">
          {clearSigning ? (
            <Alert tone="warning" actions={<Button size="sm" variant="ghost" icon={<X />} onClick={() => setClearSigning(false)}>Undo</Button>}>
              The signing identity will be removed when you save.
            </Alert>
          ) : (
            <>
              <FormGrid cols={2}>
                <Field label="Signing certificate (PEM)" hint="Include the full chain if your CA requires it.">
                  <Textarea mono rows={6} value={certPem} onChange={(e) => setCertPem(e.target.value)} disabled={ro} placeholder="-----BEGIN CERTIFICATE-----" />
                </Field>
                <Field label="Private key (PEM)" hint="Stored server-side; never shown again.">
                  <Textarea mono rows={6} value={keyPem} onChange={(e) => setKeyPem(e.target.value)} disabled={ro} placeholder="-----BEGIN PRIVATE KEY-----" />
                </Field>
              </FormGrid>
              {isAdmin && form.signing_configured && (
                <div>
                  <Button variant="danger-outline" size="sm" icon={<X />} onClick={() => setClearSigning(true)}>
                    Remove signing identity
                  </Button>
                </div>
              )}
            </>
          )}
        </CardBody>
        {isAdmin && (
          <CardFooter>
            <span className="mr-auto text-xs text-ink-muted">Server version {form.version}</span>
            <Button variant="primary" icon={<Save />} loading={saving} onClick={save}>
              Save settings
            </Button>
          </CardFooter>
        )}
      </Card>

      <ConfirmDialog
        open={rotateOpen}
        onClose={() => setRotateOpen(false)}
        onConfirm={rotate}
        destructive
        loading={rotating}
        title="Rotate the agent enrollment secret?"
        description="A new secret is generated immediately. Any install command or script that embeds the current secret will stop working. Already-enrolled agents are unaffected."
        confirmLabel="Rotate secret"
      />
    </div>
  )
}
