import { useCallback, useEffect, useState } from 'react'
import { Bot, Eye, EyeOff, RefreshCw, X } from 'lucide-react'
import { qk, useInvalidate, useSettings } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { SettingsUpdate, SettingsView } from '../../api/types'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
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
import { useEditMode } from '../../components/editor/useEditMode'
import { ObjectBar } from '../../components/editor/ObjectBar'

/** The parts of the server settings form, each shown on its own Settings page. */
export type GeneralSection = 'general' | 'enrollment' | 'remote-shell' | 'ai' | 'logging' | 'signing' | 'self-service'

export function GeneralSettings({ section = 'general' }: { section?: GeneralSection }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const settings = useSettings()
  const [form, setForm] = useState<SettingsView | null>(null)
  const [logDays, setLogDays] = useState('30')
  const [certPem, setCertPem] = useState('')
  const [keyPem, setKeyPem] = useState('')
  const [clearSigning, setClearSigning] = useState(false)
  const [showSecret, setShowSecret] = useState(false)
  const [saving, setSaving] = useState(false)
  const [rotateOpen, setRotateOpen] = useState(false)
  const [rotating, setRotating] = useState(false)

  const load = useCallback(() => {
    if (!settings.data) return
    setForm(settings.data)
    setLogDays(String(settings.data.log_retention_days))
    setCertPem('')
    setKeyPem('')
    setClearSigning(false)
  }, [settings.data])
  const ed = useEditMode({ isNew: false, canEdit: isAdmin, snapshot: { form, logDays, certPem, keyPem, clearSigning }, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])

  const logN = Number(logDays)
  const logInvalid = !Number.isInteger(logN) || logN < 1 || logN > 3650

  const save = async () => {
    if (!form) return false
    if (section === 'logging' && logInvalid) {
      toast.error('Check the retention', 'Enter a whole number of days between 1 and 3650.')
      return false
    }
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
        self_service_app_disabled: form.self_service_app_disabled,
        self_service_app_name: form.self_service_app_name,
        self_service_accent_color: form.self_service_accent_color,
        self_service_welcome: form.self_service_welcome,
      }
      if (!logInvalid) {
        body.log_retention_days = logN
        body.log_archive_enabled = form.log_archive_enabled
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
      toast.success(section === 'logging' ? 'Logging settings saved' : 'Settings saved')
      return true
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
      return false
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
  const ro = ed.readOnly
  const set = (patch: Partial<SettingsView>) => setForm((cur) => (cur ? { ...cur, ...patch } : cur))
  const secret = settings.data?.agent_enrollment_secret ?? ''

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
                <Input readOnly mono type={showSecret ? 'text' : 'password'} value={secret} className="flex-1 min-w-[240px]" />
                <Button icon={showSecret ? <EyeOff /> : <Eye />} onClick={() => setShowSecret((s) => !s)} aria-label={showSecret ? 'Hide' : 'Show'} />
                <CopyButton text={secret} />
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
              Enabling remote shell grants administrators an interactive <strong>root</strong> shell to any agent-enrolled Mac that is currently online. All terminal output is recorded to a session transcript visible under Activity (input the shell does not echo, such as passwords, is not recorded). Only enable this if your team is authorized to use it.
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

      {section === 'self-service' && (
        <Card>
          <CardHeader
            title="Self Service app"
            description="An app on every Mac with the agent where people install the apps and run the policies you make available, like Jamf Self Service+. Built with swiftDialog, which the agent installs automatically."
            actions={form.self_service_app_disabled ? <Badge>Off</Badge> : <Badge tone="good" dot>On</Badge>}
          />
          <CardBody className="flex flex-col gap-5">
            <Toggle
              checked={!form.self_service_app_disabled}
              onChange={(v) => set({ self_service_app_disabled: !v })}
              disabled={ro}
              label="Install the Self Service app"
              description="Added to /Applications at enrollment and kept up to date at each check-in. Turning this off removes it."
            />
            <FormGrid cols={2}>
              <Field label="App name" hint="Shown in /Applications, the Dock and the window title.">
                <Input value={form.self_service_app_name} onChange={(e) => set({ self_service_app_name: e.target.value })} disabled={ro} placeholder={form.self_service_app_default_name} maxLength={60} />
              </Field>
              <Field label="Accent colour" hint="Used for the icon, buttons and banner.">
                <div className="flex items-center gap-2">
                  <input type="color" value={form.self_service_accent_color || '#7C3AED'} onChange={(e) => set({ self_service_accent_color: e.target.value.toUpperCase() })} disabled={ro} className="h-9 w-12 cursor-pointer rounded-md border border-line bg-transparent" aria-label="Accent colour" />
                  <Input value={form.self_service_accent_color} onChange={(e) => set({ self_service_accent_color: e.target.value })} disabled={ro} mono className="max-w-[9rem]" />
                </div>
              </Field>
              <Field label="Welcome text" hint="The banner on the Discover page." className="md:col-span-2">
                <Textarea rows={2} value={form.self_service_welcome} onChange={(e) => set({ self_service_welcome: e.target.value })} disabled={ro} maxLength={300} placeholder="Apps and tools your IT team has made available for this Mac." />
              </Field>
            </FormGrid>
            <p className="text-xs text-ink-muted">
              Items appear when a policy or Mac app has <em>Make available in Self Service</em> on and the Mac is in its scope. Untested on real hardware.
            </p>
          </CardBody>
        </Card>
      )}

      {section === 'logging' && <LoggingCard days={logDays} setDays={setLogDays} invalid={logInvalid} archive={form.log_archive_enabled} setArchive={(v) => set({ log_archive_enabled: v })} readOnly={ro} />}

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
              {!ro && form.signing_configured && (
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

      {section === 'general' && <p className="text-xs text-ink-muted">Server version {form.version}</p>}

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
      {isAdmin && <ObjectBar ed={ed} isNew={false} listPath={section === 'general' ? '/settings' : `/settings/${section}`} noun="settings" saving={saving} onSave={save} noDone />}
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

/** Log retention and archiving. Saved with the main settings form. */
function LoggingCard({ days, setDays, invalid, archive, setArchive, readOnly }: { days: string; setDays: (v: string) => void; invalid: boolean; archive: boolean; setArchive: (v: boolean) => void; readOnly: boolean }) {
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
    </Card>
  )
}
