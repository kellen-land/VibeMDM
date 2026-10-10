import { useNavigate, useParams } from 'react-router-dom'
import { useAuth } from '../../auth/AuthContext'
import { PageHeader } from '../../components/ui/PageHeader'
import { Tabs } from '../../components/ui/Tabs'
import { Alert } from '../../components/ui/Alert'
import { GeneralSettings } from './GeneralSettings'
import { APNsSettings } from './APNsSettings'
import { SCEPSettings } from './SCEPSettings'
import { UsersSettings } from './UsersSettings'
import { TokensSettings } from './TokensSettings'
import { AuditLog } from './AuditLog'
import { NetworkOrgSettings } from './NetworkOrgSettings'
import { WebhooksSettings } from './WebhooksSettings'

const tabs = ['general', 'network', 'apns', 'scep', 'webhooks', 'users', 'tokens', 'audit'] as const
type Tab = (typeof tabs)[number]

export function SettingsPage() {
  const { tab } = useParams()
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const active: Tab = (tabs as readonly string[]).includes(tab ?? '') ? (tab as Tab) : 'general'
  const adminOnly = active === 'users' || active === 'tokens' || active === 'audit' || active === 'webhooks'
  return (
    <>
      <PageHeader title="Settings" description="Server configuration, network organization, certificates, webhooks, console users and audit history." />
      <Tabs
        className="mb-5"
        value={active}
        onChange={(t) => navigate(`/settings${t === 'general' ? '' : `/${t}`}`, { replace: true })}
        items={[
          { key: 'general', label: 'General' },
          { key: 'network', label: 'Network Organization' },
          { key: 'apns', label: 'APNs Push Certificate' },
          { key: 'scep', label: 'SCEP CA' },
          { key: 'webhooks', label: 'Webhooks', disabled: !isAdmin },
          { key: 'users', label: 'Users', disabled: !isAdmin },
          { key: 'tokens', label: 'API Tokens', disabled: !isAdmin },
          { key: 'audit', label: 'Audit Log', disabled: !isAdmin },
        ]}
      />
      {adminOnly && !isAdmin ? (
        <Alert tone="info">This section is only available to administrators.</Alert>
      ) : (
        <>
          {active === 'general' && <GeneralSettings />}
          {active === 'network' && <NetworkOrgSettings />}
          {active === 'apns' && <APNsSettings />}
          {active === 'webhooks' && <WebhooksSettings />}
          {active === 'scep' && <SCEPSettings />}
          {active === 'users' && <UsersSettings />}
          {active === 'tokens' && <TokensSettings />}
          {active === 'audit' && <AuditLog />}
        </>
      )}
    </>
  )
}
