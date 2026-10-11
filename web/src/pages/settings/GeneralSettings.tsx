import { useEffect, useState } from 'react'
import { Bot, Eye, EyeOff, RefreshCw, Save, X } from 'lucide-react'
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
import { CodeBlock, CopyButton } from '../../components/ui/CopyButton'
import { useToast } from '../../components/ui/Toast'
import { statusKey } from '../../auth/AuthContext'

/** The parts of the server settings form, each shown on its own Settings page. */
export type GeneralSection = 'general' | 'enrollment' | 'remote-shell' | 'ai' | 'logging' | 'signing'

export function GeneralSettings({ section = 'general' }: { section?: GeneralSection }) {
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
        mcp_enabled: form.mcp_enabled,
        mcp_allow_writes: form.mcp_enabled && form.mcp_allow_writes,
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
      {section === 'general' && (
        <>
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
        </>
      )}

      {section === 'enrollment' && (
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
      )}

      {section === 'remote-shell' && isAdmin && (
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

      {section === 'ai' && <MCPCard form={form} set={set} readOnly={ro} />}

      {section === 'logging' && <LoggingCard settings={settings.data} readOnly={ro} />}

      {section === 'signing' && (
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
      </Card>
      )}

      {isAdmin && section !== 'logging' && (
        <div className="flex items-center justify-end gap-3">
          {section === 'general' && <span className="mr-auto text-xs text-ink-muted">Server version {form.version}</span>}
          <Button variant="primary" icon={<Save />} loading={saving} onClick={save}>
            Save
          </Button>
        </div>
      )}

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

/** MCP server for AI agents. Saved with the main settings form. */
function MCPCard({ form, set, readOnly }: { form: SettingsView; set: (patch: Partial<SettingsView>) => void; readOnly: boolean }) {
  const base = (form.server_url || window.location.origin).replace(/\/+$/, '')
  const endpoint = `${base}/mcp`
  const https = base.startsWith('https://')
  const snippet = `claude mcp add --transport http vibemdm ${endpoint} --header "Authorization: Bearer <token>"`
  return (
    <Card>
      <CardHeader
        title="MCP server for AI agents"
        description="Lets AI agents such as Claude Code answer questions about the fleet, and optionally take a few safe actions, through the Model Context Protocol."
        actions={
          form.mcp_enabled ? (
            <Badge tone={form.mcp_allow_writes ? 'accent' : 'good'} dot>
              {form.mcp_allow_writes ? 'Enabled · writes allowed' : 'Enabled · read-only'}
            </Badge>
          ) : (
            <Badge>Disabled</Badge>
          )
        }
      />
      <CardBody className="flex flex-col gap-5">
        <Toggle
          checked={form.mcp_enabled}
          onChange={(v) => set({ mcp_enabled: v, ...(v ? {} : { mcp_allow_writes: false }) })}
          disabled={readOnly}
          label="Enable MCP server"
          description={
            <>
              Serves the endpoint <code className="font-mono text-ink">{endpoint}</code>. It needs an API token (Settings → API Tokens); browser sessions are rejected. Viewer tokens get read-only tools.
            </>
          }
        />
        <Toggle
          checked={form.mcp_enabled && form.mcp_allow_writes}
          onChange={(v) => set({ mcp_allow_writes: v })}
          disabled={readOnly || !form.mcp_enabled}
          label="Allow write tools (admin tokens only)"
          description="Admin tokens can send pushes, request inventory, run existing policies, acknowledge or fix health issues and start compliance scans. Nothing destructive is exposed, and every call is written to the audit log."
        />
        {form.mcp_enabled && !https && (
          <Alert tone="warning" title="HTTPS is required">
            The server URL is not HTTPS, so tokens would travel in clear text. Health flags MCP without HTTPS{form.mcp_allow_writes ? ', and raises it to critical while write tools are allowed' : ''}.
          </Alert>
        )}
        <div className="flex flex-col gap-1.5">
          <p className="text-sm font-medium text-ink">Connect Claude Code</p>
          <CodeBlock text={snippet} filename="Replace <token> with an API token" />
          <p className="text-xs text-ink-muted">
            Other MCP clients use the same URL with the header <code className="font-mono text-ink">Authorization: Bearer &lt;token&gt;</code>.
          </p>
        </div>
        <p className="inline-flex items-center gap-1.5 text-xs text-ink-muted">
          <Bot className="size-3.5 text-ink-faint" aria-hidden="true" />
          Read the MCP guide in <code className="font-mono text-ink">docs/mcp.md</code> for the full list of tools and the security model.
        </p>
      </CardBody>
    </Card>
  )
}

/** Log retention and archiving. Keeps its own edit state and saves only its two fields. */
function LoggingCard({ settings, readOnly }: { settings: SettingsView | undefined; readOnly: boolean }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const [days, setDays] = useState(String(settings?.log_retention_days ?? 30))
  const [archive, setArchive] = useState(!!settings?.log_archive_enabled)
  const [saving, setSaving] = useState(false)
  useEffect(() => {
    if (!settings) return
    setDays(String(settings.log_retention_days))
    setArchive(settings.log_archive_enabled)
  }, [settings?.log_retention_days, settings?.log_archive_enabled]) // eslint-disable-line react-hooks/exhaustive-deps

  const n = Number(days)
  const invalid = !Number.isInteger(n) || n < 1 || n > 3650
  const dirty = !!settings && (n !== settings.log_retention_days || archive !== settings.log_archive_enabled)

  const save = async () => {
    if (invalid) return
    setSaving(true)
    try {
      const body: SettingsUpdate = { log_retention_days: n, log_archive_enabled: archive }
      await api.put('/api/v1/settings', body)
      await invalidate(qk.settings)
      toast.success('Logging settings saved')
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card>
      <CardHeader title="Logging" description="How long server and agent logs are kept in the database, and whether they are archived before being pruned." />
      <CardBody className="flex flex-col gap-5">
        <Field label="Retention (days)" hint="1–3650. Entries older than this are pruned daily." error={invalid ? 'Enter a whole number between 1 and 3650.' : undefined} className="max-w-xs">
          <Input type="number" min={1} max={3650} step={1} value={days} onChange={(e) => setDays(e.target.value)} disabled={readOnly} />
        </Field>
        <Toggle
          checked={archive}
          onChange={setArchive}
          disabled={readOnly}
          label="Archive before pruning"
          description="Writes daily gzipped NDJSON files to the server's data volume (log-archive/) before old entries are pruned. Download them from Logs → Archives."
        />
        <p className="text-xs text-ink-muted">
          The minimum level stored in the database is set with the <code className="font-mono text-ink">VIBEMDM_LOG_DB_LEVEL</code> environment variable on the server (default <code className="font-mono text-ink">info</code>).
        </p>
      </CardBody>
      {!readOnly && (
        <CardFooter>
          <Button variant="primary" icon={<Save />} loading={saving} onClick={save} disabled={invalid || !dirty}>
            Save logging
          </Button>
        </CardFooter>
      )}
    </Card>
  )
}
