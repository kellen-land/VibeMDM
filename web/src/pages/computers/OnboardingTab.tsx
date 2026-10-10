import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Play } from 'lucide-react'
import { useComputerOnboarding, useOnboardingWorkflows } from '../../api/queries'
import { errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { ComputerDetail } from '../../api/types'
import { CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { RunsTable, RunWorkflowDialog } from '../../components/onboarding/onboarding'

/** This Mac's onboarding runs, newest first, with the same expandable details as the workflow page. */
export function ComputerOnboardingTab({ c }: { c: ComputerDetail }) {
  const { isAdmin } = useAuth()
  const q = useComputerOnboarding(c.id)
  const wfs = useOnboardingWorkflows()
  const [runOpen, setRunOpen] = useState(false)
  const name = c.name || c.hostname || c.serial_number || 'this Mac'
  return (
    <>
      <CardHeader
        title="Onboarding"
        description="White-glove setup runs shown to the first user of this Mac through the VibeMDM agent."
        actions={
          <>
            <Link to="/onboarding" className="text-xs text-accent-700 hover:underline dark:text-accent-300">
              All workflows
            </Link>
            {isAdmin && (
              <Button size="sm" icon={<Play />} onClick={() => setRunOpen(true)} disabled={!c.agent_enrolled} title={c.agent_enrolled ? undefined : 'This Mac does not have the VibeMDM agent'}>
                Run workflow…
              </Button>
            )}
          </>
        }
      />
      <RunsTable
        runs={q.data}
        loading={q.isPending}
        error={q.error ? errorMessage(q.error) : null}
        showComputer={false}
        showWorkflow
        fieldKeys={wfs.data?.field_keys}
        emptyTitle="No onboarding runs for this Mac"
        emptyDescription={c.agent_enrolled ? 'A workflow runs automatically the first time a user is logged in after enrollment, if this Mac is in its scope.' : 'Onboarding needs the VibeMDM agent on the Mac.'}
      />
      <RunWorkflowDialog open={runOpen} onClose={() => setRunOpen(false)} computerId={c.id} computerName={name} />
    </>
  )
}
