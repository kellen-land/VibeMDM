import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { qk, useExtensionAttribute, useInvalidate } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { EADataType, EAInputType, ExtensionAttribute } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Field, FormGrid } from '../components/ui/Field'
import { Input, Select, Textarea } from '../components/ui/Input'
import { Toggle } from '../components/ui/Checkbox'
import { Alert } from '../components/ui/Alert'
import { LoadingBlock } from '../components/ui/Spinner'
import { useToast } from '../components/ui/Toast'
import { CodeEditor } from '../components/CodeEditor'
import { useCloneSource, useEditMode } from '../components/editor/useEditMode'
import { ObjectBar } from '../components/editor/ObjectBar'
import { DetailsLayout, SummaryCard } from '../components/templates/DetailsLayout'
import { Badge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'

const defaultScript = `#!/bin/bash
# Print the value between <result></result> tags.
value=$(sysctl -n hw.model)
echo "<result>$value</result>"
`

export function ExtensionAttributeEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const ea = useExtensionAttribute(isNew ? undefined : id)
  const clone = useCloneSource<ExtensionAttribute>()
  const init = isNew ? clone : undefined

  const [name, setName] = useState(init?.name ?? '')
  const [description, setDescription] = useState(init?.description ?? '')
  const [dataType, setDataType] = useState<EADataType>(init?.data_type ?? 'string')
  const [inputType, setInputType] = useState<EAInputType>(init?.input_type ?? 'script')
  const [script, setScript] = useState(init?.script || defaultScript)
  const [enabled, setEnabled] = useState(init?.enabled ?? true)
  const [saving, setSaving] = useState(false)

  const load = useCallback(() => {
    if (ea.data) {
      setName(ea.data.name)
      setDescription(ea.data.description)
      setDataType(ea.data.data_type)
      setInputType(ea.data.input_type)
      setScript(ea.data.script || defaultScript)
      setEnabled(ea.data.enabled)
    }
  }, [ea.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: { name, description, dataType, inputType, script, enabled }, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])

  const save = async () => {
    if (!name.trim()) {
      toast.error('Name is required')
      return false
    }
    setSaving(true)
    try {
      const body: Partial<ExtensionAttribute> = { name: name.trim(), description, data_type: dataType, input_type: inputType, script: inputType === 'script' ? script : '', enabled }
      const out = isNew ? await api.post<ExtensionAttribute>('/api/v1/extension-attributes', body) : await api.put<ExtensionAttribute>(`/api/v1/extension-attributes/${id}`, body)
      await invalidate(qk.eas, qk.ea(out.id), qk.criteriaFields)
      toast.success(isNew ? 'Extension attribute created' : 'Extension attribute saved')
      if (isNew) navigate(`/extension-attributes/${out.id}`, { replace: true })
      return true
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
      return false
    } finally {
      setSaving(false)
    }
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/extension-attributes/${id}`)
      toast.success('Extension attribute deleted')
      await invalidate(qk.eas, qk.criteriaFields)
      navigate('/extension-attributes')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (!isNew && ea.isPending) return <LoadingBlock />
  if (!isNew && ea.error) return <Alert tone="critical">{errorMessage(ea.error)}</Alert>
  const ro = ed.readOnly
  const saved = ea.data

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Extension attributes', to: '/extension-attributes' }, { label: isNew ? 'New attribute' : ea.data?.name ?? '' }]}
        title={isNew ? 'New extension attribute' : ea.data?.name}
      />
      <DetailsLayout
        aside={
          !isNew &&
          saved && (
            <SummaryCard
              rows={[
                { label: 'Status', value: <Badge tone={saved.enabled ? 'good' : 'neutral'} dot>{saved.enabled ? 'Enabled' : 'Disabled'}</Badge> },
                { label: 'Data type', value: saved.data_type },
                { label: 'Input', value: saved.input_type === 'script' ? 'Script' : 'Text field' },
                { label: 'Last changed', value: saved.updated_at ? <RelativeTime value={saved.updated_at} /> : '' },
              ]}
            />
          )
        }
      >
        <div className="flex flex-col gap-6">
          <Card>
            <CardHeader title="General" />
            <CardBody className="flex flex-col gap-5">
              <FormGrid cols={2}>
                <Field label="Name" required>
                  <Input value={name} onChange={(e) => setName(e.target.value)} disabled={ro} />
                </Field>
                <Field label="Description">
                  <Input value={description} onChange={(e) => setDescription(e.target.value)} disabled={ro} />
                </Field>
                <Field label="Data type" hint="Controls which operators are offered in smart group criteria.">
                  <Select value={dataType} onChange={(e) => setDataType(e.target.value as EADataType)} disabled={ro} options={[{ value: 'string', label: 'String' }, { value: 'integer', label: 'Integer' }, { value: 'date', label: 'Date (YYYY-MM-DD hh:mm:ss)' }]} />
                </Field>
                <Field label="Input type" hint="Script: collected by the agent during inventory. Text field: entered manually on each computer record.">
                  <Select value={inputType} onChange={(e) => setInputType(e.target.value as EAInputType)} disabled={ro} options={[{ value: 'script', label: 'Script' }, { value: 'text', label: 'Text field' }]} />
                </Field>
              </FormGrid>
              <Toggle checked={enabled} onChange={setEnabled} disabled={ro} label="Enabled" description="Disabled attributes are not collected and are hidden from criteria." />
            </CardBody>
          </Card>
          {inputType === 'script' && (
            <Card>
              <CardHeader title="Script" description="Runs as root during each inventory update. Output the value wrapped in <result></result> tags; anything outside the tags is ignored." />
              <CardBody className="flex flex-col gap-3">
                <Alert tone="info">
                  Example: <code className="font-mono">echo "&lt;result&gt;$(sw_vers -productVersion)&lt;/result&gt;"</code>. For integer attributes print only digits; for dates use <code className="font-mono">YYYY-MM-DD hh:mm:ss</code>.
                </Alert>
                <CodeEditor language="shell" value={script} onChange={setScript} height="400px" readOnly={ro} />
              </CardBody>
            </Card>
          )}
          {inputType === 'text' && (
            <Card>
              <CardBody>
                <Textarea readOnly rows={2} value="Values for text attributes are entered on each computer's Extension Attributes tab." className="bg-surface-sunken" />
              </CardBody>
            </Card>
          )}
        </div>
      </DetailsLayout>
      <ObjectBar
        ed={ed}
        isNew={isNew}
        listPath="/extension-attributes"
        noun="extension attribute"
        name={ea.data?.name}
        saving={saving}
        onSave={save}
        onDelete={doDelete}
        deleteDescription="Collected values are deleted and smart groups using this attribute will stop matching."
        onClone={() => navigate('/extension-attributes/new', { state: { clone: { id: '', name: `Copy of ${name}`, description, data_type: dataType, input_type: inputType, script, enabled } } })}
        historyId={id}
      />
    </>
  )
}
