import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { KeyRound, LogOut } from 'lucide-react'
import { useAuth, meKey } from '../auth/AuthContext'
import { api, errorMessage } from '../api/client'
import { Button } from '../components/ui/Button'
import { Field } from '../components/ui/Field'
import { Input } from '../components/ui/Input'
import { Alert } from '../components/ui/Alert'
import { useToast } from '../components/ui/Toast'

/** Shown instead of the app shell while the signed-in account must choose a new password. */
export function ForcePasswordChangePage() {
  const { user, status, logout } = useAuth()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const toast = useToast()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const problem = next && next.length < 12 ? 'Use at least 12 characters.' : next && next === current ? 'Choose a password different from the current one.' : confirm && next !== confirm ? 'The passwords don’t match.' : null

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (problem || next !== confirm) {
      setError(problem ?? 'The passwords don’t match.')
      return
    }
    setBusy(true)
    setError(null)
    try {
      await api.post('/api/v1/auth/password', { current_password: current, new_password: next })
      // The server re-issues the session cookie; refetch me, then let any earlier 403s retry.
      await qc.invalidateQueries({ queryKey: meKey })
      await qc.invalidateQueries({ predicate: (q) => q.queryKey[0] !== meKey[0] })
      toast.success('Password changed')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const signOut = async () => {
    try {
      await logout()
    } finally {
      navigate('/login', { replace: true })
    }
  }

  return (
    <div className="flex min-h-full items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center gap-3 text-center">
          <div className="flex size-12 items-center justify-center rounded-xl bg-accent-500/15 text-accent-400 ring-1 ring-accent-500/30">
            <KeyRound className="size-6" aria-hidden="true" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-ink">Choose a new password</h1>
            <p className="text-sm text-ink-muted">
              {user?.username ? (
                <>
                  <span className="font-medium text-ink">{user.username}</span> must set a new password before continuing to {status?.org_name || 'VibeMDM'}.
                </>
              ) : (
                'Set a new password before continuing.'
              )}
            </p>
          </div>
        </div>
        <form onSubmit={submit} className="flex flex-col gap-4 rounded-xl border border-line-strong/50 bg-surface p-6 shadow-xl ring-1 ring-black/20">
          {error && <Alert tone="critical">{error}</Alert>}
          <Field label="Current password">
            <Input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" autoFocus required />
          </Field>
          <Field label="New password" hint="At least 12 characters, different from the current one.">
            <Input type="password" value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" minLength={12} required />
          </Field>
          <Field label="Confirm new password" error={confirm && next !== confirm ? 'The passwords don’t match.' : undefined}>
            <Input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" required />
          </Field>
          <Button type="submit" variant="primary" size="lg" loading={busy} disabled={!current || !next || !confirm || !!problem} className="mt-1 w-full">
            Set password and continue
          </Button>
        </form>
        <div className="mt-4 flex justify-center">
          <Button variant="ghost" size="sm" icon={<LogOut />} onClick={signOut}>
            Sign out
          </Button>
        </div>
      </div>
    </div>
  )
}
