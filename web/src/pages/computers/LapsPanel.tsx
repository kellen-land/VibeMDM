import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeySquare, MoreHorizontal, PencilLine, RefreshCw } from 'lucide-react'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { ComputerDetail, LAPSEventType, LAPSHistory, LAPSStatus } from '../../api/types'
import { CardBody, CardHeader } from '../../components/ui/Card'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { DescriptionList } from '../../components/ui/DescriptionList'
import { Dropdown } from '../../components/ui/Dropdown'
import { Field } from '../../components/ui/Field'
import { Input } from '../../components/ui/Input'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'
import { LapsRevealButton } from '../../components/LapsReveal'
import { LapsStatusBadge } from '../LapsPage'

const eventLabel: Record<LAPSEventType, string> = {
  CREATED: 'Account created',
  ROTATED: 'Password changed',
  VIEWED: 'Password viewed',
  PENDING: 'Change scheduled',
  SET: 'Password set by an admin',
  ERROR: 'Problem',
  TOKEN_GRANTED: 'Secure Token granted',
}

/** Local admin password (LAPS) for one Mac. Renders nothing unless LAPS covers this Mac. */
export function LapsPanel({ c }: { c: ComputerDetail }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const qc = useQueryClient()
  const key = ['computer', c.id, 'laps']
  const q = useQuery({ queryKey: key, queryFn: () => api.get<LAPSStatus>(`/api/v1/computers/${c.id}/laps`), refetchInterval: 30_000 })
  const [historyOpen, setHistoryOpen] = useState(false)
  const [setOpen, setSetOpen] = useState(false)
  const [newPw, setNewPw] = useState('')
  const [saving, setSaving] = useState(false)
  const [tokenOpen, setTokenOpen] = useState(false)
  const [adminUser, setAdminUser] = useState('')
  const [adminPw, setAdminPw] = useState('')
  const st = q.data
  const refresh = () => qc.invalidateQueries({ queryKey: ['computer', c.id] })
  if (!st || !st.in_scope) return null

  const rotate = async () => {
    try {
      await api.post(`/api/v1/computers/${c.id}/laps/rotate`)
      toast.success('Rotation requested', 'The Mac will set a new password shortly.')
      await refresh()
    } catch (e) {
      toast.error('Could not request a rotation', errorMessage(e))
    }
  }
  const setPassword = async () => {
    setSaving(true)
    try {
      await api.put(`/api/v1/computers/${c.id}/laps/password`, { password: newPw })
      toast.success('Password set', 'The Mac applies it at its next check-in.')
      setSetOpen(false)
      setNewPw('')
      await refresh()
    } catch (e) {
      toast.error('Could not set the password', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  const closeToken = () => {
    setTokenOpen(false)
    setAdminUser('')
    setAdminPw('')
  }
  const grantToken = async () => {
    setSaving(true)
    try {
      await api.post(`/api/v1/computers/${c.id}/laps/secure-token`, { admin_user: adminUser.trim(), admin_password: adminPw })
      toast.success('Secure Token requested', 'The Mac grants it at its next check-in; the credentials are used once and deleted.')
      closeToken()
      await refresh()
    } catch (e) {
      toast.error('Could not request a Secure Token', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <CardHeader
        title="Local admin password"
        className="border-t border-line"
        actions={
          isAdmin && (
            <>
              {st.has_password && <LapsRevealButton computerId={c.id} computerName={c.name} onDone={refresh} />}
              <Dropdown
                align="right"
                trigger={
                  <Button size="xs" variant="ghost" icon={<MoreHorizontal />} aria-label="More LAPS actions">
                    More
                  </Button>
                }
                items={[
                  { label: 'Rotate now', icon: <RefreshCw className="size-4" />, onSelect: rotate },
                  { label: 'Set password…', icon: <PencilLine className="size-4" />, onSelect: () => setSetOpen(true) },
                  ...(st.has_password && st.secure_token !== true ? [{ label: 'Grant Secure Token…', icon: <KeySquare className="size-4" />, onSelect: () => setTokenOpen(true) }] : []),
                ]}
              />
            </>
          )
        }
      />
      <CardBody className="flex flex-col gap-4">
        <DescriptionList
          cols={3}
          items={[
            { label: 'Status', value: <LapsStatusBadge status={st.rotation_status} /> },
            { label: 'Account', value: st.account, mono: true },
            { label: 'Last changed', value: st.escrowed_at ? <RelativeTime value={st.escrowed_at} /> : '' },
            ...(st.rotate_after ? [{ label: 'Changes', value: <RelativeTime value={st.rotate_after} /> }] : []),
            {
              label: 'Secure Token',
              value: st.token_grant_pending ? <Badge tone="info" dot>Grant pending</Badge> : <TriState value={st.secure_token} />,
            },
            ...(st.volume_owner !== null || st.secure_token ? [{ label: 'Volume owner', value: <TriState value={st.volume_owner} unknown="Not applicable" /> }] : []),
            ...(st.password_set_by ? [{ label: 'Waiting to apply', value: `Password set by ${st.password_set_by}` }] : []),
            ...(st.last_error && st.rotation_status === 'failing' ? [{ label: 'Last problem', value: st.last_error }] : []),
          ]}
        />
        <details className="rounded-lg border border-line" onToggle={(e) => setHistoryOpen((e.target as HTMLDetailsElement).open)}>
          <summary className="cursor-pointer select-none px-4 py-2.5 text-sm font-medium text-ink-muted hover:text-ink">History</summary>
          {historyOpen && <LapsHistoryList computerId={c.id} />}
        </details>
      </CardBody>
      <ConfirmDialog
        open={tokenOpen}
        onClose={closeToken}
        onConfirm={grantToken}
        loading={saving}
        confirmDisabled={!adminUser.trim() || !adminPw}
        title="Grant a Secure Token?"
        description="macOS only lets an administrator who already has a Secure Token give one to another account. Enter that administrator's credentials; they are sent to the Mac once, then deleted. On Apple silicon the account also becomes a volume owner when that administrator is one."
        confirmLabel="Grant"
      >
        <div className="flex flex-col gap-4">
          <Field label="Secure Token administrator">
            <Input value={adminUser} onChange={(e) => setAdminUser(e.target.value)} autoComplete="off" autoFocus placeholder="e.g. kellen" />
          </Field>
          <Field label="Password">
            <Input type="password" value={adminPw} onChange={(e) => setAdminPw(e.target.value)} autoComplete="off" />
          </Field>
        </div>
      </ConfirmDialog>
      <ConfirmDialog
        open={setOpen}
        onClose={() => {
          setSetOpen(false)
          setNewPw('')
        }}
        onConfirm={setPassword}
        loading={saving}
        confirmDisabled={newPw.length < 8}
        title="Set the local admin password?"
        description="The Mac applies this password at its next check-in. It still changes automatically after anyone views it."
        confirmLabel="Set password"
      >
        <Field label="New password" hint="8 to 128 characters.">
          <Input type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} autoComplete="new-password" autoFocus />
        </Field>
      </ConfirmDialog>
    </>
  )
}

function LapsHistoryList({ computerId }: { computerId: string }) {
  const q = useQuery({ queryKey: ['computer', computerId, 'laps', 'history'], queryFn: () => api.get<LAPSHistory>(`/api/v1/computers/${computerId}/laps/history`) })
  if (q.isPending) return <p className="border-t border-line px-4 py-3 text-sm text-ink-muted">Loading…</p>
  const events = q.data?.events ?? []
  if (events.length === 0) return <p className="border-t border-line px-4 py-3 text-sm text-ink-muted">Nothing yet.</p>
  return (
    <ul className="divide-y divide-line border-t border-line">
      {events.map((e) => (
        <li key={e.id} className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 px-4 py-2 text-sm">
          <span className="w-32 shrink-0 text-ink-muted">
            <RelativeTime value={e.at} />
          </span>
          <span className="font-medium text-ink">{eventLabel[e.event_type] ?? e.event_type}</span>
          {e.actor && e.actor !== 'agent' && <span className="text-ink-muted">by {e.actor}</span>}
          {e.detail && e.event_type !== 'ROTATED' && e.event_type !== 'PENDING' && <span className="min-w-0 truncate text-ink-faint">{e.detail}</span>}
        </li>
      ))}
    </ul>
  )
}

function TriState({ value, unknown = 'Unknown' }: { value: boolean | null; unknown?: string }) {
  if (value === null) return <span className="text-ink-faint">{unknown}</span>
  return value ? (
    <Badge tone="good" dot>
      Yes
    </Badge>
  ) : (
    <Badge tone="warning" dot>
      No
    </Badge>
  )
}
