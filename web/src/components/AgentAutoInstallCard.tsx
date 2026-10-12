import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '../api/client'
import type { AgentPackageStatus } from '../api/types'
import { Card, CardBody, CardHeader } from './ui/Card'
import { Badge } from './ui/Badge'
import { Toggle } from './ui/Checkbox'
import { RelativeTime } from './ui/RelativeTime'
import { useToast } from './ui/Toast'

const key = ['agent-package']

/** Settings for installing the agent through MDM right after a Mac enrolls. */
export function AgentAutoInstallCard({ isAdmin }: { isAdmin: boolean }) {
  const toast = useToast()
  const qc = useQueryClient()
  const q = useQuery({ queryKey: key, queryFn: () => api.get<AgentPackageStatus>('/api/v1/agent-package') })
  const d = q.data
  if (!d) return null

  const setAuto = async (v: boolean) => {
    try {
      qc.setQueryData(key, await api.put<AgentPackageStatus>('/api/v1/agent-package/settings', { auto_install: v }))
      toast.success(v ? 'Automatic agent install is on' : 'Automatic agent install is off')
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    }
  }

  return (
    <Card>
      <CardHeader
        title="Install the agent automatically"
        actions={
          d.auto_install && d.package ? (
            <Badge tone="good" dot>
              On
            </Badge>
          ) : (
            <Badge>Off</Badge>
          )
        }
      />
      <CardBody className="flex flex-col gap-4">
        <p className="text-sm text-ink-muted">After a Mac enrolls in MDM, VibeMDM sends it the signed agent package and the agent sets itself up. No Terminal step.</p>
        {d.package ? (
          <p className="text-sm text-ink">
            Package <span className="font-mono">{d.package.version}</span>
            {d.package.signed_by && <> · signed by {d.package.signed_by}</>} · uploaded <RelativeTime value={d.package.uploaded_at} />
          </p>
        ) : (
          <p className="text-sm text-ink-muted">No agent package has been built yet.</p>
        )}
        {isAdmin && <Toggle checked={d.auto_install} onChange={setAuto} disabled={!d.package} label="Install the agent on every newly enrolled Mac" />}
      </CardBody>
    </Card>
  )
}
