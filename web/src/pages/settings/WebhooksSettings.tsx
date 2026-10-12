import { useState, type FormEvent } from 'react'
import { Pencil, Plus, Send, Trash2, Webhook as WebhookIcon } from 'lucide-react'
import { qk, useInvalidate, useWebhookDeliveries, useWebhooks } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import type { Webhook, WebhookRequest } from '../../api/types'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Table } from '../../components/ui/Table'
import { Modal } from '../../components/ui/Modal'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input } from '../../components/ui/Input'
import { Checkbox, Toggle } from '../../components/ui/Checkbox'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { CodeBlock } from '../../components/ui/CopyButton'
import { useToast } from '../../components/ui/Toast'

const samplePayload = `{
  "event": "PolicyCompleted",
  "timestamp": "2026-10-10T14:03:12Z",
  "server": "https://mdm.example.com",
  "payload": { "policy_id": "…", "computer_id": "…", "status": "completed" }
}`

export function WebhooksSettings() {
  const hooks = useWebhooks()
  const deliveries = useWebhookDeliveries()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [create, setCreate] = useState(false)
  const [edit, setEdit] = useState<Webhook | null>(null)
  const [del, setDel] = useState<Webhook | null>(null)
  const [testing, setTesting] = useState(false)
  const events = hooks.data?.available_events ?? []

  const sendTest = async () => {
    setTesting(true)
    try {
      const r = await api.post<{ ok: boolean; note?: string }>('/api/v1/webhooks/test')
      toast.success('Test event queued', r.note)
      setTimeout(() => invalidate(qk.webhookDeliveries), 1500)
    } catch (e) {
      toast.error('Test failed', errorMessage(e))
    } finally {
      setTesting(false)
    }
  }
  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/webhooks/${del.id}`)
      toast.success('Webhook deleted')
      await invalidate(qk.webhooks, qk.webhookDeliveries)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader
          title="Webhooks"
          description="POST a JSON document to your endpoint whenever a subscribed event happens."
          actions={
            <>
              <Button icon={<Send />} loading={testing} onClick={sendTest} disabled={(hooks.data?.total ?? 0) === 0}>
                Send test event
              </Button>
              <Button variant="primary" icon={<Plus />} onClick={() => setCreate(true)}>
                New webhook
              </Button>
            </>
          }
        />
        <Table<Webhook>
          rows={hooks.data?.items}
          loading={hooks.isPending}
          error={hooks.error ? errorMessage(hooks.error) : null}
          rowKey={(w) => w.id}
          onRowClick={(w) => setEdit(w)}
          emptyTitle="No webhooks configured"
          emptyDescription="Add an endpoint to receive enrollment, policy, command and restricted-software events."
          rowClassName={(w) => (w.enabled ? undefined : 'opacity-70')}
          columns={[
            {
              key: 'name',
              header: 'Name',
              render: (w) => (
                <span className="flex items-center gap-2.5">
                  <WebhookIcon className="size-4 text-ink-faint" />
                  <span>
                    <span className="block font-medium text-ink">{w.name}</span>
                    <span className="block text-xs text-ink-muted font-mono truncate max-w-md">{w.url}</span>
                  </span>
                </span>
              ),
            },
            {
              key: 'events',
              header: 'Events',
              render: (w) => (
                <span className="flex flex-wrap gap-1">
                  {w.events.includes('*') ? <Badge tone="accent">All events</Badge> : w.events.slice(0, 3).map((e) => <Badge key={e}>{e}</Badge>)}
                  {!w.events.includes('*') && w.events.length > 3 && <Badge>+{w.events.length - 3}</Badge>}
                </span>
              ),
            },
            { key: 'secret', header: 'Signed', render: (w) => (w.secret ? <Badge tone="good" dot>HMAC</Badge> : <span className="text-xs text-ink-faint">No secret</span>) },
            { key: 'enabled', header: 'Status', render: (w) => <Badge tone={w.enabled ? 'good' : 'neutral'} dot>{w.enabled ? 'Enabled' : 'Disabled'}</Badge> },
            { key: 'updated', header: 'Updated', render: (w) => <RelativeTime value={w.updated_at} className="text-ink-muted" /> },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (w) => (
                <span className="inline-flex gap-1" onClick={(e) => e.stopPropagation()}>
                  <Button size="xs" variant="ghost" icon={<Pencil />} aria-label="Edit" onClick={() => setEdit(w)} />
                  <Button size="xs" variant="ghost" icon={<Trash2 />} aria-label="Delete" onClick={() => setDel(w)} />
                </span>
              ),
            },
          ]}
        />
      </Card>

      <Card>
        <CardHeader title="Recent deliveries" description="The last 200 delivery attempts across all webhooks. Refreshes automatically." />
        <Table
          rows={deliveries.data?.items}
          loading={deliveries.isPending}
          error={deliveries.error ? errorMessage(deliveries.error) : null}
          rowKey={(d) => String(d.id)}
          dense
          emptyTitle="No deliveries yet"
          emptyDescription="Deliveries appear once an enabled webhook receives an event. Try “Send test event”."
          columns={[
            { key: 'hook', header: 'Webhook', render: (d) => <span className="font-medium text-ink">{d.webhook_name}</span> },
            { key: 'event', header: 'Event', render: (d) => <Badge>{d.event}</Badge> },
            {
              key: 'status',
              header: 'Result',
              render: (d) => (
                <Badge tone={d.status >= 200 && d.status < 300 ? 'good' : d.status === 0 ? 'critical' : 'warning'} dot>
                  {d.status === 0 ? 'No response' : `HTTP ${d.status}`}
                </Badge>
              ),
            },
            { key: 'attempts', header: 'Attempts', align: 'right', render: (d) => <span className="tabular-nums">{d.attempts}</span> },
            { key: 'error', header: 'Error', render: (d) => (d.error ? <span className="text-xs text-critical font-mono truncate block max-w-sm" title={d.error}>{d.error}</span> : <span className="text-ink-faint text-xs">—</span>) },
            { key: 'at', header: 'When', render: (d) => <RelativeTime value={d.at} className="text-ink-muted" /> },
          ]}
        />
      </Card>

      <Card>
        <CardHeader title="Payload format" description="Every delivery is an HTTP POST with Content-Type: application/json." />
        <CardBody className="flex flex-col gap-3 text-sm text-ink-muted">
          <CodeBlock text={samplePayload} filename="request body" />
          <p>
            Headers: <span className="font-mono text-ink">X-VibeMDM-Event</span> carries the event name. When a secret is set, <span className="font-mono text-ink">X-VibeMDM-Signature</span> is the base64-encoded HMAC-SHA256 of the raw request body using that secret — verify it before trusting the payload.
          </p>
          <p>
            Subscribe to <span className="font-mono text-ink">*</span> to receive every event, including <span className="font-mono text-ink">Test</span> events sent from this page.
          </p>
        </CardBody>
      </Card>

      {create && <WebhookDialog events={events} onClose={() => setCreate(false)} />}
      {edit && <WebhookDialog events={events} hook={edit} onClose={() => setEdit(null)} />}
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete webhook "${del?.name}"?`} description="Pending deliveries to this endpoint are dropped." confirmLabel="Delete webhook" />
    </div>
  )
}

