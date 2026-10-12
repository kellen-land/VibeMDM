import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { qk, useAppLabels, useCustomLabel, useInvalidate } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { emptyInstallomatorOptions, type CustomLabel, type InstallomatorOptions } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Field, FormGrid, FormSection } from '../components/ui/Field'
import { Input, Select, Textarea } from '../components/ui/Input'
import { Alert } from '../components/ui/Alert'
import { LoadingBlock } from '../components/ui/Spinner'
import { useToast } from '../components/ui/Toast'
import { ChipInput } from '../components/pickers/ChipInput'
import { InstallomatorCommandPreview, InstallomatorOptionsEditor } from '../components/installomator/Installomator'
import { InstallomatorFooter } from './AppsPage'
import { useCloneSource, useEditMode } from '../components/editor/useEditMode'
import { ObjectBar } from '../components/editor/ObjectBar'
import { SummaryCard } from '../components/templates/DetailsLayout'
import { RelativeTime } from '../components/ui/RelativeTime'

type Form = Omit<CustomLabel, 'id' | 'created_at' | 'updated_at' | 'blocking_processes'> & { blocking_processes: string[] }

const blank = (): Form => ({
  key: '',
  name: '',
  type: '',
  download_url: '',
  expected_team_id: '',
  package_id: '',
  app_name: '',
  archive_name: '',
  version_key: '',
  app_new_version: '',
  blocking_processes: [],
  description: '',
})

const typeHints: Record<string, string> = {
  pkg: 'A flat installer package.',
  dmg: 'A disk image containing the .app.',
  zip: 'A zip archive containing the .app.',
  tbz: 'A tar.bz2 archive containing the .app.',
  pkgInDmg: 'A disk image containing an installer package.',
  pkgInZip: 'A zip archive containing an installer package.',
  appInDmgInZip: 'A zip containing a disk image containing the .app.',
}

