import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '../api/client'
import type { AgentPackageStatus } from '../api/types'
import { Card, CardBody, CardHeader } from './ui/Card'
import { Badge } from './ui/Badge'
import { Toggle } from './ui/Checkbox'
import { CodeBlock } from './ui/CopyButton'
import { RelativeTime } from './ui/RelativeTime'
import { useToast } from './ui/Toast'

const key = ['agent-package']

/** Settings for installing the agent through MDM right after a Mac enrolls. */
export function AgentAutoInstallCard({ isAdmin, serverURL }: { isAdmin: boolean; serverURL: string }) {
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
  const build = `VIBEMDM_URL=${serverURL || 'https://mdm.example.com'} VIBEMDM_API_TOKEN=<admin API token> ./deploy/macos/build-agent-pkg.sh`

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
          <p className="text-sm text-ink">No package yet. Build it once on any Mac:</p>
        )}
        {isAdmin && (
          <Toggle checked={d.auto_install} onChange={setAuto} disabled={!d.package} label="Install the agent on every newly enrolled Mac" />
        )}
        <details className="rounded-lg border border-line" open={!d.package && isAdmin}>
          <summary className="cursor-pointer select-none px-4 py-2.5 text-sm font-medium text-ink-muted hover:text-ink">{d.package ? 'Build a new package' : 'How to build it'}</summary>
          <div className="flex flex-col gap-2 border-t border-line p-4 text-xs text-ink-muted">
            <p>On any Mac with a copy of this repository, run:</p>
            <CodeBlock text={build} filename="Terminal" />
            <p>It downloads the agent from this server, builds the package, signs it with a certificate from VibeMDM's package-signing CA (the key stays on that Mac) and uploads it. Run it again after upgrading VibeMDM.</p>
            {d.ca_fingerprint && (
              <p>
                Package-signing CA SHA-256: <span className="break-all font-mono">{d.ca_fingerprint}</span>
              </p>
            )}
          </div>
        </details>
      </CardBody>
    </Card>
  )
}
