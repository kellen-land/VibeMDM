import { Link, useNavigate } from 'react-router-dom'
import { CircleOff, HandHelping, Plus } from 'lucide-react'
import { useOnboardingWorkflows } from '../../api/queries'
import { errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { OnboardingWorkflow } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Table } from '../../components/ui/Table'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ScopeSummary } from '../../components/pickers/ScopeEditor'

export function OnboardingPage() {
  const { isAdmin } = useAuth()
  const navigate = useNavigate()
  const q = useOnboardingWorkflows()
  const rows = [...(q.data?.items ?? [])].sort((a, b) => Number(b.enabled) - Number(a.enabled) || a.priority - b.priority || a.name.localeCompare(b.name))

  return (
    <>
      <PageHeader
        title="Onboarding"
        description="White-glove setup that greets the first user of a newly enrolled Mac, collects details, names the Mac and installs what they need."
        actions={
          isAdmin &&
          rows.length > 0 && (
            <Link to="/onboarding/new" data-shortcut="new">
              <Button variant="primary" icon={<Plus />}>
                New workflow
              </Button>
            </Link>
          )
        }
      />
      {q.isPending && <LoadingBlock />}
      {q.error && <Alert tone="critical">{errorMessage(q.error)}</Alert>}
      {q.data && rows.length === 0 && <EmptyIntro isAdmin={isAdmin} />}
      {rows.length > 0 && (
        <Card>
          <Table<OnboardingWorkflow>
            rows={rows}
            rowKey={(w) => w.id}
            onRowClick={(w) => navigate(`/onboarding/${w.id}`)}
            rowClassName={(w) => (!w.enabled ? 'opacity-70' : undefined)}
            columns={[
              {
                key: 'name',
                header: 'Workflow',
                render: (w) => (
                  <span className="block">
                    <Link to={`/onboarding/${w.id}`} onClick={(e) => e.stopPropagation()} className="block font-medium text-ink hover:underline">
                      {w.name}
                    </Link>
                    <span className="block truncate text-xs text-ink-muted">{w.config.title || 'Setting up your Mac'}</span>
                  </span>
                ),
              },
              {
                key: 'state',
                header: 'State',
                render: (w) =>
                  w.enabled ? (
                    <Badge tone="good" dot>
                      Enabled
                    </Badge>
                  ) : (
                    <Badge icon={<CircleOff />}>Disabled</Badge>
                  ),
              },
              {
                key: 'priority',
                header: 'Priority',
                align: 'right',
                render: (w) => <span className="tabular-nums text-ink-muted">{w.priority}</span>,
              },
              { key: 'scope', header: 'Scope', render: (w) => <ScopeSummary scope={w.scope} /> },
              {
                key: 'fields',
                header: 'Fields',
                align: 'right',
                render: (w) => <span className="tabular-nums text-ink-muted">{w.config.fields?.length ?? 0}</span>,
              },
              {
                key: 'steps',
                header: 'Steps',
                align: 'right',
                render: (w) => <span className="tabular-nums text-ink-muted">{w.config.steps?.length ?? 0}</span>,
              },
              { key: 'updated', header: 'Updated', align: 'right', render: (w) => <RelativeTime value={w.updated_at} className="text-ink-muted" /> },
            ]}
          />
        </Card>
      )}
      {rows.length > 1 && <p className="mt-3 text-xs text-ink-muted">When several enabled workflows include the same Mac, only the one with the lowest priority number runs, once per Mac.</p>}
    </>
  )
}

function EmptyIntro({ isAdmin }: { isAdmin: boolean }) {
  return (
    <Card className="px-6 py-12">
      <div className="mx-auto flex max-w-xl flex-col items-center text-center">
        <div className="mb-4 flex size-11 items-center justify-center rounded-full bg-accent-500/12 text-accent-400 ring-1 ring-accent-500/25">
          <HandHelping className="size-5" aria-hidden="true" />
        </div>
        <h2 className="text-base font-semibold text-ink">Welcome the first user of every new Mac</h2>
        <p className="mt-2 text-[13px] leading-relaxed text-ink-muted">
          An onboarding workflow runs through the VibeMDM agent the first time someone is logged in after enrollment. It shows a swiftDialog welcome screen, then a live progress list while
          the Mac is set up.
        </p>
        <ul className="mt-4 space-y-1.5 text-left text-[13px] text-ink-muted">
          <li>
            <span className="text-ink">Collect details</span> such as the asset tag, department or assigned user, straight into inventory.
          </li>
          <li>
            <span className="text-ink">Name the Mac</span> from a template like <span className="font-mono">MAC-{'{serial}'}</span>.
          </li>
          <li>
            <span className="text-ink">Install and configure</span> with apps, policies and scripts, then restart, log out or just finish.
          </li>
        </ul>
        <div className="mt-6">
          {isAdmin ? (
            <Link to="/onboarding/new" data-shortcut="new">
              <Button variant="primary" icon={<Plus />}>
                Create a workflow
              </Button>
            </Link>
          ) : (
            <p className="text-xs text-ink-faint">An administrator can create the first workflow.</p>
          )}
        </div>
      </div>
    </Card>
  )
}
