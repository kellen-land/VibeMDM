import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Plus, ShieldCheck, Upload } from 'lucide-react'
import { useMobileProfiles } from '../../api/queries'
import { errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { MobileConfigProfile } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card } from '../../components/ui/Card'
import { Table } from '../../components/ui/Table'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { SearchInput } from '../../components/ui/SearchInput'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { MobileScopeSummary } from '../../components/pickers/MobileScopeEditor'
import { DeploymentCounts } from '../ProfilesPage'

/** Mobile Device Configuration Profiles: separate from computer profiles, like Jamf Pro. */
export function MobileProfilesPage() {
  const { isAdmin } = useAuth()
  const profiles = useMobileProfiles()
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const needle = q.toLowerCase()
  const items = (profiles.data?.items ?? []).filter((p) => !needle || p.name.toLowerCase().includes(needle) || p.category.toLowerCase().includes(needle))

  return (
    <>
      <PageHeader
        title="Configuration Profiles"
        description="Settings for iPhones and iPads, delivered through MDM to the devices in scope."
        actions={
          isAdmin && (
            <>
              <Link to="/mobile-profiles/new?upload=1">
                <Button variant="ghost" icon={<Upload />}>
                  Upload
                </Button>
              </Link>
              <Link to="/mobile-profiles/new">
                <Button variant="primary" icon={<Plus />}>
                  New profile
                </Button>
              </Link>
            </>
          )
        }
      />
      <Card>
        {(profiles.data?.items.length ?? 0) > 8 && (
          <div className="border-b border-line px-5 py-3">
            <SearchInput value={q} onChange={setQ} placeholder="Filter profiles…" className="w-full sm:w-72" />
          </div>
        )}
        <Table<MobileConfigProfile>
          rows={items}
          loading={profiles.isPending}
          error={profiles.error ? errorMessage(profiles.error) : null}
          rowKey={(p) => p.id}
          onRowClick={(p) => navigate(`/mobile-profiles/${p.id}`)}
          emptyTitle={q ? 'No profiles match' : 'No mobile profiles yet'}
          emptyDescription={q ? undefined : 'Build one from payloads such as Wi-Fi, Restrictions or Home Screen Layout.'}
          emptyAction={
            isAdmin && !q ? (
              <Link to="/mobile-profiles/new">
                <Button variant="primary" size="sm" icon={<Plus />}>
                  New profile
                </Button>
              </Link>
            ) : undefined
          }
          columns={[
            {
              key: 'name',
              header: 'Name',
              render: (p) => (
                <span className="flex items-center gap-2.5">
                  <ShieldCheck className="size-4 text-ink-faint" />
                  <span className="font-medium text-ink">{p.name}</span>
                  {p.payloads === null && <Badge title="Uploaded as a .mobileconfig">Uploaded</Badge>}
                </span>
              ),
            },
            { key: 'category', header: 'Category', render: (p) => (p.category ? <Badge>{p.category}</Badge> : <span className="text-ink-faint">—</span>) },
            { key: 'scope', header: 'Scope', render: (p) => <MobileScopeSummary scope={p.scope} /> },
            { key: 'status', header: 'Deployment', render: (p) => <DeploymentCounts status={p.status} /> },
            { key: 'updated', header: 'Updated', render: (p) => <RelativeTime value={p.updated_at} className="text-ink-muted" /> },
          ]}
        />
      </Card>
    </>
  )
}
