import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Plus, Puzzle, Trash2 } from 'lucide-react'
import { qk, useExtensionAttributes, useInvalidate } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { ExtensionAttribute } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card } from '../components/ui/Card'
import { Table } from '../components/ui/Table'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { useToast } from '../components/ui/Toast'

export function ExtensionAttributesPage() {
  const { isAdmin } = useAuth()
  const eas = useExtensionAttributes()
  const navigate = useNavigate()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [del, setDel] = useState<ExtensionAttribute | null>(null)

  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/extension-attributes/${del.id}`)
      toast.success('Extension attribute deleted')
      await invalidate(qk.eas, qk.criteriaFields)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader
        title="Extension Attributes"
        description="Custom inventory fields collected by a script on each computer or entered by hand."
        actions={
          isAdmin && (
            <Link to="/extension-attributes/new" data-shortcut="new">
              <Button variant="primary" icon={<Plus />}>
                New attribute
              </Button>
            </Link>
          )
        }
      />
      <Card>
        <Table<ExtensionAttribute>
          rows={eas.data?.items}
          loading={eas.isPending}
          error={eas.error ? errorMessage(eas.error) : null}
          rowKey={(e) => e.id}
          onRowClick={(e) => navigate(`/extension-attributes/${e.id}`)}
          emptyTitle="No extension attributes yet"
          emptyDescription="Create one to collect custom data, then use it in smart group criteria."
          emptyAction={
            isAdmin ? (
              <Link to="/extension-attributes/new" data-shortcut="new">
                <Button variant="primary" size="sm" icon={<Plus />}>
                  New attribute
                </Button>
              </Link>
            ) : undefined
          }
          rowClassName={(e) => (e.enabled ? undefined : 'opacity-70')}
          columns={[
            {
              key: 'name',
              header: 'Name',
              render: (e) => (
                <span className="flex items-center gap-2.5">
                  <Puzzle className="size-4 text-ink-faint" />
                  <span className="min-w-0">
                    <span className="block font-medium text-ink">{e.name}</span>
                    {e.description && <span className="block text-xs text-ink-muted truncate max-w-md">{e.description}</span>}
                  </span>
                </span>
              ),
            },
            { key: 'data', header: 'Data type', render: (e) => <Badge>{e.data_type}</Badge> },
            { key: 'input', header: 'Input', render: (e) => <span className="text-ink-muted">{e.input_type === 'script' ? 'Script' : 'Text field'}</span> },
            { key: 'enabled', header: 'Status', render: (e) => <Badge tone={e.enabled ? 'good' : 'neutral'} dot>{e.enabled ? 'Enabled' : 'Disabled'}</Badge> },
            { key: 'updated', header: 'Updated', render: (e) => <RelativeTime value={e.updated_at} className="text-ink-muted" /> },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (e) =>
                isAdmin ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    icon={<Trash2 />}
                    aria-label="Delete attribute"
                    onClick={(ev) => {
                      ev.stopPropagation()
                      setDel(e)
                    }}
                  />
                ) : null,
            },
          ]}
        />
      </Card>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete "${del?.name}"?`} description="Collected values are deleted and smart groups using this attribute will stop matching." confirmLabel="Delete attribute" />
    </>
  )
}
