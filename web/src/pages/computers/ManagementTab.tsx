import { useState, type ReactNode } from 'react'
import {

  Bell,
  Download,
  Lock,
  LogOut,
  Monitor,
  MonitorOff,
  Play,
  Power,
  Radio,
  RefreshCw,
  RotateCcw,
  Send,
  ShieldOff,
  Terminal,
  Trash2,
  Zap,
  ListChecks,
  FileCode,
  KeyRound,
} from 'lucide-react'
import type { CheckinResponse, ComputerDetail, CreateTaskRequest, SendCommandRequest, SendCommandResponse } from '../../api/types'
import { api, errorMessage } from '../../api/client'
import { qk, useInvalidate, useScript } from '../../api/queries'
import { useAuth } from '../../auth/AuthContext'
import { CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Modal } from '../../components/ui/Modal'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Textarea } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { Badge } from '../../components/ui/Badge'
import { useToast } from '../../components/ui/Toast'
import { ScriptPicker, PolicyPicker, ProfilePicker } from '../../components/pickers/ScriptPicker'
import { cn } from '../../lib/cn'

type DialogKind =
  | 'lock'
  | 'restart'
  | 'shutdown'
  | 'erase'
  | 'remove-mdm'
  | 'install-profile'
  | 'run-script'
  | 'run-command'
  | 'run-policy'
  | 'custom-trigger'
  | 'notify'
  | 'logout'
  | 'agent-restart'
  | 'agent-shutdown'
  | 'reset-agent'
  | 'raw'
  | null

