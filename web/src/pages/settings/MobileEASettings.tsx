import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { useMobileEAs } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { MobileExtensionAttribute } from '../../api/types'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Table } from '../../components/ui/Table'
import { Badge } from '../../components/ui/Badge'
import { Modal } from '../../components/ui/Modal'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Field } from '../../components/ui/Field'
import { Input, Select, Textarea } from '../../components/ui/Input'
import { useToast } from '../../components/ui/Toast'

type Draft = Pick<MobileExtensionAttribute, 'name' | 'description' | 'data_type' | 'input_type'> & { id?: string; choices: string }

const blank: Draft = { name: '', description: '', data_type: 'string', input_type: 'text', choices: '' }

/** Custom inventory fields for iPhones and iPads, filled in by administrators on each device record. */
export function MobileEASettings() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const qc = useQueryClient()
  const q = useMobileEAs()
  const [draft, setDraft] = useState<Draft | null>(null)
  const [del, setDel] = useState<MobileExtensionAttribute | null>(null)
  const [saving, setSaving] = useState(false)

  const save = async () => {
    if (!draft) return
    setSaving(true)
    try {
      const body = {
        name: draft.name,
        description: draft.description,
        data_type: draft.data_type,
        input_type: draft.input_type,
        popup_choices: draft.choices.split('\n').map((c) => c.trim()).filter(Boolean),
      }
      if (draft.id) await api.put(`/api/v1/mobile-extension-attributes/${draft.id}`, body)
      else await api.post('/api/v1/mobile-extension-attributes', body)
      await qc.invalidateQueries({ queryKey: ['mobile-eas'] })
      toast.success('Saved')
      setDraft(null)
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }
  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(`/api/v1/mobile-extension-attributes/${del.id}`)
      await qc.invalidateQueries({ queryKey: ['mobile-eas'] })
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <Card>
      <CardHeader
        title="Mobile device attributes"
        description="Custom fields for iPhones and iPads (extension attributes). iOS has no agent, so they are filled in on each device record and can be used in groups and searches."
        actions={
          isAdmin && (
            <Button size="sm" icon={<Plus />} onClick={() => setDraft({ ...blank })}>
              New attribute
            </Button>
          )
        }
      />
      <Table<MobileExtensionAttribute>
        rows={q.data?.items}
        loading={q.isPending}
        rowKey={(e) => e.id}
        onRowClick={isAdmin ? (e) => setDraft({ id: e.id, name: e.name, description: e.description, data_type: e.data_type, input_type: e.input_type, choices: e.popup_choices.join('\n') }) : undefined}
        emptyTitle="No mobile device attributes yet"
        columns={[
          { key: 'name', header: 'Name', render: (e) => <span className="font-medium text-ink">{e.name}</span> },
          { key: 'type', header: 'Input', render: (e) => <Badge>{e.input_type === 'popup' ? `Pop-up (${e.popup_choices.length})` : 'Text'}</Badge> },
          { key: 'data', header: 'Data type', render: (e) => <span className="text-ink-muted">{e.data_type}</span> },
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
      <Modal
        open={!!draft}
        onClose={() => setDraft(null)}
        title={draft?.id ? 'Edit attribute' : 'New attribute'}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDraft(null)}>
              Cancel
            </Button>
            <Button variant="primary" loading={saving} onClick={save} disabled={!draft?.name.trim()}>
              Save
            </Button>
          </>
        }
      >
        {draft && (
          <CardBody className="flex flex-col gap-4 p-0">
            <Field label="Name" required>
              <Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder="e.g. Cart number" autoFocus />
            </Field>
            <Field label="Description">
              <Input value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} />
            </Field>
            <div className="grid grid-cols-2 gap-4">
              <Field label="Input">
                <Select
                  value={draft.input_type}
                  onChange={(e) => setDraft({ ...draft, input_type: e.target.value as Draft['input_type'] })}
                  options={[
                    { value: 'text', label: 'Text field' },
                    { value: 'popup', label: 'Pop-up menu' },
                  ]}
                />
              </Field>
              <Field label="Data type">
                <Select
                  value={draft.data_type}
                  onChange={(e) => setDraft({ ...draft, data_type: e.target.value as Draft['data_type'] })}
                  options={[
                    { value: 'string', label: 'String' },
                    { value: 'integer', label: 'Integer' },
                    { value: 'date', label: 'Date (YYYY-MM-DD)' },
                  ]}
                />
              </Field>
            </div>
            {draft.input_type === 'popup' && (
              <Field label="Choices" hint="One per line.">
                <Textarea rows={4} value={draft.choices} onChange={(e) => setDraft({ ...draft, choices: e.target.value })} />
              </Field>
            )}
          </CardBody>
        )}
      </Modal>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={doDelete} destructive title={`Delete "${del?.name}"?`} description="Its values on every device are deleted too." confirmLabel="Delete" />
    </Card>
  )
}
