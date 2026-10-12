import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Play } from 'lucide-react'
import { qk, useInvalidate, useScript } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { RunResult, Script, ScriptPriority } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Field, FormGrid } from '../components/ui/Field'
import { Input, Select, Textarea } from '../components/ui/Input'
import { Alert } from '../components/ui/Alert'
import { LoadingBlock } from '../components/ui/Spinner'
import { Modal } from '../components/ui/Modal'
import { RelativeTime } from '../components/ui/RelativeTime'
import { useToast } from '../components/ui/Toast'
import { CodeEditor } from '../components/CodeEditor'
import { TargetPicker, emptyTargets, type Targets } from '../components/pickers/TargetPicker'
import { useCloneSource, useEditMode } from '../components/editor/useEditMode'
import { ObjectBar } from '../components/editor/ObjectBar'

const defaultScript = `#!/bin/bash
# $1 = mount point, $2 = computer name, $3 = username
# $4 - $11 = policy parameters

echo "Hello from VibeMDM"
exit 0
`

export function ScriptEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const script = useScript(isNew ? undefined : id)
  const clone = useCloneSource<Script>()
  const init = isNew ? clone : undefined

  const [name, setName] = useState(init?.name ?? '')
  const [category, setCategory] = useState(init?.category ?? '')
  const [info, setInfo] = useState(init?.info ?? '')
  const [notes, setNotes] = useState(init?.notes ?? '')
  const [priority, setPriority] = useState<ScriptPriority>(init?.priority ?? 'after')
  const [labels, setLabels] = useState<string[]>(() => {
    const l = [...(init?.parameter_labels ?? [])]
    while (l.length < 8) l.push('')
    return l.slice(0, 8)
  })
  const [contents, setContents] = useState(init?.contents ?? defaultScript)
  const [osReq, setOsReq] = useState(init?.os_requirements ?? '')
  const [saving, setSaving] = useState(false)
  const [runOpen, setRunOpen] = useState(false)

  const load = useCallback(() => {
    if (script.data) {
      setName(script.data.name)
      setCategory(script.data.category)
      setInfo(script.data.info)
      setNotes(script.data.notes)
      setPriority(script.data.priority)
      const l = [...(script.data.parameter_labels ?? [])]
      while (l.length < 8) l.push('')
      setLabels(l.slice(0, 8))
      setContents(script.data.contents)
      setOsReq(script.data.os_requirements)
    }
  }, [script.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: { name, category, info, notes, priority, labels, contents, osReq }, reset: load })
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
      const body: Partial<Script> = { name: name.trim(), category, info, notes, priority, parameter_labels: labels, contents, os_requirements: osReq }
      const out = isNew ? await api.post<Script>('/api/v1/scripts', body) : await api.put<Script>(`/api/v1/scripts/${id}`, body)
      await invalidate(qk.scripts, qk.script(out.id), qk.dashboard)
      toast.success(isNew ? 'Script created' : 'Script saved')
      if (isNew) navigate(`/scripts/${out.id}`, { replace: true })
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
      await api.del(`/api/v1/scripts/${id}`)
      toast.success('Script deleted')
      await invalidate(qk.scripts, qk.dashboard)
      navigate('/scripts')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (!isNew && script.isPending) return <LoadingBlock />
  if (!isNew && script.error) return <Alert tone="critical">{errorMessage(script.error)}</Alert>
  const ro = ed.readOnly

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Scripts', to: '/scripts' }, { label: isNew ? 'New script' : script.data?.name ?? '' }]}
        title={isNew ? 'New script' : script.data?.name}
        meta={
          !isNew && script.data ? (
            <span className="text-xs text-ink-muted">
              Updated <RelativeTime value={script.data.updated_at} />
            </span>
          ) : undefined
        }
      />
      <div className="grid gap-6 xl:grid-cols-[1fr_360px]">
        <div className="flex flex-col gap-6">
          <Card>
            <CardHeader title="General" />
            <CardBody>
              <FormGrid cols={2}>
                <Field label="Name" required>
                  <Input value={name} onChange={(e) => setName(e.target.value)} disabled={ro} />
                </Field>
                <Field label="Category">
                  <Input value={category} onChange={(e) => setCategory(e.target.value)} disabled={ro} placeholder="e.g. Maintenance" />
                </Field>
                <Field label="Information" hint="Shown to admins when selecting the script.">
                  <Textarea value={info} onChange={(e) => setInfo(e.target.value)} rows={2} disabled={ro} />
                </Field>
                <Field label="Notes">
                  <Textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} disabled={ro} />
                </Field>
                <Field label="Default priority" hint="Whether the script runs before or after packages in a policy.">
                  <Select value={priority} onChange={(e) => setPriority(e.target.value as ScriptPriority)} disabled={ro} options={[{ value: 'before', label: 'Before packages' }, { value: 'after', label: 'After packages' }]} />
                </Field>
                <Field label="OS requirements" hint="Free text, e.g. 13.x, 14.x">
                  <Input value={osReq} onChange={(e) => setOsReq(e.target.value)} disabled={ro} />
                </Field>
              </FormGrid>
            </CardBody>
          </Card>
          <Card>
            <CardHeader title="Script" description="Runs as root. $1 is the mount point, $2 the computer name, $3 the current user; $4–$11 are the policy parameters." />
            <CardBody>
              <CodeEditor language="shell" value={contents} onChange={setContents} height="480px" readOnly={ro} />
            </CardBody>
          </Card>
        </div>
        <Card className="self-start">
          <CardHeader title="Parameter labels" description="Give $4–$11 friendly names so they are easy to fill in when the script is added to a policy." />
          <CardBody className="flex flex-col gap-3">
            {labels.map((l, i) => (
              <Field key={i} label={`Parameter $${i + 4}`}>
                <Input value={l} onChange={(e) => setLabels(labels.map((x, j) => (j === i ? e.target.value : x)))} disabled={ro} placeholder="Label" />
              </Field>
            ))}
          </CardBody>
        </Card>
      </div>
      <ObjectBar
        ed={ed}
        isNew={isNew}
        listPath="/scripts"
        noun="script"
        name={script.data?.name}
        saving={saving}
        onSave={save}
        onDelete={doDelete}
        deleteDescription="Policies that include this script will no longer run it."
        onClone={() => navigate('/scripts/new', { state: { clone: { ...script.data, name: `Copy of ${name}`, category, info, notes, priority, parameter_labels: labels, contents, os_requirements: osReq, id: '' } } })}
        historyId={id}
        extras={
          !isNew &&
          isAdmin && (
            <Button variant="ghost" icon={<Play />} onClick={() => setRunOpen(true)}>
              Run on computers
            </Button>
          )
        }
      />
      {!isNew && script.data && <RunScriptOnComputersDialog open={runOpen} onClose={() => setRunOpen(false)} script={script.data} />}
    </>
  )
}

