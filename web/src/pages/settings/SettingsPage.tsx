import { useNavigate, useParams } from 'react-router-dom'
import { useAuth } from '../../auth/AuthContext'
import { PageHeader } from '../../components/ui/PageHeader'
import { Alert } from '../../components/ui/Alert'
import { SectionNav } from '../../components/ui/SectionNav'
import { GeneralSettings, type GeneralSection } from './GeneralSettings'
import { APNsSettings } from './APNsSettings'
import { SCEPSettings } from './SCEPSettings'
import { UsersSettings } from './UsersSettings'
import { TokensSettings } from './TokensSettings'
import { AuditLog } from './AuditLog'
import { NetworkOrgSettings } from './NetworkOrgSettings'
import { WebhooksSettings } from './WebhooksSettings'
import { MobileEASettings } from './MobileEASettings'

interface Item {
  key: string
  label: string
  adminOnly?: boolean
}

/** Settings are grouped like Jamf Pro's: one short list on the left, one panel at a time. */
const groups: { heading: string; items: Item[] }[] = [
  {
    heading: 'Server',
    items: [
      { key: 'general', label: 'General' },
      { key: 'logging', label: 'Logging' },
      { key: 'audit', label: 'Audit log', adminOnly: true },
    ],
  },
  {
    heading: 'Enrollment',
    items: [
      { key: 'enrollment', label: 'Enrollment' },
      { key: 'apns', label: 'Push certificate' },
      { key: 'scep', label: 'SCEP CA' },
      { key: 'signing', label: 'Profile signing' },
    ],
  },
  {
    heading: 'Organization',
    items: [
      { key: 'network', label: 'Buildings and networks' },
      { key: 'mobile-attributes', label: 'Mobile device attributes' },
      { key: 'self-service', label: 'Self Service' },
    ],
  },
  {
    heading: 'Access',
    items: [
      { key: 'users', label: 'Users', adminOnly: true },
      { key: 'tokens', label: 'API tokens', adminOnly: true },
      { key: 'remote-shell', label: 'Remote shell', adminOnly: true },
    ],
  },
  {
    heading: 'Integrations',
    items: [
      { key: 'webhooks', label: 'Webhooks', adminOnly: true },
      { key: 'ai', label: 'AI assistants (MCP)' },
    ],
  },
]
const all = groups.flatMap((g) => g.items)
const generalSections: readonly string[] = ['general', 'enrollment', 'remote-shell', 'ai', 'logging', 'signing', 'self-service']

export function SettingsPage() {
  const { tab } = useParams()
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const active = all.find((i) => i.key === tab) ?? all[0]
  const go = (key: string) => navigate(`/settings${key === 'general' ? '' : `/${key}`}`, { replace: true })
  const visible = (i: Item) => isAdmin || !i.adminOnly

  return (
    <>
      <PageHeader title="Settings" />
      <div className="flex flex-col gap-6 lg:flex-row">
        <SectionNav label="Settings sections" groups={groups.map((g) => ({ heading: g.heading, items: g.items.filter(visible) }))} value={active.key} onChange={go} />
        <div className="min-w-0 flex-1">
          {active.adminOnly && !isAdmin ? (
            <Alert tone="info">This section is only available to administrators.</Alert>
          ) : generalSections.includes(active.key) ? (
            <GeneralSettings key={active.key} section={active.key as GeneralSection} />
          ) : (
            <>
              {active.key === 'network' && <NetworkOrgSettings />}
              {active.key === 'apns' && <APNsSettings />}
              {active.key === 'webhooks' && <WebhooksSettings />}
              {active.key === 'scep' && <SCEPSettings />}
              {active.key === 'users' && <UsersSettings />}
              {active.key === 'tokens' && <TokensSettings />}
              {active.key === 'audit' && <AuditLog />}
              {active.key === 'mobile-attributes' && <MobileEASettings />}
            </>
          )}
        </div>
      </div>
    </>
  )
}
