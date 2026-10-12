import { useState, type FormEvent } from 'react'
import { KeyRound, Pencil, Plus, Trash2 } from 'lucide-react'
import { qk, useInvalidate, useUsers } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { AdminUser, Role } from '../../api/types'
import { Card, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Table } from '../../components/ui/Table'
import { Modal } from '../../components/ui/Modal'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select } from '../../components/ui/Input'
import { Toggle } from '../../components/ui/Checkbox'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'

export function UsersSettings() {
  const { user: me } = useAuth()
  const users = useUsers()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [create, setCreate] = useState(false)
  const [edit, setEdit] = useState<AdminUser | null>(null)
  const [pw, setPw] = useState<AdminUser | null>(null)
  const [del, setDel] = useState<AdminUser | null>(null)

  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/users/${del.id}`)
      toast.success('User deleted')
      await invalidate(qk.users)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <Card>
        <CardHeader
          title="Console users"
          description="Administrators can change anything; viewers have read-only access."
          actions={
            <Button variant="primary" icon={<Plus />} onClick={() => setCreate(true)}>
              New user
            </Button>
          }
        />
        <Table<AdminUser>
          rows={users.data?.items}
          loading={users.isPending}
          error={users.error ? errorMessage(users.error) : null}
          rowKey={(u) => u.id}
          emptyTitle="No users"
          rowClassName={(u) => (u.enabled ? undefined : 'opacity-70')}
          columns={[
            {
              key: 'user',
              header: 'User',
              render: (u) => (
                <span>
                  <span className="block font-medium text-ink">
                    {u.display_name || u.username} {u.id === me?.id && <Badge tone="accent" className="ml-1">You</Badge>}
                  </span>
                  <span className="block text-xs text-ink-muted">
                    {u.username}
                    {u.email ? ` · ${u.email}` : ''}
                  </span>
                </span>
              ),
            },
            { key: 'role', header: 'Role', render: (u) => <Badge tone={u.role === 'admin' ? 'accent' : 'neutral'}>{u.role}</Badge> },
            { key: 'enabled', header: 'Status', render: (u) => <Badge tone={u.enabled ? 'good' : 'neutral'} dot>{u.enabled ? 'Enabled' : 'Disabled'}</Badge> },
            { key: 'last', header: 'Last login', render: (u) => <RelativeTime value={u.last_login_at} className="text-ink-muted" /> },
            { key: 'created', header: 'Created', render: (u) => <RelativeTime value={u.created_at} className="text-ink-muted" /> },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (u) => (
                <span className="inline-flex gap-1">
                  <Button size="xs" variant="ghost" icon={<Pencil />} aria-label="Edit" onClick={() => setEdit(u)} />
                  <Button size="xs" variant="ghost" icon={<KeyRound />} aria-label="Reset password" onClick={() => setPw(u)} />
                  {u.id !== me?.id && <Button size="xs" variant="ghost" icon={<Trash2 />} aria-label="Delete" onClick={() => setDel(u)} />}
                </span>
              ),
            },
          ]}
        />
      </Card>
      <UserDialog open={create} onClose={() => setCreate(false)} />
      {edit && <UserDialog open user={edit} onClose={() => setEdit(null)} isSelf={edit.id === me?.id} />}
      {pw && <PasswordDialog user={pw} onClose={() => setPw(null)} />}
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete user "${del?.username}"?`} description="Their sessions end immediately. Audit history is kept." confirmLabel="Delete user" />
    </>
  )
}

function UserDialog({ open, onClose, user, isSelf }: { open: boolean; onClose: () => void; user?: AdminUser; isSelf?: boolean }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const [form, setForm] = useState({ username: user?.username ?? '', display_name: user?.display_name ?? '', email: user?.email ?? '', role: (user?.role ?? 'admin') as Role, enabled: user?.enabled ?? true, password: '' })
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      if (user) {
        await api.put(`/api/v1/users/${user.id}`, { display_name: form.display_name, email: form.email, role: form.role, enabled: form.enabled })
        toast.success('User updated')
      } else {
        await api.post('/api/v1/users', { username: form.username.trim(), display_name: form.display_name, email: form.email, role: form.role, password: form.password })
        toast.success('User created')
      }
      await invalidate(qk.users)
      onClose()
    } catch (err) {
      toast.error('Save failed', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open={open} onClose={onClose} title={user ? `Edit ${user.username}` : 'New user'} size="md">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <FormGrid cols={2}>
          <Field label="Username" required>
            <Input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} disabled={!!user} required autoComplete="off" />
          </Field>
          <Field label="Display name">
            <Input value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
          </Field>
          <Field label="Email">
            <Input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
          </Field>
          <Field label="Role" hint={isSelf ? 'You cannot change your own role.' : undefined}>
            <Select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as Role })} disabled={isSelf} options={[{ value: 'admin', label: 'Administrator' }, { value: 'viewer', label: 'Viewer (read-only)' }]} />
          </Field>
          {!user && (
            <Field label="Password" required hint="At least 8 characters." className="md:col-span-2">
              <Input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} minLength={8} required autoComplete="new-password" />
            </Field>
          )}
        </FormGrid>
        {user && <Toggle checked={form.enabled} onChange={(v) => setForm({ ...form, enabled: v })} disabled={isSelf} label="Enabled" description="Disabled users cannot sign in." />}
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={onClose} data-shortcut="cancel">
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={busy} data-shortcut="save">
            {user ? 'Save' : 'Create user'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}

function PasswordDialog({ user, onClose }: { user: AdminUser; onClose: () => void }) {
  const toast = useToast()
  const [pw, setPw] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (pw !== confirm) {
      toast.error('Passwords do not match')
      return
    }
    setBusy(true)
    try {
      await api.post(`/api/v1/users/${user.id}/password`, { password: pw })
      toast.success(`Password reset for ${user.username}`)
      onClose()
    } catch (err) {
      toast.error('Reset failed', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open onClose={onClose} title={`Reset password for ${user.username}`} size="sm">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field label="New password" hint="At least 8 characters.">
          <Input type="password" value={pw} onChange={(e) => setPw(e.target.value)} minLength={8} required autoFocus autoComplete="new-password" />
        </Field>
        <Field label="Confirm password">
          <Input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required autoComplete="new-password" />
        </Field>
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={onClose} data-shortcut="cancel">
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={busy} data-shortcut="save">
            Reset password
          </Button>
        </div>
      </form>
    </Modal>
  )
}
