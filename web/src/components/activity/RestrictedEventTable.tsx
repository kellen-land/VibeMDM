import { Link } from 'react-router-dom'
import type { RestrictedEvent } from '../../api/types'
import { Table } from '../ui/Table'
import { Badge } from '../ui/Badge'
import { RelativeTime } from '../ui/RelativeTime'

export function RestrictedEventTable({ events, loading, error, showComputer = true, footer }: { events: RestrictedEvent[] | undefined; loading?: boolean; error?: string | null; showComputer?: boolean; footer?: React.ReactNode }) {
  return (
    <Table<RestrictedEvent>
      rows={events}
      loading={loading}
      error={error}
      rowKey={(e) => String(e.id)}
      dense
      emptyTitle="No restricted software events"
      emptyDescription="Events are reported by the agent when a restricted process is seen on a scoped computer."
      footer={footer}
      columns={[
        {
          key: 'rule',
          header: 'Rule',
          render: (e) => (
            <Link to={`/restricted-software/${e.restricted_id}`} onClick={(ev) => ev.stopPropagation()} className="font-medium text-ink hover:underline">
              {e.rule_name || 'Deleted rule'}
            </Link>
          ),
        },
        ...(showComputer
          ? [
              {
                key: 'computer',
                header: 'Computer',
                render: (e: RestrictedEvent) => (
                  <Link to={`/computers/${e.computer_id}`} onClick={(ev) => ev.stopPropagation()} className="text-ink hover:underline">
                    {e.computer_name || 'Deleted computer'}
                  </Link>
                ),
              },
            ]
          : []),
        { key: 'process', header: 'Process', render: (e) => <span className="font-mono text-xs">{e.process}</span> },
        { key: 'user', header: 'User', render: (e) => <span className="text-ink-muted">{e.username || '—'}</span> },
        { key: 'killed', header: 'Action', render: (e) => (e.killed ? <Badge tone="critical" dot>Killed</Badge> : <Badge tone="warning" dot>Reported</Badge>) },
        { key: 'at', header: 'When', render: (e) => <RelativeTime value={e.at} className="text-ink-muted" /> },
      ]}
    />
  )
}
