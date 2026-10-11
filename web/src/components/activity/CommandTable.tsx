import { useState } from 'react'
import { Link } from 'react-router-dom'
import { SearchCode, XCircle } from 'lucide-react'
import type { MDMCommand } from '../../api/types'
import { Table } from '../ui/Table'
import { Badge, StatusBadge } from '../ui/Badge'
import { RelativeTime } from '../ui/RelativeTime'
import { Button } from '../ui/Button'
import { CommandInspectorModal, commandStatus } from './CommandInspector'

/** Kept for existing imports: the command detail modal is now the inspector. */
export { CommandInspectorModal as CommandDetailModal } from './CommandInspector'

export function CommandTable({
  commands,
  loading,
  showComputer = true,
  onCancel,
  canCancel,
  footer,
}: {
  commands: MDMCommand[] | undefined
  loading?: boolean
  showComputer?: boolean
  onCancel?: (c: MDMCommand) => void
  canCancel?: boolean
  footer?: React.ReactNode
}) {
  const [selected, setSelected] = useState<string | null>(null)
  return (
    <>
      <Table<MDMCommand>
        rows={commands}
        loading={loading}
        rowKey={(c) => c.command_uuid}
        onRowClick={(c) => setSelected(c.command_uuid)}
        emptyTitle="No MDM commands"
        footer={footer}
        columns={[
          { key: 'type', header: 'Command', render: (c) => <span className="font-medium text-ink">{c.request_type}</span> },
          ...(showComputer
            ? [
                {
                  key: 'computer',
                  header: 'Device',
                  render: (c: MDMCommand) => (
                    <Link to={c.mobile_device_id ? `/mobile-devices/${c.mobile_device_id}` : `/computers/${c.computer_id}`} onClick={(e) => e.stopPropagation()} className="text-ink hover:underline">
                      {c.computer_name || c.enrollment_id.split(':')[0]?.slice(0, 8) || '—'}
                    </Link>
                  ),
                },
              ]
            : []),
          { key: 'kind', header: 'Kind', render: (c) => <Badge>{c.kind || 'management'}</Badge> },
          { key: 'status', header: 'Status', render: (c) => <StatusBadge status={commandStatus(c)} /> },
          { key: 'by', header: 'Sent by', render: (c) => <span className="text-ink-muted">{c.created_by || 'system'}</span> },
          { key: 'created', header: 'Created', render: (c) => <RelativeTime value={c.created_at} className="text-ink-muted" /> },
          { key: 'updated', header: 'Result', render: (c) => <RelativeTime value={c.result_updated_at} emptyLabel="—" className="text-ink-muted" /> },
          {
            key: 'actions',
            header: '',
            align: 'right' as const,
            render: (c: MDMCommand) => (
              <span className="inline-flex items-center justify-end gap-1">
                <Link
                  to={`/commands/${c.command_uuid}`}
                  onClick={(e) => e.stopPropagation()}
                  className="inline-flex h-7 items-center gap-1 rounded-md px-2 text-xs text-ink-muted hover:bg-line/40 hover:text-ink focus-ring [&>svg]:size-3.5"
                >
                  <SearchCode /> Inspect
                </Link>
                {onCancel && c.active && (c.status === 'Pending' || c.status === 'NotNow') && canCancel && (
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<XCircle />}
                    onClick={(e) => {
                      e.stopPropagation()
                      onCancel(c)
                    }}
                  >
                    Cancel
                  </Button>
                )}
              </span>
            ),
          },
        ]}
      />
      <CommandInspectorModal uuid={selected} onClose={() => setSelected(null)} />
    </>
  )
}

