import { useState } from 'react'
import { ChevronDown, ChevronRight, KeyRound, Plus, ShieldOff, Trash2, UserMinus, UserPlus } from 'lucide-react'
import type { PolicyAccount, PolicyAccountAction } from '../../api/types'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button, IconButton } from '../../components/ui/Button'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Select } from '../../components/ui/Input'
import { Checkbox } from '../../components/ui/Checkbox'
import { Badge } from '../../components/ui/Badge'

const actions: { value: PolicyAccountAction; label: string; icon: React.ReactNode }[] = [
  { value: 'create', label: 'Create Account', icon: <UserPlus /> },
  { value: 'reset_password', label: 'Reset Password', icon: <KeyRound /> },
  { value: 'delete', label: 'Delete Account', icon: <UserMinus /> },
  { value: 'disable_filevault', label: 'Disable Account for FileVault', icon: <ShieldOff /> },
]

const blankAccount = (action: PolicyAccountAction): PolicyAccount => ({
  action,
  username: '',
  full_name: '',
  password: '',
  verify_password: '',
  password_set: false,
  password_hint: '',
  home_directory: '',
  admin: false,
  enable_filevault: false,
  delete_home: false,
})

const needsPassword = (a: PolicyAccount) => a.action === 'create' || a.action === 'reset_password'

/** The first problem the server would also reject, in plain words (or null). */
export function accountProblem(accounts: PolicyAccount[]): string | null {
  for (const a of accounts) {
    const who = a.username || 'An account'
    if (!a.username.trim()) return 'Every local account action needs a username.'
    if (needsPassword(a) && !a.password && !a.password_set) return `${who}: enter a password.`
    if (needsPassword(a) && (a.password ?? '') !== (a.verify_password ?? '')) return `${who}: the passwords do not match.`
  }
  return null
}

function summary(a: PolicyAccount): string {
  const parts: string[] = []
  if (a.action === 'create') {
    if (a.full_name) parts.push(a.full_name)
    parts.push(a.admin ? 'Administrator' : 'Standard user')
    if (a.enable_filevault) parts.push('FileVault')
  }
  if (a.action === 'delete') parts.push(a.delete_home ? 'Deletes home folder' : 'Keeps home folder')
  if (needsPassword(a) && !a.password && !a.password_set) parts.push('No password yet')
  return parts.join(' · ')
}

/** Jamf Pro's Local Accounts payload: create, reset password, delete, disable for FileVault. */
export function LocalAccountsSection({ value, onChange, disabled }: { value: PolicyAccount[]; onChange: (v: PolicyAccount[]) => void; disabled?: boolean }) {
  const [open, setOpen] = useState<number | null>(value.length === 1 ? 0 : null)
  const [adding, setAdding] = useState<PolicyAccountAction>('create')
  const patch = (i: number, p: Partial<PolicyAccount>) => onChange(value.map((a, j) => (j === i ? { ...a, ...p } : a)))
  const add = () => {
    onChange([...value, blankAccount(adding)])
    setOpen(value.length)
  }
  return (
    <Card>
      <CardHeader
        title="Local accounts"
        description="Create, change or remove local accounts on the Mac. Actions run in order, after Installomator apps. Passwords are encrypted on the server and never shown again."
      />
      <CardBody className="flex flex-col gap-3">
        {value.length === 0 && <p className="text-sm text-ink-muted">No local account actions.</p>}
        {value.map((a, i) => {
          const def = actions.find((x) => x.value === a.action)!
          const expanded = open === i
          return (
            <div key={i} className="rounded-xl border border-line">
              <div className="flex items-center gap-3 px-4 py-3">
                <button type="button" className="flex min-w-0 flex-1 items-center gap-3 text-left" onClick={() => setOpen(expanded ? null : i)} aria-expanded={expanded}>
                  <span className="text-ink-faint [&>svg]:size-4">{expanded ? <ChevronDown /> : <ChevronRight />}</span>
                  <span className="text-ink-faint [&>svg]:size-4">{def.icon}</span>
                  <span className="min-w-0">
                    <span className="block truncate font-medium text-ink">
                      {def.label} {a.username && <span className="font-mono text-sm">{a.username}</span>}
                    </span>
                    {summary(a) && <span className="block truncate text-xs text-ink-muted">{summary(a)}</span>}
                  </span>
                </button>
                {needsPassword(a) && a.password_set && !a.password && <Badge>Password stored</Badge>}
                {!disabled && (
                  <IconButton label="Remove action" variant="ghost" onClick={() => onChange(value.filter((_, j) => j !== i))}>
                    <Trash2 />
                  </IconButton>
                )}
              </div>
              {expanded && <AccountForm a={a} onChange={(p) => patch(i, p)} disabled={disabled} />}
            </div>
          )
        })}
        {!disabled && (
          <div className="flex items-end gap-2">
            <Field label="Add action" className="max-w-xs flex-1">
              <Select value={adding} onChange={(e) => setAdding(e.target.value as PolicyAccountAction)}>
                {actions.map((x) => (
                  <option key={x.value} value={x.value}>
                    {x.label}
                  </option>
                ))}
              </Select>
            </Field>
            <Button icon={<Plus />} onClick={add}>
              Add
            </Button>
          </div>
        )}
      </CardBody>
    </Card>
  )
}