function WebhookDialog({ hook, events, onClose }: { hook?: Webhook; events: string[]; onClose: () => void }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const [form, setForm] = useState({ name: hook?.name ?? '', url: hook?.url ?? '', secret: '', enabled: hook?.enabled ?? true, events: hook?.events ?? ['*'] })
  const [busy, setBusy] = useState(false)
  const all = form.events.includes('*')
  const toggleEvent = (ev: string, on: boolean) => {
    const next = form.events.filter((e) => e !== '*' && e !== ev)
    setForm({ ...form, events: on ? [...next, ev] : next })
  }
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const body: WebhookRequest = { name: form.name.trim(), url: form.url.trim(), enabled: form.enabled, events: form.events.length ? form.events : ['*'] }
      // Only send the secret key when the user typed one (or on create); omitting it keeps the existing secret.
      if (form.secret || !hook) body.secret = form.secret
      if (hook) await api.put(`/api/v1/webhooks/${hook.id}`, body)
      else await api.post('/api/v1/webhooks', body)
      await invalidate(qk.webhooks)
      toast.success(hook ? 'Webhook saved' : 'Webhook created')
      onClose()
    } catch (err) {
      toast.error('Save failed', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open onClose={onClose} title={hook ? `Edit ${hook.name}` : 'New webhook'} size="lg">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <FormGrid cols={2}>
          <Field label="Name" required>
            <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required autoFocus placeholder="e.g. SIEM forwarder" />
          </Field>
          <Field label="URL" required hint="Must start with http:// or https://.">
            <Input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} required mono placeholder="https://hooks.example.com/vibemdm" />
          </Field>
          <Field label="Secret" hint={hook?.secret ? 'A secret is set. Leave blank to keep it; enter a new value to replace it.' : 'Optional. Used to sign deliveries with HMAC-SHA256.'} className="md:col-span-2">
            <Input type="password" value={form.secret} onChange={(e) => setForm({ ...form, secret: e.target.value })} autoComplete="new-password" placeholder={hook?.secret ? '********' : ''} mono />
          </Field>
        </FormGrid>
        <Toggle checked={form.enabled} onChange={(v) => setForm({ ...form, enabled: v })} label="Enabled" description="Disabled webhooks receive nothing." />
        <Field label="Events">
          <div className="flex flex-col gap-2 rounded-lg border border-line bg-surface-sunken/40 p-3">
            <Checkbox checked={all} onChange={(e) => setForm({ ...form, events: e.target.checked ? ['*'] : [] })} label="All events (*)" description="Includes events added in future releases." />
            <div className="grid gap-2 sm:grid-cols-2 border-t border-line pt-2">
              {events.map((ev) => (
                <Checkbox key={ev} checked={all || form.events.includes(ev)} disabled={all} onChange={(e) => toggleEvent(ev, e.target.checked)} label={<span className="font-mono text-xs">{ev}</span>} />
              ))}
            </div>
          </div>
        </Field>
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={onClose} data-shortcut="cancel">
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={busy} data-shortcut="save">
            {hook ? 'Save' : 'Create webhook'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}
