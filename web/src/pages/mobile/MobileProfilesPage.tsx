import { Link } from 'react-router-dom'
import { ShieldCheck, Trash2, Upload } from 'lucide-react'
import { qk, useInvalidate, useMobileProfiles } from '../../api/queries'
import { api } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { MobileConfigProfile } from '../../api/types'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { MobileScopeSummary } from '../../components/pickers/MobileScopeEditor'
import { IndexPage } from '../../components/templates/IndexPage'
import { DeploymentCounts } from '../ProfilesPage'

/** Mobile Device Configuration Profiles: separate from computer profiles, like Jamf Pro. */
export function MobileProfilesPage() {
  const { isAdmin } = useAuth()
  const profiles = useMobileProfiles()
  const invalidate = useInvalidate()
  const rows = profiles.data?.items
  const categories = [...new Set((rows ?? []).map((p) => p.category).filter(Boolean))].sort()
  const plural = (n: number) => `${n} ${n === 1 ? 'profile' : 'profiles'}`

  return (
    <IndexPage<MobileConfigProfile>
      title="Configuration profiles"
      description="Settings for iPhones and iPads, delivered through MDM to the devices in scope."
      newTo="/mobile-profiles/new"
      newLabel="New profile"
      secondaryActions={
        isAdmin && (
          <Link to="/mobile-profiles/new?upload=1">
            <Button variant="ghost" icon={<Upload />}>
              Upload
            </Button>
          </Link>
        )
      }
      rows={rows}
      loading={profiles.isPending}
      error={profiles.error}
      rowKey={(p) => p.id}
      rowHref={(p) => `/mobile-profiles/${p.id}`}
      search={{ placeholder: 'Search profiles…', text: (p) => `${p.name} ${p.category}` }}
      filters={[
        ...(categories.length ? [{ key: 'category', label: 'Any category', options: categories.map((c) => ({ value: c, label: c })), match: (p: MobileConfigProfile, v: string) => p.category === v }] : []),
        { key: 'failed', label: 'Any status', options: [{ value: 'failed', label: 'With failures' }], match: (p) => (p.status.failed ?? 0) > 0 },
      ]}
      defaultSort={{ key: 'name', dir: 'asc' }}
      onChanged={() => invalidate(qk.mobileProfiles)}
      bulkActions={[
        {
          label: 'Delete',
          icon: <Trash2 />,
          destructive: true,
          confirm: { title: (n) => `Delete ${plural(n)}?`, description: 'Each profile is removed from every device that has it, then deleted.' },
          run: (p) => api.del(`/api/v1/mobile-device-profiles/${p.id}`),
          done: (n) => `${plural(n)} deleted`,
        },
      ]}
      empty={{ title: 'No profiles yet', description: 'Build one from payloads such as Wi-Fi, Restrictions or Home Screen Layout.' }}
      columns={[
        {
          key: 'name',
          header: 'Name',
          sortValue: (p) => p.name,
          render: (p) => (
            <span className="flex items-center gap-2.5">
              <ShieldCheck className="size-4 text-ink-faint" />
              <span className="font-medium text-ink">{p.name}</span>
              {p.payloads === null && <Badge title="Uploaded as a .mobileconfig">Uploaded</Badge>}
            </span>
          ),
        },
        { key: 'category', header: 'Category', sortValue: (p) => p.category, render: (p) => (p.category ? <Badge>{p.category}</Badge> : <span className="text-ink-faint">—</span>) },
        { key: 'scope', header: 'Scope', render: (p) => <MobileScopeSummary scope={p.scope} /> },
        { key: 'status', header: 'Deployment', sortValue: (p) => p.status.failed ?? 0, render: (p) => <DeploymentCounts status={p.status} /> },
        { key: 'updated', header: 'Updated', sortValue: (p) => p.updated_at, render: (p) => <RelativeTime value={p.updated_at} className="text-ink-muted" /> },
      ]}
    />
  )
}