export function CustomLabelEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const label = useCustomLabel(isNew ? undefined : id)
  const labels = useAppLabels()
  const clone = useCloneSource<Form>()
  const [f, setF] = useState<Form>(() => (isNew && clone ? { ...blank(), ...clone, blocking_processes: clone.blocking_processes ?? [] } : blank()))
  const [previewOpts, setPreviewOpts] = useState<InstallomatorOptions>(emptyInstallomatorOptions())
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const load = useCallback(() => {
    if (label.data) {
      const { id: _i, created_at: _c, updated_at: _u, ...rest } = label.data
      void _i
      void _c
      void _u
      setF({ ...blank(), ...rest, blocking_processes: rest.blocking_processes ?? [] })
    }
  }, [label.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: f, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])

  const set = (patch: Partial<Form>) => setF((cur) => ({ ...cur, ...patch }))

  const save = async () => {
    setSaving(true)
    setSaveError(null)
    try {
      const body = { ...f, key: f.key.trim(), name: f.name.trim(), download_url: f.download_url.trim(), expected_team_id: f.expected_team_id.trim() }
      const out = isNew ? await api.post<CustomLabel>('/api/v1/custom-labels', body) : await api.put<CustomLabel>(`/api/v1/custom-labels/${id}`, body)
      await invalidate(qk.customLabels, qk.customLabel(out.id), qk.appLabels)
      toast.success(isNew ? 'Custom label created' : 'Custom label saved', `Use it as custom:${out.key}.`)
      if (isNew) navigate(`/apps/custom-labels/${out.id}`, { replace: true })
      return true
    } catch (e) {
      setSaveError(errorMessage(e))
      toast.error('Save failed', 'See the errors above the form.')
      return false
    } finally {
      setSaving(false)
    }
  }

  const doDelete = async () => {
    try {
      await api.del(`/api/v1/custom-labels/${id}`)
      toast.success('Custom label deleted')
      await invalidate(qk.customLabels, qk.appLabels)
      navigate('/apps/custom-labels')
    } catch (e) {
      setSaveError(errorMessage(e))
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (!isNew && label.isPending) return <LoadingBlock />
  if (!isNew && label.error) return <Alert tone="critical">{errorMessage(label.error)}</Alert>
  const ro = ed.readOnly
  const types = labels.data?.choices?.type ?? []
  const hasInput = !!(f.key || f.name || f.download_url || f.type || f.expected_team_id)

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Mac apps', to: '/apps' }, { label: 'Custom labels', to: '/apps/custom-labels' }, { label: isNew ? 'New' : label.data?.name ?? '' }]}
        title={isNew ? 'New custom label' : label.data?.name}
      />
      {saveError && (
        <Alert tone="critical" title="The label could not be saved" className="mb-5">
          <span className="whitespace-pre-wrap">{saveError}</span>
        </Alert>
      )}
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,26rem)]">
        <Card>
          <CardHeader title="Definition" description="Passed to Installomator with valuesfromarguments. Every value is validated and single-quoted by the server." />
          <CardBody className="flex flex-col gap-8">
            <FormGrid cols={2}>
              <Field label="Key" required hint={<>Lowercase letters, digits, - or _. Referenced as <span className="font-mono">custom:{f.key || '<key>'}</span>.</>}>
                <Input value={f.key} onChange={(e) => set({ key: e.target.value.toLowerCase() })} disabled={ro} mono placeholder="acme-tool" autoComplete="off" spellCheck={false} />
              </Field>
              <Field label="Name" required hint="The app or package name (Installomator's name).">
                <Input value={f.name} onChange={(e) => set({ name: e.target.value })} disabled={ro} placeholder="Acme Tool" />
              </Field>
              <Field label="Type" required hint={typeHints[f.type] ?? 'How the download is packaged.'}>
                <Select value={f.type} onChange={(e) => set({ type: e.target.value })} disabled={ro} options={[{ value: '', label: labels.isPending ? 'Loading…' : 'Select a type…' }, ...types.map((t) => ({ value: t, label: t })), ...(f.type && !types.includes(f.type) ? [{ value: f.type, label: f.type }] : [])]} />
              </Field>
              <Field label="Expected Team ID" required hint="The vendor's 10-character Apple Developer Team ID. Installomator refuses the install if the download's code signature doesn't match it.">
                <Input value={f.expected_team_id} onChange={(e) => set({ expected_team_id: e.target.value.toUpperCase() })} disabled={ro} mono maxLength={10} placeholder="ABCDE12345" autoComplete="off" spellCheck={false} />
              </Field>
              <Field label="Download URL" required hint="A plain https:// URL to the installer." className="md:col-span-2">
                <Input value={f.download_url} onChange={(e) => set({ download_url: e.target.value })} disabled={ro} mono placeholder="https://downloads.example.com/AcmeTool.dmg" autoComplete="off" spellCheck={false} />
              </Field>
            </FormGrid>
            <FormSection title="Optional" description="Same meaning as in Installomator labels; leave blank unless the vendor's packaging needs it.">
              <FormGrid cols={2}>
                <Field label="Package ID" hint="packageID: receipt identifier, for pkg types.">
                  <Input value={f.package_id} onChange={(e) => set({ package_id: e.target.value })} disabled={ro} mono placeholder="com.example.acmetool" />
                </Field>
                <Field label="App name" hint="appName: the .app inside the archive, if it differs from the name.">
                  <Input value={f.app_name} onChange={(e) => set({ app_name: e.target.value })} disabled={ro} placeholder="Acme Tool.app" />
                </Field>
                <Field label="Archive name" hint="archiveName: the downloaded file name.">
                  <Input value={f.archive_name} onChange={(e) => set({ archive_name: e.target.value })} disabled={ro} mono placeholder="AcmeTool.dmg" />
                </Field>
                <Field label="Version key" hint="versionKey: Info.plist key used for the installed version (default CFBundleShortVersionString).">
                  <Input value={f.version_key} onChange={(e) => set({ version_key: e.target.value })} disabled={ro} mono placeholder="CFBundleShortVersionString" />
                </Field>
                <Field label="App new version" hint="appNewVersion: the latest version, so up-to-date Macs skip the download.">
                  <Input value={f.app_new_version} onChange={(e) => set({ app_new_version: e.target.value })} disabled={ro} mono placeholder="1.2.3" />
                </Field>
                <Field label="Blocking processes" hint="blockingProcesses: process names that must not be running. Press Enter or comma to add.">
                  <ChipInput value={f.blocking_processes} onChange={(blocking_processes) => set({ blocking_processes })} disabled={ro} mono split={/,+/} placeholder="e.g. Acme Tool" />
                </Field>
                <Field label="Description" className="md:col-span-2">
                  <Textarea rows={2} value={f.description} onChange={(e) => set({ description: e.target.value })} disabled={ro} placeholder="Internal notes." />
                </Field>
              </FormGrid>
            </FormSection>
          </CardBody>
        </Card>
        <div className="flex flex-col gap-6 xl:sticky xl:top-6 xl:self-start">
          {!isNew && label.data && (
            <SummaryCard
              rows={[
                { label: 'Reference', value: <span className="font-mono">custom:{label.data.key}</span> },
                { label: 'Type', value: label.data.type && <span className="font-mono">{label.data.type}</span> },
                { label: 'Team ID', value: label.data.expected_team_id && <span className="font-mono">{label.data.expected_team_id}</span> },
                { label: 'Last changed', value: label.data.updated_at ? <RelativeTime value={label.data.updated_at} /> : '' },
              ]}
            />
          )}
          <Card>
            <CardHeader title="Command preview" description="Exactly what the agent runs, updated as you type." />
            <CardBody className="flex flex-col gap-4">
              <InstallomatorCommandPreview body={hasInput ? { custom: f, options: previewOpts } : null} emptyHint="Fill in the definition to see the command." />
              <FormSection title="Preview with options" description="Options are set where the label is used (Mac app or policy step); these only affect the preview.">
                <InstallomatorOptionsEditor value={previewOpts} onChange={setPreviewOpts} />
              </FormSection>
            </CardBody>
          </Card>
          <InstallomatorFooter />
        </div>
      </div>
      <ObjectBar
        ed={ed}
        isNew={isNew}
        listPath="/apps/custom-labels"
        noun="custom label"
        name={label.data ? `custom:${label.data.key}` : undefined}
        saving={saving}
        onSave={save}
        onDelete={doDelete}
        deleteDescription="Labels still used by a Mac app or a policy can't be deleted; remove them there first."
        onClone={() => navigate('/apps/custom-labels/new', { state: { clone: { ...f, key: `${f.key}-copy` } } })}
        historyId={id}
      />
    </>
  )
}
