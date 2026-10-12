import { useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Save, Trash2 } from 'lucide-react'
import { qk, useApp, useAppLabels, useAppLogs, useInvalidate } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { emptyInstallomatorOptions, emptyScope, normalizeScope, type AppDeployment, type AppUpdateBehavior, type MacApp } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Field, FormGrid, FormSection } from '../components/ui/Field'
import { Input, Select, Textarea } from '../components/ui/Input'
import { Checkbox, Toggle } from '../components/ui/Checkbox'
import { Tabs } from '../components/ui/Tabs'
import { Alert } from '../components/ui/Alert'
import { Badge } from '../components/ui/Badge'
import { LoadingBlock } from '../components/ui/Spinner'
import { ConfirmDialog } from '../components/ui/ConfirmDialog'
import { Pagination } from '../components/ui/Pagination'
import { useToast } from '../components/ui/Toast'
import { ScopeEditor } from '../components/pickers/ScopeEditor'
import { LabelCombobox, allLabels } from '../components/pickers/LabelCombobox'
import { InstallomatorCommandPreview, InstallomatorOptionsEditor } from '../components/installomator/Installomator'
import { AppInstallLogTable } from '../components/activity/AppInstallLogTable'
import { InstallomatorFooter, updateBehaviorLabels } from './AppsPage'

type Tab = 'general' | 'scope' | 'selfservice' | 'logs'

const blank = (): MacApp => ({
  id: '',
  name: '',
  label: '',
  category: '',
  description: '',
  update_behavior: 'install_once',
  deployment: 'automatic',
  options: emptyInstallomatorOptions(),
  enabled: true,
  scope: emptyScope(),
  self_service: { enabled: false, display_name: '', description: '', button_text: 'Install' },
  created_at: '',
  updated_at: '',
})

