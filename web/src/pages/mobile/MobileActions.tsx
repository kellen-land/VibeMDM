import { useState, type ReactNode } from 'react'
import { Bluetooth, Eraser, KeyRound, Lock, MapPin, Pencil, Power, RefreshCw, RotateCcw, Siren, Trash2, Volume2 } from 'lucide-react'
import { qk, useInvalidate } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { MobileDeviceDetail } from '../../api/types'
import { CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Modal } from '../../components/ui/Modal'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Field } from '../../components/ui/Field'
import { Input, Select } from '../../components/ui/Input'
import { Checkbox } from '../../components/ui/Checkbox'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'

type Dialog = null | 'lock' | 'rename' | 'settings' | 'lost' | 'clear' | 'restart' | 'shutdown' | 'cellular' | 'erase' | 'unlost'

function Row({ icon, title, description, children, needsSupervision, supervised }: { icon: ReactNode; title: string; description: string; children: ReactNode; needsSupervision?: boolean; supervised: boolean }) {
  const blocked = needsSupervision && !supervised
  return (
    <div className="flex items-center gap-4 py-3">
      <span className="text-ink-faint [&>svg]:size-5">{icon}</span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-ink">{title}</p>
        <p className="text-xs text-ink-muted">{blocked ? 'Needs a supervised device (Apple Configurator).' : description}</p>
      </div>
      <div className={blocked ? 'pointer-events-none opacity-40' : ''}>{children}</div>
    </div>
  )
}