function AccountForm({ a, onChange, disabled }: { a: PolicyAccount; onChange: (p: Partial<PolicyAccount>) => void; disabled?: boolean }) {
  const mismatch = needsPassword(a) && (a.password ?? '') !== (a.verify_password ?? '')
  const pwHint = a.password_set ? 'Leave blank to keep the stored password.' : undefined
  return (
    <div className="border-t border-line px-4 py-4">
      <FormGrid cols={2}>
        <Field label="Username" required hint={a.action === 'create' ? 'Short name, e.g. student. Letters, digits, . _ -' : undefined}>
          <Input value={a.username} onChange={(e) => onChange({ username: e.target.value.trim() })} disabled={disabled} mono autoComplete="off" />
        </Field>
        {a.action === 'create' && (
          <Field label="Full name">
            <Input value={a.full_name} onChange={(e) => onChange({ full_name: e.target.value })} disabled={disabled} placeholder={a.username} />
          </Field>
        )}
        {needsPassword(a) && (
          <>
            <Field label={a.action === 'reset_password' ? 'New password' : 'Password'} required={!a.password_set} hint={pwHint ?? "Must meet the Mac's password policy (mSCP/CIS baselines usually require 15+ characters)."}>
              <Input type="password" value={a.password ?? ''} onChange={(e) => onChange({ password: e.target.value })} disabled={disabled} autoComplete="new-password" placeholder={a.password_set ? '••••••••' : undefined} />
            </Field>
            <Field label="Verify password" error={mismatch && a.verify_password ? 'The passwords do not match.' : undefined}>
              <Input type="password" value={a.verify_password ?? ''} onChange={(e) => onChange({ verify_password: e.target.value })} disabled={disabled} autoComplete="new-password" />
            </Field>
          </>
        )}
        {a.action === 'create' && (
          <>
            <Field label="Password hint">
              <Input value={a.password_hint} onChange={(e) => onChange({ password_hint: e.target.value })} disabled={disabled} />
            </Field>
            <Field label="Home directory">
              <Input value={a.home_directory} onChange={(e) => onChange({ home_directory: e.target.value })} disabled={disabled} mono placeholder={`/Users/${a.username || 'username'}`} />
            </Field>
          </>
        )}
      </FormGrid>
      {a.action === 'create' && (
        <div className="mt-4 flex flex-col gap-3">
          <Checkbox checked={a.admin} onChange={(e) => onChange({ admin: e.target.checked })} disabled={disabled} label="Allow user to administer computer" />
          <Checkbox
            checked={a.enable_filevault}
            onChange={(e) => onChange({ enable_filevault: e.target.checked })}
            disabled={disabled}
            label="Enable user for FileVault"
            description="Grants a Secure Token through the LAPS account, which needs one itself. Without it, macOS grants the token at the user's first login when a bootstrap token is escrowed."
          />
        </div>
      )}
      {a.action === 'reset_password' && (
        <p className="mt-3 text-xs text-ink-muted">
          Accounts with a Secure Token are reset through the LAPS account (it needs a Secure Token). The user's login keychain keeps the old password until they update it.
        </p>
      )}
      {a.action === 'delete' && (
        <div className="mt-4 flex flex-col gap-2">
          <Checkbox checked={a.delete_home} onChange={(e) => onChange({ delete_home: e.target.checked })} disabled={disabled} label="Delete home folder" description="Off keeps the home folder on the Mac." />
          <p className="text-xs text-ink-muted">The logged-in user, the LAPS account and the last account with a Secure Token are never deleted.</p>
        </div>
      )}
      {a.action === 'disable_filevault' && <p className="mt-3 text-xs text-ink-muted">The account can no longer unlock FileVault. The last FileVault user is never removed.</p>}
    </div>
  )
}
