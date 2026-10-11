import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { MoreHorizontal, PencilLine, RefreshCw } from 'lucide-react'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { ComputerDetail, LAPSEventType, LAPSHistory, LAPSStatus } from '../../api/types'
import { CardBody, CardHeader } from '../../components/ui/Card'
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