export function ManagementTab({ c }: { c: ComputerDetail }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [dialog, setDialog] = useState<DialogKind>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const close = () => setDialog(null)

  const sendCommand = async (req: SendCommandRequest, label: string) => {
    setBusy(req.type)
    try {
      const r = await api.post<SendCommandResponse>(`/api/v1/computers/${c.id}/commands`, req)
      if (r.push_error) toast.warning(`${label} queued, but push failed`, r.push_error)
      else toast.success(`${label} sent`, r.command_uuid ? `Command ${r.command_uuid.slice(0, 8)}…` : undefined)
      await invalidate(qk.computer(c.id), ['commands'], qk.dashboard)
      close()
    } catch (e) {
      toast.error(`${label} failed`, errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  const queueTask = async (req: CreateTaskRequest, label: string) => {
    setBusy(req.type)
    try {
      await api.post(`/api/v1/computers/${c.id}/tasks`, req)
      toast.success(`${label} queued`, c.agent_online ? 'The agent is online and will pick it up now.' : 'The agent will run it at its next check-in.')
      await invalidate(qk.computer(c.id), ['tasks'], qk.dashboard)
      close()
    } catch (e) {
      toast.error(`${label} failed`, errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  const blankPush = async () => {
    setBusy('push')
    try {
      await api.post(`/api/v1/computers/${c.id}/push`)
      toast.success('Push notification sent')
    } catch (e) {
      toast.error('Push failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  const checkIn = async () => {
    setBusy('checkin')
    try {
      const r = await api.post<CheckinResponse>(`/api/v1/computers/${c.id}/checkin`)
      toast.success(`Check-in requested (agent ${r.agent_online ? 'online' : 'offline'})`, r.agent_online ? 'The agent is checking in now; policies evaluate on demand.' : 'The agent will check in when it next connects.')
      await invalidate(qk.computer(c.id), ['tasks'], ['policy-logs'], qk.dashboard)
      // The agent checks in a moment later; refresh again so the new check-in time and any runs show.
      if (r.agent_online) setTimeout(() => void invalidate(qk.computer(c.id), ['tasks'], ['policy-logs'], qk.dashboard), 5000)
    } catch (e) {
      toast.error('Check-in request failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  const updateInventory = async () => {
    setBusy('inventory')
    try {
      await api.post(`/api/v1/computers/${c.id}/update-inventory`)
      toast.success('Inventory update requested')
      await invalidate(qk.computer(c.id), ['commands'], ['tasks'])
    } catch (e) {
      toast.error('Inventory update failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  const resetAgent = async () => {
    setBusy('reset-agent')
    try {
      await api.post(`/api/v1/computers/${c.id}/reset-agent`)
      toast.success('Agent token revoked', 'Reinstall the agent on this Mac to enroll it again.')
      await invalidate(qk.computer(c.id), ['computers'], qk.dashboard)
      close()
    } catch (e) {
      toast.error('Reset agent failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  if (!isAdmin) {
    return (
      <CardBody>
        <Alert tone="info">Management actions are only available to administrators.</Alert>
      </CardBody>
    )
  }

  const mdmOff = !c.mdm_enrolled
  const agentOff = !c.agent_enrolled

  return (
    <>
      <CardHeader
        title="Management"
        description="Send MDM commands and agent tasks to this computer."
        actions={
          <>
            <Badge tone={c.mdm_enrolled ? 'good' : 'neutral'} dot>
              {c.mdm_enrolled ? 'MDM enrolled' : 'MDM not enrolled'}
            </Badge>
            <Badge tone={c.agent_enrolled ? (c.agent_online ? 'good' : 'info') : 'neutral'} dot>
              {c.agent_enrolled ? (c.agent_online ? 'Agent online' : 'Agent offline') : 'Agent not installed'}
            </Badge>
          </>
        }
      />
      <CardBody className="flex flex-col gap-8">
        {mdmOff && agentOff && <Alert tone="warning">This computer is neither MDM enrolled nor running the agent, so no actions can be delivered.</Alert>}

        <Section title="Inventory" description="Refresh what VibeMDM knows about this computer.">
          <Action icon={<RefreshCw />} label="Update inventory" description="Request fresh MDM device info and an agent inventory report." onClick={updateInventory} loading={busy === 'inventory'} disabled={mdmOff && agentOff} />
          <Action icon={<Radio />} label="Check in now" description="Ask the agent to check in immediately and evaluate its policies." onClick={checkIn} loading={busy === 'checkin'} disabled={agentOff} />
          <Action icon={<Send />} label="Send blank push" description="Wake the device's MDM client so it polls for queued commands." onClick={blankPush} loading={busy === 'push'} disabled={mdmOff} />
        </Section>

        <Section title="MDM commands" description="Delivered through Apple Push Notification service. The device must be MDM enrolled.">
          <Action icon={<Lock />} label="Lock computer" description="Lock the Mac with a 6-digit PIN and an optional message." onClick={() => setDialog('lock')} disabled={mdmOff} />
          <Action icon={<RotateCcw />} label="Restart" description="Restart the computer immediately." onClick={() => setDialog('restart')} disabled={mdmOff} />
          <Action icon={<Power />} label="Shut down" description="Shut the computer down immediately." onClick={() => setDialog('shutdown')} disabled={mdmOff} />
          <Action icon={<Monitor />} label="Enable Remote Desktop" description="Turn on Remote Management for screen sharing." onClick={() => sendCommand({ type: 'EnableRemoteDesktop' }, 'Enable Remote Desktop')} loading={busy === 'EnableRemoteDesktop'} disabled={mdmOff} />
          <Action icon={<MonitorOff />} label="Disable Remote Desktop" description="Turn off Remote Management." onClick={() => sendCommand({ type: 'DisableRemoteDesktop' }, 'Disable Remote Desktop')} loading={busy === 'DisableRemoteDesktop'} disabled={mdmOff} />
          <Action icon={<Download />} label="Install profile" description="Push a configuration profile from the library to this computer." onClick={() => setDialog('install-profile')} disabled={mdmOff} />
          <Action icon={<FileCode />} label="Send raw command" description="Send a hand-written MDM command plist." onClick={() => setDialog('raw')} disabled={mdmOff} />
          <Action icon={<ShieldOff />} label="Remove MDM profile" description="Unmanage the device by removing the enrollment profile." onClick={() => setDialog('remove-mdm')} disabled={mdmOff} danger />
          <Action icon={<Trash2 />} label="Wipe / erase" description="Erase all content and settings. This cannot be undone." onClick={() => setDialog('erase')} disabled={mdmOff} danger />
        </Section>

        <Section title="Agent tasks" description="Executed by the VibeMDM agent on the device at its next check-in (immediately if online).">
          <Action icon={<Terminal />} label="Run script" description="Run a script from the library with up to 8 parameters." onClick={() => setDialog('run-script')} disabled={agentOff} />
          <Action icon={<Play />} label="Run command" description="Run a one-off shell command as root." onClick={() => setDialog('run-command')} disabled={agentOff} />
          <Action icon={<ListChecks />} label="Run policy" description="Run a policy now, ignoring its triggers and frequency." onClick={() => setDialog('run-policy')} disabled={agentOff} />
          <Action icon={<Zap />} label="Custom trigger" description="Fire a custom event name so matching policies run." onClick={() => setDialog('custom-trigger')} disabled={agentOff} />
          <Action icon={<Bell />} label="Send notification" description="Show a message to the logged-in user." onClick={() => setDialog('notify')} disabled={agentOff} />
          <Action icon={<LogOut />} label="Log out user" description="Log out the current console user." onClick={() => setDialog('logout')} disabled={agentOff} />
          <Action icon={<RotateCcw />} label="Restart (agent)" description="Ask the agent to restart the Mac, with an optional message." onClick={() => setDialog('agent-restart')} disabled={agentOff} />
          <Action icon={<Power />} label="Shut down (agent)" description="Ask the agent to shut the Mac down." onClick={() => setDialog('agent-shutdown')} disabled={agentOff} />
          <Action icon={<KeyRound />} label="Reset agent" description="Revoke the agent token before reinstalling the agent." onClick={() => setDialog('reset-agent')} loading={busy === 'reset-agent'} disabled={agentOff} danger />
        </Section>
      </CardBody>

      <ConfirmDialog
        open={dialog === 'reset-agent'}
        onClose={close}
        destructive
        title="Reset this Mac's agent?"
        description="Revokes this Mac's agent token. Use it before reinstalling the agent (for example after erasing the Mac). The current agent stops working immediately."
        confirmLabel="Reset agent"
        onConfirm={resetAgent}
        loading={busy === 'reset-agent'}
      />
      <LockDialog open={dialog === 'lock'} onClose={close} busy={busy === 'DeviceLock'} onSubmit={(pin, message, phone) => sendCommand({ type: 'DeviceLock', pin, message, phone }, 'Lock')} />
      <ConfirmDialog open={dialog === 'restart'} onClose={close} title="Restart this computer?" description="The device will restart immediately when it receives the command. Unsaved work may be lost." confirmLabel="Restart" onConfirm={() => sendCommand({ type: 'RestartDevice' }, 'Restart')} loading={busy === 'RestartDevice'} />
      <ConfirmDialog open={dialog === 'shutdown'} onClose={close} title="Shut down this computer?" description="The device will shut down immediately when it receives the command." confirmLabel="Shut down" onConfirm={() => sendCommand({ type: 'ShutDownDevice' }, 'Shut down')} loading={busy === 'ShutDownDevice'} />
      <EraseDialog open={dialog === 'erase'} onClose={close} name={c.name || c.serial_number} busy={busy === 'EraseDevice'} onSubmit={(pin) => sendCommand({ type: 'EraseDevice', pin }, 'Erase')} />
      <ConfirmDialog
        open={dialog === 'remove-mdm'}
        onClose={close}
        destructive
        title="Remove the MDM profile?"
        description="This unenrolls the device from management. Managed profiles will be removed and no further MDM commands can be sent. The agent, if installed, keeps working."
        confirmLabel="Remove MDM profile"
        onConfirm={() => sendCommand({ type: 'RemoveMDMProfile' }, 'Remove MDM profile')}
        loading={busy === 'RemoveMDMProfile'}
      />
      <InstallProfileDialog open={dialog === 'install-profile'} onClose={close} busy={busy === 'InstallProfile'} onSubmit={(profile_id) => sendCommand({ type: 'InstallProfile', profile_id }, 'Install profile')} />
      <RawCommandDialog open={dialog === 'raw'} onClose={close} busy={busy === 'Raw'} onSubmit={(plist) => sendCommand({ type: 'Raw', plist }, 'Raw command')} />
      <RunScriptDialog open={dialog === 'run-script'} onClose={close} busy={busy === 'run_script'} onSubmit={(script_id, parameters) => queueTask({ type: 'run_script', script_id, parameters }, 'Script')} />
      <TextDialog
        open={dialog === 'run-command'}
        onClose={close}
        busy={busy === 'run_command'}
        title="Run command"
        label="Shell command"
        hint="Runs as root via /bin/sh. Output is captured in the task result."
        mono
        multiline
        submitLabel="Run"
        onSubmit={(command) => queueTask({ type: 'run_command', command }, 'Command')}
      />
      <RunPolicyDialog open={dialog === 'run-policy'} onClose={close} busy={busy === 'run_policy'} onSubmit={(policy_id) => queueTask({ type: 'run_policy', policy_id }, 'Policy run')} />
      <TextDialog
        open={dialog === 'custom-trigger'}
        onClose={close}
        busy={busy === 'custom_trigger'}
        title="Fire custom trigger"
        label="Event name"
        hint="Policies whose custom trigger matches this name will run (respecting their frequency)."
        placeholder="e.g. install-office"
        submitLabel="Trigger"
        onSubmit={(trigger) => queueTask({ type: 'custom_trigger', trigger }, 'Custom trigger')}
      />
      <TextDialog
        open={dialog === 'notify'}
        onClose={close}
        busy={busy === 'notify'}
        title="Send notification"
        label="Message"
        hint="Shown to the logged-in user as a system notification."
        multiline
        submitLabel="Send"
        onSubmit={(message) => queueTask({ type: 'notify', message }, 'Notification')}
      />
      <TextDialog
        open={dialog === 'logout'}
        onClose={close}
        busy={busy === 'logout_user'}
        title="Log out the current user"
        label="Message (optional)"
        hint="Optionally shown before logging the user out."
        multiline
        optional
        submitLabel="Log out user"
        onSubmit={(message) => queueTask({ type: 'logout_user', message }, 'Log out')}
      />
      <TextDialog
        open={dialog === 'agent-restart'}
        onClose={close}
        busy={busy === 'restart'}
        title="Restart via agent"
        label="Message (optional)"
        hint="Optionally shown to the user before restarting."
        multiline
        optional
        submitLabel="Restart"
        onSubmit={(message) => queueTask({ type: 'restart', message }, 'Restart')}
      />
      <TextDialog
        open={dialog === 'agent-shutdown'}
        onClose={close}
        busy={busy === 'shutdown'}
        title="Shut down via agent"
        label="Message (optional)"
        hint="Optionally shown to the user before shutting down."
        multiline
        optional
        submitLabel="Shut down"
        onSubmit={(message) => queueTask({ type: 'shutdown', message }, 'Shut down')}
      />
    </>
  )
}

function Section({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return (
    <section>
      <h3 className="text-sm font-semibold text-ink">{title}</h3>
      {description && <p className="mt-0.5 text-xs text-ink-muted">{description}</p>}
      <div className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-3">{children}</div>
    </section>
  )
}

function Action({ icon, label, description, onClick, disabled, loading, danger }: { icon: ReactNode; label: string; description: string; onClick: () => void; disabled?: boolean; loading?: boolean; danger?: boolean }) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled || loading}
      className={cn(
        'flex items-start gap-3 rounded-xl border p-3 text-left transition-colors focus-ring disabled:cursor-not-allowed disabled:opacity-50',
        danger ? 'border-critical/30 hover:bg-critical/5' : 'border-line hover:border-line-strong hover:bg-surface-sunken/60',
      )}
    >
      <span className={cn('mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg [&>svg]:size-4', danger ? 'bg-critical/10 text-critical' : 'bg-accent-50 text-accent-700 dark:bg-accent-950/60 dark:text-accent-200')}>
        {loading ? <RefreshCw className="animate-spin" /> : icon}
      </span>
      <span className="min-w-0">
        <span className={cn('block text-sm font-medium', danger ? 'text-critical' : 'text-ink')}>{label}</span>
        <span className="block text-xs text-ink-muted">{description}</span>
      </span>
    </button>
  )
}

function LockDialog({ open, onClose, onSubmit, busy }: { open: boolean; onClose: () => void; onSubmit: (pin: string, message: string, phone: string) => void; busy: boolean }) {
  const [pin, setPin] = useState('')
  const [message, setMessage] = useState('')
  const [phone, setPhone] = useState('')
  const valid = /^\d{6}$/.test(pin)
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Lock computer"
      description="The Mac will lock immediately and require this PIN to unlock. Store the PIN safely; it cannot be recovered later."
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!valid} loading={busy} onClick={() => onSubmit(pin, message, phone)}>
            Lock computer
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Field label="6-digit PIN" required error={pin && !valid ? 'PIN must be exactly 6 digits' : undefined}>
          <Input value={pin} onChange={(e) => setPin(e.target.value.replace(/\D/g, '').slice(0, 6))} inputMode="numeric" mono autoFocus placeholder="123456" />
        </Field>
        <Field label="Message (optional)" hint="Shown on the lock screen.">
          <Input value={message} onChange={(e) => setMessage(e.target.value)} placeholder="This Mac has been locked by IT." />
        </Field>
        <Field label="Phone number (optional)">
          <Input value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+1 555 0100" />
        </Field>
      </div>
    </Modal>
  )
}

function EraseDialog({ open, onClose, onSubmit, busy, name }: { open: boolean; onClose: () => void; onSubmit: (pin: string) => void; busy: boolean; name: string }) {
  const [pin, setPin] = useState('')
  const pinOk = pin === '' || /^\d{6}$/.test(pin)
  return (
    <ConfirmDialog
      open={open}
      onClose={onClose}
      destructive
      title="Erase this computer?"
      description="All content and settings on the Mac will be erased. The device will restart into Setup Assistant. This cannot be undone."
      confirmLabel="Erase computer"
      typeToConfirm={name}
      loading={busy}
      onConfirm={() => {
        if (pinOk) onSubmit(pin)
      }}
    >
      <Field label="Unlock PIN (optional, Intel Macs)" hint="Apple silicon Macs ignore the PIN. Must be 6 digits if provided." error={!pinOk ? 'PIN must be 6 digits' : undefined}>
        <Input value={pin} onChange={(e) => setPin(e.target.value.replace(/\D/g, '').slice(0, 6))} inputMode="numeric" mono />
      </Field>
    </ConfirmDialog>
  )
}

function InstallProfileDialog({ open, onClose, onSubmit, busy }: { open: boolean; onClose: () => void; onSubmit: (id: string) => void; busy: boolean }) {
  const [id, setId] = useState('')
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Install configuration profile"
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!id} loading={busy} onClick={() => onSubmit(id)}>
            Install
          </Button>
        </>
      }
    >
      <Field label="Profile" hint="The profile is installed regardless of its scope. Scope changes may remove it later.">
        <ProfilePicker value={id} onChange={setId} />
      </Field>
    </Modal>
  )
}

function RawCommandDialog({ open, onClose, onSubmit, busy }: { open: boolean; onClose: () => void; onSubmit: (plist: string) => void; busy: boolean }) {
  const [plist, setPlist] = useState('')
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Send raw MDM command"
      description="Paste a complete command plist including the Command dictionary. A CommandUUID is generated if missing."
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!plist.trim()} loading={busy} onClick={() => onSubmit(plist)}>
            Send
          </Button>
        </>
      }
    >
      <Textarea mono rows={14} value={plist} onChange={(e) => setPlist(e.target.value)} placeholder={'<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC ...>\n<plist version="1.0">\n<dict>\n  <key>Command</key>\n  <dict>\n    <key>RequestType</key>\n    <string>DeviceInformation</string>\n  </dict>\n</dict>\n</plist>'} />
    </Modal>
  )
}

export function ScriptParameterInputs({ scriptId, values, onChange }: { scriptId: string; values: string[]; onChange: (v: string[]) => void }) {
  const script = useScript(scriptId || undefined)
  const labels = script.data?.parameter_labels ?? []
  return (
    <FormGrid cols={2}>
      {Array.from({ length: 8 }, (_, i) => (
        <Field key={i} label={`$${i + 4}${labels[i] ? ` — ${labels[i]}` : ''}`}>
          <Input value={values[i] ?? ''} onChange={(e) => onChange(values.map((v, j) => (j === i ? e.target.value : v)))} placeholder={labels[i] || 'Optional'} />
        </Field>
      ))}
    </FormGrid>
  )
}

function RunScriptDialog({ open, onClose, onSubmit, busy }: { open: boolean; onClose: () => void; onSubmit: (scriptId: string, params: string[]) => void; busy: boolean }) {
  const [id, setId] = useState('')
  const [params, setParams] = useState<string[]>(Array(8).fill(''))
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Run script"
      description="The script runs as root. Parameters $1–$3 are reserved (mount point, computer name, username)."
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!id} loading={busy} onClick={() => onSubmit(id, params)}>
            Run script
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Field label="Script" required>
          <ScriptPicker value={id} onChange={setId} />
        </Field>
        {id && <ScriptParameterInputs scriptId={id} values={params} onChange={setParams} />}
      </div>
    </Modal>
  )
}

function RunPolicyDialog({ open, onClose, onSubmit, busy }: { open: boolean; onClose: () => void; onSubmit: (id: string) => void; busy: boolean }) {
  const [id, setId] = useState('')
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Run policy now"
      description="The policy runs immediately on this computer, ignoring its triggers, frequency and scope."
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!id} loading={busy} onClick={() => onSubmit(id)}>
            Run policy
          </Button>
        </>
      }
    >
      <Field label="Policy" required>
        <PolicyPicker value={id} onChange={setId} />
      </Field>
    </Modal>
  )
}

function TextDialog({
  open,
  onClose,
  onSubmit,
  busy,
  title,
  label,
  hint,
  placeholder,
  multiline,
  mono,
  optional,
  submitLabel = 'Submit',
}: {
  open: boolean
  onClose: () => void
  onSubmit: (v: string) => void
  busy: boolean
  title: string
  label: string
  hint?: string
  placeholder?: string
  multiline?: boolean
  mono?: boolean
  optional?: boolean
  submitLabel?: string
}) {
  const [v, setV] = useState('')
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!optional && !v.trim()} loading={busy} onClick={() => onSubmit(v)}>
            {submitLabel}
          </Button>
        </>
      }
    >
      <Field label={label} hint={hint} required={!optional}>
        {multiline ? <Textarea mono={mono} rows={mono ? 8 : 4} value={v} onChange={(e) => setV(e.target.value)} placeholder={placeholder} autoFocus /> : <Input mono={mono} value={v} onChange={(e) => setV(e.target.value)} placeholder={placeholder} autoFocus />}
      </Field>
    </Modal>
  )
}

