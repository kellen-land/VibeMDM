import { useEffect, useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { FileCode } from 'lucide-react'
import { useAuth } from '../auth/AuthContext'
import { Button } from '../components/ui/Button'
import { Field } from '../components/ui/Field'
import { Input } from '../components/ui/Input'
import { Alert } from '../components/ui/Alert'
import { errorMessage } from '../api/client'
import { pageTitle } from '../components/layout/RouteAnnouncer'

export function LoginPage() {
  useEffect(() => {
    document.title = pageTitle('Sign in')
  }, [])
  const { user, loading, login, status } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()
  const loc = useLocation()
  const next = new URLSearchParams(loc.search).get('next')
  const target = next && next.startsWith('/') && !next.startsWith('/login') ? next : '/'

  if (!loading && user) return <Navigate to={target} replace />

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await login(username.trim(), password)
      navigate(target, { replace: true })
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-full items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center gap-3 text-center">
          <div className="flex size-12 items-center justify-center rounded-xl bg-accent-500/15 text-accent-400 ring-1 ring-accent-500/30">
            <FileCode className="size-6" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-ink">{status?.org_name || 'VibeMDM'}</h1>
            <p className="text-sm text-ink-muted">Sign in to the management console</p>
          </div>
        </div>
        <form onSubmit={submit} className="rounded-xl border border-line-strong/50 bg-surface p-6 shadow-xl ring-1 ring-black/20 flex flex-col gap-4">
          {error && <Alert tone="critical">{error}</Alert>}
          <Field label="Username">
            <Input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus required />
          </Field>
          <Field label="Password">
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
          </Field>
          <Button type="submit" variant="primary" size="lg" loading={busy} className="mt-1 w-full">
            Sign in
          </Button>
        </form>
        {status?.version && <p className="mt-4 text-center text-xs text-ink-faint">VibeMDM {status.version}</p>}
      </div>
    </div>
  )
}