export function RunScriptOnComputersDialog({ open, onClose, script }: { open: boolean; onClose: () => void; script: Script }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const [targets, setTargets] = useState<Targets>(emptyTargets())
  const [params, setParams] = useState<string[]>(Array(8).fill(''))
  const [busy, setBusy] = useState(false)
  const labels = script.parameter_labels ?? []
  const run = async () => {
    setBusy(true)
    try {
      const r = await api.post<RunResult>(`/api/v1/scripts/${script.id}/run`, { ...targets, parameters: params })
      if (r.queued === 0 && r.skipped.length === 0) toast.warning('No computers targeted')
      else if (r.skipped.length) toast.warning(`Queued on ${r.queued} computer${r.queued === 1 ? '' : 's'}; ${r.skipped.length} skipped`, r.skipped.join('\n'))
      else toast.success(`Queued on ${r.queued} computer${r.queued === 1 ? '' : 's'}`)
      await invalidate(['tasks'], qk.dashboard)
      onClose()
    } catch (e) {
      toast.error('Run failed', errorMessage(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Run "${script.name}"`}
      description="The script is queued as an agent task on every targeted computer that has the agent installed."
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} data-shortcut="cancel">
            Cancel
          </Button>
          <Button variant="primary" loading={busy} data-shortcut="save" disabled={targets.computer_ids.length + targets.group_ids.length === 0} onClick={run}>
            Run script
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-6">
        <TargetPicker value={targets} onChange={setTargets} />
        <div>
          <p className="mb-2 text-sm font-medium text-ink">Parameters</p>
          <FormGrid cols={2}>
            {Array.from({ length: 8 }, (_, i) => (
              <Field key={i} label={`$${i + 4}${labels[i] ? ` — ${labels[i]}` : ''}`}>
                <Input value={params[i] ?? ''} onChange={(e) => setParams(params.map((v, j) => (j === i ? e.target.value : v)))} placeholder={labels[i] || 'Optional'} />
              </Field>
            ))}
          </FormGrid>
        </div>
      </div>
    </Modal>
  )
}
