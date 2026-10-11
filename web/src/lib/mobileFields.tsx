import type { ReactNode } from 'react'
import type { MobileDevice } from '../api/types'
import { BoolBadge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'

export const defaultMobileDisplayFields = ['name', 'device_type', 'model_name', 'os_version', 'serial_number', 'assigned_user', 'is_supervised', 'last_checkin']

/** Render one display field (keys from GET /mobile-device-searches/fields) for a mobile device row. */
export function renderMobileField(key: string, d: MobileDevice): ReactNode {
  const dash = (v: string | null | undefined) => (v ? v : <span className="text-ink-faint">—</span>)
  const gb = (n: number | null) => (n === null ? dash('') : <span className="tabular-nums">{Math.round(n * 10) / 10} GB</span>)
  switch (key) {
    case 'name':
      return <span className="font-medium text-ink">{d.name || d.serial_number || 'Unnamed'}</span>
    case 'serial_number':
    case 'udid':
    case 'imei':
    case 'model':
      return <span className="font-mono text-xs">{dash(d[key])}</span>
    case 'os_version':
      return <span className="tabular-nums">{dash(d.os_version)}</span>
    case 'capacity_gb':
    case 'available_gb':
      return gb(d[key])
    case 'is_supervised':
    case 'passcode_present':
      return <BoolBadge value={d[key]} />
    case 'lost_mode_enabled':
      return <BoolBadge value={d.lost_mode_enabled} yes="On" no="Off" />
    case 'last_checkin':
      return <RelativeTime value={d.mdm_last_checkin} className="text-ink-muted" />
    case 'last_inventory':
      return <RelativeTime value={d.last_inventory_at} className="text-ink-muted" />
    case 'device_type':
    case 'model_name':
    case 'phone_number':
    case 'carrier':
    case 'assigned_user':
    case 'department':
    case 'building':
    case 'asset_tag':
      return <span className="text-ink-muted">{dash(d[key])}</span>
  }
  return dash('')
}
