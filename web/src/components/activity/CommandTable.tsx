import { useState } from 'react'
import { Link } from 'react-router-dom'
import { XCircle } from 'lucide-react'
import type { MDMCommand } from '../../api/types'
import { useCommand } from '../../api/queries'
import { Table } from '../ui/Table'
import { Badge, StatusBadge } from '../ui/Badge'
import { RelativeTime } from '../ui/RelativeTime'
import { Button } from '../ui/Button'
import { Modal } from '../ui/Modal'
import { LoadingBlock } from '../ui/Spinner'
import { DescriptionList } from '../ui/DescriptionList'
import { CodeBlock } from '../ui/CopyButton'
import { absoluteTime } from '../../lib/format'

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
                  header: 'Computer',
                  render: (c: MDMCommand) => (
                    <Link to={`/computers/${c.computer_id}`} onClick={(e) => e.stopPropagation()} className="text-ink hover:underline">
                      {c.computer_name || c.enrollment_id.split(':')[0]?.slice(0, 8) || '—'}
                    </Link>
                  ),
                },
              ]
            : []),
          { key: 'kind', header: 'Kind', render: (c) => <Badge>{c.kind || 'management'}</Badge> },
          { key: 'status', header: 'Status', render: (c) => <StatusBadge status={c.active || c.status !== 'Pending' ? c.status : 'Cancelled'} /> },
          { key: 'by', header: 'Sent by', render: (c) => <span className="text-ink-muted">{c.created_by || 'system'}</span> },
          { key: 'created', header: 'Created', render: (c) => <RelativeTime value={c.created_at} className="text-ink-muted" /> },
          { key: 'updated', header: 'Result', render: (c) => <RelativeTime value={c.result_updated_at} emptyLabel="—" className="text-ink-muted" /> },
          ...(onCancel
            ? [
                {
                  key: 'actions',
                  header: '',
                  align: 'right' as const,
                  render: (c: MDMCommand) =>
                    c.active && (c.status === 'Pending' || c.status === 'NotNow') && canCancel ? (
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
                    ) : null,
                },
              ]
            : []),
        ]}
      />
      <CommandDetailModal uuid={selected} onClose={() => setSelected(null)} />
    </>
  )
}

export function CommandDetailModal({ uuid, onClose }: { uuid: string | null; onClose: () => void }) {
  const q = useCommand(uuid ?? undefined)
  const c = q.data
  return (
    <Modal open={!!uuid} onClose={onClose} title={c ? c.request_type : 'MDM command'} description={uuid ?? undefined} size="xl">
      {q.isPending && <LoadingBlock />}
      {c && (
        <div className="flex flex-col gap-5">
          <DescriptionList
            cols={3}
            items={[
              { label: 'Status', value: <StatusBadge status={c.active || c.status !== 'Pending' ? c.status : 'Cancelled'} /> },
              { label: 'Kind', value: c.kind },
              { label: 'Sent by', value: c.created_by || 'system' },
              { label: 'Computer', value: c.computer_id ? <Link to={`/computers/${c.computer_id}`} className="hover:underline">{c.computer_name || c.computer_id}</Link> : '—' },
              { label: 'Created', value: absoluteTime(c.created_at) },
              { label: 'Result received', value: c.result_updated_at ? absoluteTime(c.result_updated_at) : '—' },
            ]}
          />
          <div>
            <p className="mb-1.5 text-xs font-medium text-ink-muted">Command (plist)</p>
            <CodeBlock text={c.command || '(not available)'} maxHeight="20rem" />
          </div>
          <div>
            <p className="mb-1.5 text-xs font-medium text-ink-muted">Result (plist)</p>
            <CodeBlock text={c.result || '(no result yet)'} maxHeight="20rem" />
          </div>
        </div>
      )}
    </Modal>
  )
}