export function AppEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const app = useApp(isNew ? undefined : id)
  const labels = useAppLabels()
  const [a, setA] = useState<MacApp>(blank())
  const [saving, setSaving] = useState(false)
  const [del, setDel] = useState(false)
  const tab = (sp.get('tab') as Tab) || 'general'
  const setTab = (t: Tab) => setSp(t === 'general' ? {} : { tab: t }, { replace: true })

  useEffect(() => {
    if (app.data) setA({ ...blank(), ...app.data, scope: normalizeScope(app.data.scope), options: { ...emptyInstallomatorOptions(), ...app.data.options }, self_service: { ...blank().self_service, ...app.data.self_service } })
  }, [app.data])

  const set = (patch: Partial<MacApp>) => setA((cur) => ({ ...cur, ...patch }))
  const ssOn = a.deployment === 'self_service' || a.self_service.enabled

  const save = async () => {
    if (!a.name.trim()) {
      toast.error('Name is required')
      return
    }
    if (!a.label.trim() || (labels.data && !allLabels(labels.data).includes(a.label.trim()))) {
      toast.error('A valid Installomator label is required')
      return
    }
    setSaving(true)
    try {
      const body = { ...a, name: a.name.trim(), label: a.label.trim() }
      const out = isNew ? await api.post<MacApp>('/api/v1/apps', body) : await api.put<MacApp>(`/api/v1/apps/${id}`, body)
      await invalidate(qk.apps, qk.app(out.id))
      toast.success(isNew ? 'App added' : 'App saved', 'Scoped agents were asked to check in.')
      if (isNew) navigate(`/apps/${out.id}`, { replace: true })
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }
  const doDelete = async () => {
    try {
      await api.del(`/api/v1/apps/${id}`)
      toast.success('App removed')
      await invalidate(qk.apps)
      navigate('/apps')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (!isNew && app.isPending) return <LoadingBlock />
  if (!isNew && app.error) return <Alert tone="critical">{errorMessage(app.error)}</Alert>
  const ro = !isAdmin
  const tabs: { key: Tab; label: string }[] = [
    { key: 'general', label: 'General' },
    { key: 'scope', label: 'Scope' },
    { key: 'selfservice', label: 'Self Service' },
  ]
  if (!isNew) tabs.push({ key: 'logs', label: 'Install Logs' })

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Mac Apps', to: '/apps' }, { label: isNew ? 'Add app' : app.data?.name ?? '' }]}
        title={isNew ? 'Add app to catalog' : app.data?.name}
        meta={
          !isNew ? (
            <>
              <Badge tone={a.enabled ? 'good' : 'neutral'} dot>
                {a.enabled ? 'Enabled' : 'Disabled'}
              </Badge>
              <span className="text-xs text-ink-muted font-mono">{a.label}</span>
              <Badge tone={a.update_behavior === 'keep_updated' ? 'accent' : 'neutral'}>{updateBehaviorLabels[a.update_behavior]}</Badge>
            </>
          ) : undefined
        }
        actions={
          isAdmin && (
            <>
              {!isNew && (
                <Button variant="danger-outline" icon={<Trash2 />} onClick={() => setDel(true)}>
                  Remove
                </Button>
              )}
              <Button variant="primary" icon={<Save />} loading={saving} onClick={save}>
                {isNew ? 'Add app' : 'Save changes'}
              </Button>
            </>
          )
        }
      />
      <Tabs className="mb-5" value={tab} onChange={setTab} items={tabs} />

      {tab === 'general' && (
        <Card>
          <CardHeader title="General" />
          <CardBody className="flex flex-col gap-8">
            <FormGrid cols={2}>
              <Field label="Installomator label" required hint={labels.data ? `${labels.data.labels.length.toLocaleString()} labels from ${labels.data.source}${labels.data.custom_labels?.length ? ` plus ${labels.data.custom_labels.length} custom` : ''}.` : 'The label determines what gets downloaded and how it is verified.'}>
                <LabelCombobox
                  value={a.label}
                  onChange={(label) => set({ label, name: a.name || label })}
                  disabled={ro}
                />
              </Field>
              <Field label="Display name" required>
                <Input value={a.name} onChange={(e) => set({ name: e.target.value })} disabled={ro} placeholder="e.g. Google Chrome" />
              </Field>
              <Field label="Category">
                <Input value={a.category} onChange={(e) => set({ category: e.target.value })} disabled={ro} placeholder="e.g. Browsers" />
              </Field>
              <Field
                label="Deployment"
                hint={a.deployment === 'self_service' ? 'Macs in scope see it in Self Service; it is installed only when someone clicks Install.' : 'Installed on every Mac in scope at its next check-in.'}
              >
                <Select
                  value={a.deployment}
                  onChange={(e) => set({ deployment: e.target.value as AppDeployment })}
                  disabled={ro}
                  options={[
                    { value: 'automatic', label: 'Install automatically' },
                    { value: 'self_service', label: 'Self Service only' },
                  ]}
                />
              </Field>
              <Field
                label="Updates"
                hint={
                  a.update_behavior === 'keep_updated'
                    ? a.deployment === 'self_service'
                      ? 'Re-runs Installomator weekly on Macs where the app is installed (from Self Service or otherwise), so it stays on the latest vendor release.'
                      : 'Re-runs Installomator weekly so the app stays on the latest vendor release.'
                    : 'Installed once; later updates are left to the app or the user.'
                }
              >
                <Select value={a.update_behavior} onChange={(e) => set({ update_behavior: e.target.value as AppUpdateBehavior })} disabled={ro} options={Object.entries(updateBehaviorLabels).map(([value, label]) => ({ value, label }))} />
              </Field>
              <Field label="Description" className="md:col-span-2">
                <Textarea rows={3} value={a.description} onChange={(e) => set({ description: e.target.value })} disabled={ro} placeholder="Internal notes about why this app is deployed." />
              </Field>
            </FormGrid>
            <Toggle checked={a.enabled} onChange={(v) => set({ enabled: v })} disabled={ro} label="Enabled" description="Disabled apps are not installed, updated or shown in Self Service." />
            <FormSection title="Installomator options" description="Applied to every install and update of this app. Leave a field on Default to use Installomator's own default.">
              <div className="flex flex-col gap-4">
                <InstallomatorOptionsEditor value={a.options} onChange={(options) => set({ options })} disabled={ro} />
                <InstallomatorCommandPreview body={a.label.trim() ? { label: a.label.trim(), options: a.options } : null} emptyHint="Pick a label to see the command." />
              </div>
            </FormSection>
            <InstallomatorFooter />
          </CardBody>
        </Card>
      )}

      {tab === 'scope' && (
        <Card>
          <CardHeader
            title="Scope"
            description={a.deployment === 'self_service' ? 'Computers in scope see the app in Self Service.' : 'Computers in scope get the app installed automatically at their next check-in.'}
          />
          <CardBody>
            <ScopeEditor value={a.scope} onChange={(scope) => set({ scope })} disabled={ro} />
          </CardBody>
        </Card>
      )}

      {tab === 'selfservice' && (
        <Card>
          <CardHeader
            title="Self Service"
            description={a.deployment === 'self_service' ? 'How the app appears in the Self Service app on each Mac in scope.' : 'Optionally also list the app in Self Service, for example to reinstall it.'}
          />
          <CardBody className="flex flex-col gap-5">
            {a.deployment !== 'self_service' && (
              <Toggle checked={a.self_service.enabled} onChange={(v) => set({ self_service: { ...a.self_service, enabled: v } })} disabled={ro} label="Also show in Self Service" />
            )}
            <FormSection title="Listing">
              <FormGrid cols={2}>
                <Field label="Display name">
                  <Input value={a.self_service.display_name} onChange={(e) => set({ self_service: { ...a.self_service, display_name: e.target.value } })} disabled={ro || !ssOn} placeholder={a.name} />
                </Field>
                <Field label="Button text">
                  <Input value={a.self_service.button_text} onChange={(e) => set({ self_service: { ...a.self_service, button_text: e.target.value } })} disabled={ro || !ssOn} placeholder="Install" />
                </Field>
                <Field label="Description" className="md:col-span-2">
                  <Textarea rows={3} value={a.self_service.description} onChange={(e) => set({ self_service: { ...a.self_service, description: e.target.value } })} disabled={ro || !ssOn} />
                </Field>
                <Field label="Category" hint="Groups the item in Self Service.">
                  <Input value={a.self_service.category ?? ''} onChange={(e) => set({ self_service: { ...a.self_service, category: e.target.value } })} disabled={ro || !ssOn} placeholder={a.category || 'General'} />
                </Field>
                <Field label="Featured">
                  <Checkbox checked={!!a.self_service.featured} onChange={(e) => set({ self_service: { ...a.self_service, featured: e.target.checked } })} disabled={ro || !ssOn} label="Show on the Discover page" />
                </Field>
              </FormGrid>
            </FormSection>
          </CardBody>
        </Card>
      )}

      {tab === 'logs' && !isNew && id && <AppLogsTab appId={id} />}

      <ConfirmDialog open={del} onClose={() => setDel(false)} onConfirm={doDelete} destructive title={`Remove "${a.name}" from the catalog?`} description="Computers that already have the app keep it." confirmLabel="Remove app" />
    </>
  )
}

function AppLogsTab({ appId }: { appId: string }) {
  const [page, setPage] = useState({ limit: 25, offset: 0 })
  const logs = useAppLogs(appId, page)
  return (
    <Card>
      <CardHeader title="Install logs" description="Each Installomator run on each computer. Expand a row for the full output." />
      {logs.error && <p className="px-5 py-4 text-sm text-critical">{errorMessage(logs.error)}</p>}
      <AppInstallLogTable logs={logs.data?.items} loading={logs.isPending} showApp={false} />
      <Pagination total={logs.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} />
    </Card>
  )
}
