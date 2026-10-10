import type { ReactNode } from 'react'
import type { Computer } from '../api/types'
import { BoolBadge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'
import { formatMB } from './format'
import { latest } from '../pages/DashboardPage'

/** Render one export/display field (keys from GET /advanced-searches/fields) for a computer row. */
export function renderComputerField(key: string, c: Computer): ReactNode {
  const dash = (v: string | null | undefined) => (v ? v : <span className="text-ink-faint">—</span>)
  switch (key) {
    case 'name':
      return <span className="font-medium text-ink">{c.name || c.hostname || c.serial_number || 'Unnamed'}</span>
    case 'serial_number':
    case 'udid':
    case 'mac_address':
    case 'ip_address':
      return <span className="font-mono text-xs">{dash(c[key])}</span>
    case 'model_identifier':
    case 'os_build':
    case 'agent_version':
      return <span className="font-mono text-xs text-ink-muted">{dash(c[key])}</span>
    case 'model_name':
      return <span className="text-ink-muted">{dash(c.model_name || c.model)}</span>
    case 'os_version':
      return <span className="tabular-nums">{dash(c.os_version)}</span>
    case 'memory_mb':
    case 'disk_total_mb':
    case 'disk_free_mb':
      return <span className="tabular-nums">{formatMB(c[key])}</span>
    case 'mdm_enrolled':
      return <BoolBadge value={c.mdm_enrolled} yes="Enrolled" no="No" />
    case 'agent_enrolled':
      return <BoolBadge value={c.agent_enrolled} yes="Installed" no="No" />
    case 'filevault_enabled':
    case 'sip_enabled':
    case 'firewall_enabled':
    case 'gatekeeper_enabled':
      return <BoolBadge value={c[key]} yes="On" no="Off" />
    case 'last_checkin':
      return <RelativeTime value={latest(c.agent_last_checkin, c.mdm_last_checkin)} className="text-ink-muted" />
    case 'last_inventory_at':
      return <RelativeTime value={c.last_inventory_at} className="text-ink-muted" />
    default: {
      const v = (c as unknown as Record<string, unknown>)[key]
      if (v === null || v === undefined || v === '') return <span className="text-ink-faint">—</span>
      if (typeof v === 'boolean') return <BoolBadge value={v} />
      return <span className="text-ink-muted">{String(v)}</span>
    }
  }
}

export const defaultDisplayFields = ['name', 'serial_number', 'model_name', 'os_version', 'last_user', 'building', 'department', 'mdm_enrolled', 'agent_enrolled', 'filevault_enabled', 'last_checkin']
