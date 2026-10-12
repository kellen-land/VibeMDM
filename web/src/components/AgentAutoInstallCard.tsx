import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Hammer } from 'lucide-react'
import { api, errorMessage } from '../api/client'
import type { AgentPackageStatus } from '../api/types'
import { Card, CardBody, CardHeader } from './ui/Card'
import { Badge } from './ui/Badge'
import { Toggle } from './ui/Checkbox'
import { CodeBlock } from './ui/CopyButton'
import { RelativeTime } from './ui/RelativeTime'
import { useToast } from './ui/Toast'
import { Button } from './ui/Button'
import { Spinner } from './ui/Spinner'
import { ComputerSelect } from './pickers/ComputerSelect'

const key = ['agent-package']

/** Settings for installing the agent through MDM right after a Mac enrolls. */
export function AgentAutoInstallCard({ isAdmin, serverURL }: { isAdmin: boolean; serverURL: string }) {
  const toast = useToast()
  const qc = useQueryClient()
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<AgentPackageStatus>('/api/v1/agent-package'),
    refetchInterval: (query) => (['trusting', 'building', 'cleaning'].includes(query.state.data?.build?.status ?? '') ? 3000 : false),
  })
  const [buildMac, setBuildMac] = useState('')
  const [starting, setStarting] = useState(false)
  const d = q.data
  if (!d) return null
  const b = d.build
  const running = !!b && ['trusting', 'building', 'cleaning'].includes(b.status)

  const startBuild = async () => {
    setStarting(true)
    try {
      await api.post('/api/v1/agent-package/build', { computer_id: buildMac })
      await qc.invalidateQueries({ queryKey: key })
      toast.success('Build started', 'This takes a minute or two. The Mac trusts the signing CA only while it builds.')
    } catch (e) {
      toast.error('Build not started', errorMessage(e))
    } finally {
      setStarting(false)
    }
  }

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
          <p className="text-sm text-ink">No package yet. Build it on one of your Macs below.</p>
        )}
        {isAdmin && (
          <Toggle checked={d.auto_install} onChange={setAuto} disabled={!d.package} label="Install the agent on every newly enrolled Mac" />
        )}
        {isAdmin && (
          <div className="flex flex-col gap-2 rounded-lg border border-line p-4">
            <p className="text-sm font-medium text-ink">{d.package ? 'Build a new package' : 'Build the package'}</p>
            <p className="text-xs text-ink-muted">
              Choose an enrolled Mac with the agent. VibeMDM makes it trust the package-signing CA for the build, builds and signs the package there, uploads it and removes the trust again. Build again after upgrading VibeMDM.
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <ComputerSelect value={buildMac} onChange={setBuildMac} placeholder="Choose a Mac…" className="w-72" />
              <Button icon={<Hammer />} onClick={startBuild} loading={starting} disabled={!buildMac || running}>
                Build on this Mac
              </Button>
            </div>
            {b && b.status !== 'idle' && (
              <div className="mt-1 text-xs">
                <p className={b.status === 'failed' ? 'text-critical' : b.status === 'completed' ? 'text-good' : 'text-ink-muted'}>
                  {running && <Spinner className="mr-1.5 inline size-3" />}
                  {b.message}
                  {b.finished_at && <> · <RelativeTime value={b.finished_at} /></>}
                </p>
                {b.output && b.status === 'failed' && <CodeBlock text={b.output} filename="Build output" className="mt-2" maxHeight="14rem" />}
              </div>
            )}
          </div>
        )}
        <details className="rounded-lg border border-line">
          <summary className="cursor-pointer select-none px-4 py-2.5 text-sm font-medium text-ink-muted hover:text-ink">Build it in Terminal instead</summary>
          <div className="flex flex-col gap-2 border-t border-line p-4 text-xs text-ink-muted">
            <p>On a Mac that trusts the package-signing CA (for example through a profile), with a copy of this repository, run:</p>
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
