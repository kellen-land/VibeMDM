import { useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Save, Trash2 } from 'lucide-react'
import { qk, useInvalidate, useRestrictedEvents, useRestrictedSoftwareItem } from '../api/queries'
import { api, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { emptyScope, normalizeScope, type RestrictedMatchType, type RestrictedSoftware } from '../api/types'
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
import { RestrictedEventTable } from '../components/activity/RestrictedEventTable'
import { matchTypeLabels } from './RestrictedSoftwarePage'

type Tab = 'general' | 'scope' | 'events'

const blank = (): RestrictedSoftware => ({
  id: '',
  name: '',
  process_name: '',
  match_type: 'exact',
  kill_process: true,
  notify_user: true,
  display_message: '',
  enabled: true,
  scope: emptyScope(),
  created_at: '',
  updated_at: '',
})

export function RestrictedSoftwareEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const item = useRestrictedSoftwareItem(isNew ? undefined : id)
  const [r, setR] = useState<RestrictedSoftware>(blank())
  const [saving, setSaving] = useState(false)
  const [del, setDel] = useState(false)
  const tab = (sp.get('tab') as Tab) || 'general'
  const setTab = (t: Tab) => setSp(t === 'general' ? {} : { tab: t }, { replace: true })

  useEffect(() => {
    if (item.data) setR({ ...blank(), ...item.data, scope: normalizeScope(item.data.scope) })
  }, [item.data])

  const set = (patch: Partial<RestrictedSoftware>) => setR((cur) => ({ ...cur, ...patch }))

  const save = async () => {
    if (!r.name.trim() || !r.process_name.trim()) {
      toast.error('Name and process name are required')
      return
    }
    setSaving(true)
    try {
      const body = { ...r, name: r.name.trim(), process_name: r.process_name.trim() }
      const out = isNew ? await api.post<RestrictedSoftware>('/api/v1/restricted-software', body) : await api.put<RestrictedSoftware>(`/api/v1/restricted-software/${id}`, body)
      await invalidate(qk.restricted, qk.restrictedItem(out.id))
      toast.success(isNew ? 'Rule created' : 'Rule saved', 'Scoped agents were asked to check in.')
      if (isNew) navigate(`/restricted-software/${out.id}`, { replace: true })
    } catch (e) {
      toast.error('Save failed', errorMessage(e))
    } finally {
      setSaving(false)
    }
  }
  const doDelete = async () => {
    try {
      await api.del(`/api/v1/restricted-software/${id}`)
      toast.success('Rule deleted')
      await invalidate(qk.restricted)
      navigate('/restricted-software')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  if (!isNew && item.isPending) return <LoadingBlock />
  if (!isNew && item.error) return <Alert tone="critical">{errorMessage(item.error)}</Alert>
  const ro = !isAdmin
  const tabs: { key: Tab; label: string }[] = [
    { key: 'general', label: 'General' },
    { key: 'scope', label: 'Scope' },
  ]
  if (!isNew) tabs.push({ key: 'events', label: 'Events' })

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Restricted Software', to: '/restricted-software' }, { label: isNew ? 'New rule' : item.data?.name ?? '' }]}
        title={isNew ? 'New restricted software rule' : item.data?.name}
        meta={
          !isNew ? (
            <>
              <Badge tone={r.enabled ? 'good' : 'neutral'} dot>
                {r.enabled ? 'Enabled' : 'Disabled'}
              </Badge>
              <span className="text-xs text-ink-muted font-mono">{r.process_name}</span>
            </>
          ) : undefined
        }
        actions={
          isAdmin && (
            <>
              {!isNew && (
                <Button variant="danger-outline" icon={<Trash2 />} onClick={() => setDel(true)}>
                  Delete
                </Button>
              )}
              <Button variant="primary" icon={<Save />} loading={saving} onClick={save}>
                {isNew ? 'Create rule' : 'Save changes'}
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
              <Field label="Display name" required>
                <Input value={r.name} onChange={(e) => set({ name: e.target.value })} disabled={ro} placeholder="e.g. Block BitTorrent" />
              </Field>
              <Field label="Process name" required hint={r.match_type === 'regex' ? 'Go regular expression matched against the process name.' : r.match_type === 'contains' ? 'Case-insensitive substring of the process name.' : 'Exact process name as shown by ps, e.g. "Transmission".'}>
                <Input value={r.process_name} onChange={(e) => set({ process_name: e.target.value })} disabled={ro} mono placeholder={r.match_type === 'regex' ? '^(Transmission|qbittorrent)$' : 'Transmission'} />
              </Field>
              <Field label="Match type">
                <Select value={r.match_type} onChange={(e) => set({ match_type: e.target.value as RestrictedMatchType })} disabled={ro} options={Object.entries(matchTypeLabels).map(([value, label]) => ({ value, label }))} />
              </Field>
            </FormGrid>
            <Toggle checked={r.enabled} onChange={(v) => set({ enabled: v })} disabled={ro} label="Enabled" description="Disabled rules are not sent to agents." />
            <FormSection title="Enforcement" description="What the agent does when a matching process is found.">
              <div className="grid gap-3 sm:grid-cols-2">
                <Checkbox checked={r.kill_process} onChange={(e) => set({ kill_process: e.target.checked })} disabled={ro} label="Kill process" description="Terminate the process immediately. Otherwise the event is only reported." />
                <Checkbox checked={r.notify_user} onChange={(e) => set({ notify_user: e.target.checked })} disabled={ro} label="Notify user" description="Show a notification with the message below." />
              </div>
              <Field label="Message shown to the user" className="max-w-2xl">
                <Textarea rows={3} value={r.display_message} onChange={(e) => set({ display_message: e.target.value })} disabled={ro || !r.notify_user} placeholder="This application is not permitted on company computers. Contact IT if you need it for your work." />
              </Field>
            </FormSection>
          </CardBody>
        </Card>
      )}

      {tab === 'scope' && (
        <Card>
          <CardHeader title="Scope" description="Which computers enforce this rule." />
          <CardBody>
            <ScopeEditor value={r.scope} onChange={(scope) => set({ scope })} disabled={ro} />
          </CardBody>
        </Card>
      )}

      {tab === 'events' && !isNew && id && <RuleEventsTab ruleId={id} />}

      <ConfirmDialog open={del} onClose={() => setDel(false)} onConfirm={doDelete} destructive title={`Delete "${r.name}"?`} description="Agents stop enforcing the rule at their next check-in." confirmLabel="Delete rule" />
    </>
  )
}

function RuleEventsTab({ ruleId }: { ruleId: string }) {
  // The events endpoint filters by computer only; filter by rule client-side within the fetched page.
  const [page, setPage] = useState({ limit: 100, offset: 0 })
  const q = useRestrictedEvents(page)
  const rows = q.data?.items.filter((e) => e.restricted_id === ruleId)
  return (
    <Card>
      <CardHeader title="Events" description="Violations of this rule within the most recent events." />
      <RestrictedEventTable events={rows} loading={q.isPending} error={q.error ? errorMessage(q.error) : null} footer={<Pagination total={q.data?.total ?? 0} limit={page.limit} offset={page.offset} onChange={setPage} pageSizes={[100, 200]} />} />
    </Card>
  )
}
