import { useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { ComputerDetail, LAPSStatus } from '../../api/types'
import { CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { DescriptionList } from '../../components/ui/DescriptionList'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'
import { LapsRevealButton } from '../../components/LapsReveal'
import { LapsStatusBadge } from '../LapsPage'

/** Local admin password (LAPS) for one Mac. Renders nothing unless LAPS covers this Mac. */
export function LapsPanel({ c }: { c: ComputerDetail }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const qc = useQueryClient()
  const key = ['computer', c.id, 'laps']
  const q = useQuery({ queryKey: key, queryFn: () => api.get<LAPSStatus>(`/api/v1/computers/${c.id}/laps`), refetchInterval: 30_000 })
  const st = q.data
  if (!st || !st.in_scope) return null

  const rotate = async () => {
    try {
      await api.post(`/api/v1/computers/${c.id}/laps/rotate`)
      toast.success('Rotation requested', 'The Mac will set a new password shortly.')
      await qc.invalidateQueries({ queryKey: key })
    } catch (e) {
      toast.error('Could not request a rotation', errorMessage(e))
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
              {st.has_password && <LapsRevealButton computerId={c.id} computerName={c.name} onDone={() => qc.invalidateQueries({ queryKey: key })} />}
              <Button size="xs" variant="ghost" icon={<RefreshCw />} onClick={rotate}>
                Rotate now
              </Button>
            </>
          )
        }
      />
      <CardBody>
        <DescriptionList
          cols={3}
          items={[
            { label: 'Status', value: <LapsStatusBadge status={st.rotation_status} /> },
            { label: 'Account', value: st.account, mono: true },
            { label: 'Last changed', value: st.escrowed_at ? <RelativeTime value={st.escrowed_at} /> : '' },
            ...(st.last_viewed_at ? [{ label: 'Last viewed', value: <span>{st.last_viewed_by} · <RelativeTime value={st.last_viewed_at} /></span> }] : []),
            ...(st.last_error ? [{ label: 'Last problem', value: st.last_error }] : []),
          ]}
        />
      </CardBody>
    </>
  )
}