/** Remote commands for an iPhone or iPad, one row each. */
export function MobileActions({ d, onDelete }: { d: MobileDeviceDetail; onDelete: () => void }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [open, setOpen] = useState<Dialog>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [phone, setPhone] = useState('')
  const [footnote, setFootnote] = useState('')
  const [name, setName] = useState(d.name)
  const [confirm, setConfirm] = useState('')
  const [preservePlan, setPreservePlan] = useState(false)
  const [esim, setEsim] = useState('')
  const [setting, setSetting] = useState<{ item: string; enabled: boolean; time_zone: string }>({ item: 'Bluetooth', enabled: true, time_zone: '' })
  const supervised = d.is_supervised === true
  const lost = d.lost_mode_enabled === true

  const close = () => {
    setOpen(null)
    setMessage('')
    setPhone('')
    setFootnote('')
    setConfirm('')
  }
  const send = async (body: Record<string, unknown>, done: string) => {
    setBusy(true)
    try {
      await api.post(`/api/v1/mobile-devices/${d.id}/actions`, body)
      toast.success(done, 'The device runs it at its next MDM check-in, usually within seconds.')
      await invalidate(qk.mobileDevice(d.id))
      close()
    } catch (e) {
      toast.error('Command not sent', errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  if (!d.mdm_enrolled) {
    return (
      <>
        <CardHeader title="Actions" />
        <CardBody>
          <p className="text-sm text-ink-muted">This device is not enrolled in MDM, so no commands can be sent.</p>
        </CardBody>
      </>
    )
  }
  if (!isAdmin) {
    return (
      <>
        <CardHeader title="Actions" />
        <CardBody>
          <p className="text-sm text-ink-muted">Remote commands are available to administrators.</p>
        </CardBody>
      </>
    )
  }

  return (
    <>
      <CardHeader title="Actions" description={supervised ? 'Supervised: every command is available.' : 'Not supervised: some commands need a device prepared with Apple Configurator.'} />
      <CardBody className="divide-y divide-line py-0">
        <Row icon={<Lock />} title="Lock" description="Locks the screen; shows an optional message and phone number." supervised={supervised}>
          <Button size="sm" onClick={() => setOpen('lock')}>
            Lock
          </Button>
        </Row>
        <Row
          icon={<Siren />}
          title="Lost Mode"
          description={lost ? 'On: the device shows your message and can be located.' : 'Locks the device with a message and lets you locate it and play a sound.'}
          needsSupervision
          supervised={supervised}
        >
          {lost ? (
            <span className="flex gap-1.5">
              <Button size="sm" icon={<MapPin />} onClick={() => send({ action: 'locate' }, 'Location requested')}>
                Locate
              </Button>
              <Button size="sm" icon={<Volume2 />} onClick={() => send({ action: 'play_sound' }, 'Sound requested')}>
                Play sound
              </Button>
              <Button size="sm" onClick={() => setOpen('unlost')}>
                Turn off
              </Button>
            </span>
          ) : (
            <Button size="sm" onClick={() => setOpen('lost')}>
              Turn on
            </Button>
          )}
        </Row>
        {lost && d.location && (
          <p className="py-2 pl-9 text-xs text-ink-muted">
            Last location {d.location.latitude.toFixed(5)}, {d.location.longitude.toFixed(5)}
            {d.location.accuracy !== null && ` (±${Math.round(d.location.accuracy)} m)`} · <RelativeTime value={d.location.at} /> ·{' '}
            <a className="text-ink hover:underline" href={`https://www.openstreetmap.org/?mlat=${d.location.latitude}&mlon=${d.location.longitude}#map=17/${d.location.latitude}/${d.location.longitude}`} target="_blank" rel="noreferrer">
              Open map
            </a>
          </p>
        )}
        <Row icon={<Pencil />} title="Rename" description="Sets the device name shown in Settings, Finder and AirDrop." needsSupervision supervised={supervised}>
          <Button size="sm" onClick={() => setOpen('rename')}>
            Rename
          </Button>
        </Row>
        <Row icon={<KeyRound />} title="Clear passcode" description="Removes the passcode so the user can set a new one." supervised={supervised}>
          <Button size="sm" onClick={() => setOpen('clear')}>
            Clear
          </Button>
        </Row>
        <Row icon={<RotateCcw />} title="Restart" description="Restarts the device." needsSupervision supervised={supervised}>
          <Button size="sm" onClick={() => setOpen('restart')}>
            Restart
          </Button>
        </Row>
        <Row icon={<Eraser />} title="Erase" description="Erases all content and settings. The device leaves management." supervised={supervised}>
          <Button size="sm" variant="danger-outline" onClick={() => setOpen('erase')}>
            Erase…
          </Button>
        </Row>
        <details className="py-3">
          <summary className="cursor-pointer select-none text-sm font-medium text-ink-muted hover:text-ink">More actions</summary>
          <div className="mt-1 divide-y divide-line">
            <Row icon={<Bluetooth />} title="Device settings" description="Bluetooth, data roaming, Personal Hotspot or time zone." supervised={supervised}>
              <Button size="sm" onClick={() => setOpen('settings')}>
                Change
              </Button>
            </Row>
            <Row icon={<Power />} title="Shut down" description="Turns the device off." needsSupervision supervised={supervised}>
              <Button size="sm" onClick={() => setOpen('shutdown')}>
                Shut down
              </Button>
            </Row>
            {(d.imei || d.phone_number) && (
              <Row icon={<RefreshCw />} title="Refresh cellular plans" description="Asks the carrier's eSIM server for plans." supervised={supervised}>
                <Button size="sm" onClick={() => setOpen('cellular')}>
                  Refresh
                </Button>
              </Row>
            )}
            <Row icon={<Trash2 />} title="Delete record" description="Removes the record from VibeMDM only; the device stays enrolled." supervised={supervised}>
              <Button size="sm" variant="ghost" onClick={onDelete}>
                Delete
              </Button>
            </Row>
          </div>
        </details>
      </CardBody>

      <ConfirmDialog open={open === 'lock'} onClose={close} onConfirm={() => send({ action: 'lock', message, phone }, 'Lock sent')} loading={busy} title="Lock this device?" confirmLabel="Lock">
        <div className="flex flex-col gap-3">
          <Field label="Message (optional)">
            <Input value={message} onChange={(e) => setMessage(e.target.value)} placeholder="Please return to the IT office" />
          </Field>
          <Field label="Phone number (optional)">
            <Input value={phone} onChange={(e) => setPhone(e.target.value)} />
          </Field>
        </div>
      </ConfirmDialog>
      <ConfirmDialog
        open={open === 'lost'}
        onClose={close}
        onConfirm={() => send({ action: 'enable_lost_mode', message, phone, footnote }, 'Lost Mode turned on')}
        loading={busy}
        confirmDisabled={!message.trim() && !phone.trim()}
        title="Turn on Lost Mode?"
        description="The device locks and shows this message until Lost Mode is turned off from here."
        confirmLabel="Turn on"
      >
        <div className="flex flex-col gap-3">
          <Field label="Message">
            <Input value={message} onChange={(e) => setMessage(e.target.value)} placeholder="This iPad belongs to …" autoFocus />
          </Field>
          <Field label="Phone number">
            <Input value={phone} onChange={(e) => setPhone(e.target.value)} />
          </Field>
          <Field label="Footnote (optional)">
            <Input value={footnote} onChange={(e) => setFootnote(e.target.value)} />
          </Field>
        </div>
      </ConfirmDialog>
      <ConfirmDialog open={open === 'unlost'} onClose={close} onConfirm={() => send({ action: 'disable_lost_mode' }, 'Lost Mode turned off')} loading={busy} title="Turn off Lost Mode?" confirmLabel="Turn off" />
      <ConfirmDialog
        open={open === 'rename'}
        onClose={close}
        onConfirm={() => send({ action: 'settings', settings: [{ item: 'DeviceName', device_name: name }] }, 'Rename sent')}
        loading={busy}
        confirmDisabled={!name.trim()}
        title="Rename this device?"
        confirmLabel="Rename"
      >
        <Field label="New name">
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </Field>
      </ConfirmDialog>
      <Modal
        open={open === 'settings'}
        onClose={close}
        title="Device settings"
        footer={
          <>
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button
              variant="primary"
              loading={busy}
              onClick={() =>
                send({ action: 'settings', settings: [setting.item === 'TimeZone' ? { item: 'TimeZone', time_zone: setting.time_zone } : { item: setting.item, enabled: setting.enabled }] }, 'Setting sent')
              }
            >
              Apply
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          <Field label="Setting">
            <Select
              value={setting.item}
              onChange={(e) => setSetting({ ...setting, item: e.target.value })}
              options={[
                { value: 'Bluetooth', label: 'Bluetooth (supervised)' },
                { value: 'DataRoaming', label: 'Data roaming' },
                { value: 'PersonalHotspot', label: 'Personal Hotspot' },
                { value: 'TimeZone', label: 'Time zone (supervised)' },
              ]}
            />
          </Field>
          {setting.item === 'TimeZone' ? (
            <Field label="Time zone" hint="An IANA name, e.g. America/Chicago.">
              <Input value={setting.time_zone} onChange={(e) => setSetting({ ...setting, time_zone: e.target.value })} />
            </Field>
          ) : (
            <Field label="State">
              <Select value={setting.enabled ? 'on' : 'off'} onChange={(e) => setSetting({ ...setting, enabled: e.target.value === 'on' })} options={[{ value: 'on', label: 'On' }, { value: 'off', label: 'Off' }]} />
            </Field>
          )}
        </div>
      </Modal>
      <ConfirmDialog open={open === 'clear'} onClose={close} onConfirm={() => send({ action: 'clear_passcode' }, 'Clear passcode sent')} loading={busy} title="Clear the passcode?" description="Anyone holding the device can then open it until a new passcode is set." confirmLabel="Clear passcode" destructive />
      <ConfirmDialog open={open === 'restart'} onClose={close} onConfirm={() => send({ action: 'restart' }, 'Restart sent')} loading={busy} title="Restart this device?" confirmLabel="Restart" />
      <ConfirmDialog open={open === 'shutdown'} onClose={close} onConfirm={() => send({ action: 'shutdown' }, 'Shut down sent')} loading={busy} title="Shut down this device?" description="It stays off until someone turns it on." confirmLabel="Shut down" />
      <ConfirmDialog
        open={open === 'cellular'}
        onClose={close}
        onConfirm={() => send({ action: 'refresh_cellular_plans', esim_server_url: esim }, 'Refresh sent')}
        loading={busy}
        confirmDisabled={!esim.startsWith('https://')}
        title="Refresh cellular plans"
        confirmLabel="Refresh"
      >
        <Field label="Carrier eSIM server URL" hint="From your carrier.">
          <Input value={esim} onChange={(e) => setEsim(e.target.value)} placeholder="https://" mono />
        </Field>
      </ConfirmDialog>
      <ConfirmDialog
        open={open === 'erase'}
        onClose={close}
        onConfirm={() => send({ action: 'erase', confirm_name: confirm, preserve_data_plan: preservePlan }, 'Erase sent')}
        loading={busy}
        confirmDisabled={confirm !== d.name && confirm !== d.serial_number}
        destructive
        title={`Erase ${d.name}?`}
        description="All content and settings are deleted and the device leaves management. This cannot be undone. If Activation Lock is on, you will need the bypass code or the user's Apple Account."
        confirmLabel="Erase"
      >
        <div className="flex flex-col gap-3">
          {(d.imei || d.phone_number) && <Checkbox checked={preservePlan} onChange={(e) => setPreservePlan(e.target.checked)} label="Keep the eSIM data plan" />}
          <Field label={`Type ${d.name ? 'the device name' : 'the serial number'} to confirm`}>
            <Input value={confirm} onChange={(e) => setConfirm(e.target.value)} placeholder={d.name || d.serial_number} autoComplete="off" />
          </Field>
          {!supervised && <Badge tone="warning">Not supervised: the user can stop management before the erase arrives.</Badge>}
        </div>
      </ConfirmDialog>
    </>
  )
}
