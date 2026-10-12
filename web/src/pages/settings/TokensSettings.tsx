import { useState, type FormEvent } from 'react'
import { KeyRound, Plus, Trash2 } from 'lucide-react'
import { qk, useAPITokens, useInvalidate } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import type { APIToken, Role } from '../../api/types'
import { Card, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Table } from '../../components/ui/Table'
import { Modal } from '../../components/ui/Modal'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { CodeBlock, CopyButton } from '../../components/ui/CopyButton'
import { useToast } from '../../components/ui/Toast'

export function TokensSettings() {
  const tokens = useAPITokens()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [create, setCreate] = useState(false)
  const [created, setCreated] = useState<{ token: string; api_token: APIToken } | null>(null)
  const [del, setDel] = useState<APIToken | null>(null)

  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/api-tokens/${del.id}`)
      toast.success('Token revoked')
      await invalidate(qk.apiTokens)
      setDel(null)
    } catch (e) {
      toast.error('Revoke failed', errorMessage(e))
    }
  }

  return (
    <>
      <Card>
        <CardHeader
          title="API tokens"
          description="Bearer tokens for scripts and integrations. Send them as Authorization: Bearer <token>."
          actions={
            <Button variant="primary" icon={<Plus />} onClick={() => setCreate(true)}>
              New token
            </Button>
          }
        />
        <Table<APIToken>
          rows={tokens.data?.items}
          loading={tokens.isPending}
          error={tokens.error ? errorMessage(tokens.error) : null}
          rowKey={(t) => t.id}
          emptyTitle="No API tokens"
          emptyDescription="Create a token to automate VibeMDM from scripts or CI."
          columns={[
            {
              key: 'name',
              header: 'Name',
              render: (t) => (
                <span className="flex items-center gap-2.5">
                  <KeyRound className="size-4 text-ink-faint" />
                  <span className="font-medium text-ink">{t.name}</span>
                </span>
              ),
            },
            { key: 'prefix', header: 'Prefix', render: (t) => <span className="font-mono text-xs text-ink-muted">{t.token_prefix}…</span> },
            { key: 'role', header: 'Role', render: (t) => <Badge tone={t.role === 'admin' ? 'accent' : 'neutral'}>{t.role}</Badge> },
            { key: 'used', header: 'Last used', render: (t) => <RelativeTime value={t.last_used_at} className="text-ink-muted" /> },
            {
              key: 'expires',
              header: 'Expires',
              render: (t) => {
                const expired = t.expires_at && new Date(t.expires_at) < new Date()
                return t.expires_at ? (
                  <span className={expired ? 'text-critical' : 'text-ink-muted'}>
                    {expired ? 'Expired ' : ''}
                    <RelativeTime value={t.expires_at} />
                  </span>
                ) : (
                  <span className="text-ink-muted">Never</span>
                )
              },
            },
            { key: 'created', header: 'Created', render: (t) => <RelativeTime value={t.created_at} className="text-ink-muted" /> },
            { key: 'actions', header: '', align: 'right', render: (t) => <Button size="xs" variant="ghost" icon={<Trash2 />} aria-label="Revoke" onClick={() => setDel(t)} /> },
          ]}
        />
      </Card>
      <CreateTokenDialog
        open={create}
        onClose={() => setCreate(false)}
        onCreated={(r) => {
          setCreate(false)
          setCreated(r)
        }}
      />
      <Modal open={!!created} onClose={() => setCreated(null)} title="Token created" description="Copy it now. For security the full token is never shown again." size="md" footer={<Button variant="primary" onClick={() => setCreated(null)}>Done</Button>}>
        {created && (
          <div className="flex flex-col gap-4">
            <div className="flex items-center gap-2">
              <Input readOnly mono value={created.token} className="flex-1" />
              <CopyButton text={created.token} />
            </div>
            <CodeBlock text={`curl -H "Authorization: Bearer ${created.token}" ${window.location.origin}/api/v1/computers`} filename="Example" />
            <Alert tone="info">Token “{created.api_token.name}” has the {created.api_token.role} role.</Alert>
          </div>
        )}
      </Modal>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Revoke token "${del?.name}"?`} description="Requests using this token will be rejected immediately." confirmLabel="Revoke token" />
    </>
  )
}

function CreateTokenDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (r: { token: string; api_token: APIToken }) => void }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const [name, setName] = useState('')
  const [role, setRole] = useState<Role>('admin')
  const [days, setDays] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const r = await api.post<{ token: string; api_token: APIToken }>('/api/v1/api-tokens', { name: name.trim(), role, expires_days: days ? Number(days) : null })
      await invalidate(qk.apiTokens)
      setName('')
      setDays('')
      onCreated(r)
    } catch (err) {
      toast.error('Could not create token', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open={open} onClose={onClose} title="New API token" size="sm">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field label="Name" required hint="What will use this token, e.g. “Inventory sync”.">
          <Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        </Field>
        <FormGrid cols={2}>
          <Field label="Role">
            <Select value={role} onChange={(e) => setRole(e.target.value as Role)} options={[{ value: 'admin', label: 'Administrator' }, { value: 'viewer', label: 'Viewer (read-only)' }]} />
          </Field>
          <Field label="Expires in (days)" hint="Blank for no expiry.">
            <Input type="number" min={1} value={days} onChange={(e) => setDays(e.target.value)} placeholder="Never" />
          </Field>
        </FormGrid>
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={onClose} data-shortcut="cancel">
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={busy} data-shortcut="save">
            Create token
          </Button>
        </div>
      </form>
    </Modal>
  )
}
