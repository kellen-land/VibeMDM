import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Download, Play } from 'lucide-react'
import { qk, useMobileSearch, useMobileSearchRun, useMobileCriteriaFields, useInvalidate, useMobileSearchFields } from '../api/queries'
import { api, downloadFile, errorMessage } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { AdvancedSearch, Criterion, MobileDevice } from '../api/types'
import { PageHeader } from '../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { Field, FormGrid } from '../components/ui/Field'
import { Input } from '../components/ui/Input'
import { Checkbox } from '../components/ui/Checkbox'
import { Tabs } from '../components/ui/Tabs'
import { Table } from '../components/ui/Table'
import { Alert } from '../components/ui/Alert'
import { Badge } from '../components/ui/Badge'
import { LoadingBlock } from '../components/ui/Spinner'
import { RelativeTime } from '../components/ui/RelativeTime'
import { useToast } from '../components/ui/Toast'
import { useCloneSource, useEditMode } from '../components/editor/useEditMode'
import { ObjectBar } from '../components/editor/ObjectBar'
import { CriteriaBuilder, describeCriteria } from '../components/pickers/CriteriaBuilder'
import { defaultMobileDisplayFields, renderMobileField } from '../lib/mobileFields'

type Tab = 'settings' | 'results'

export function MobileSearchEditorPage() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const search = useMobileSearch(isNew ? undefined : id)
  const criteriaFields = useMobileCriteriaFields()
  const displayFieldOpts = useMobileSearchFields()
  const tab = (sp.get('tab') as Tab) || 'settings'
  const setTab = (t: Tab) => setSp(t === 'settings' ? {} : { tab: t }, { replace: true })

  const clone = useCloneSource<Pick<AdvancedSearch, 'name' | 'criteria' | 'display_fields'>>()
  const [name, setName] = useState(() => (isNew && clone ? clone.name : ''))
  const [criteria, setCriteria] = useState<Criterion[]>(() => (isNew && clone ? clone.criteria ?? [] : []))
  const [displayFields, setDisplayFields] = useState<string[]>(() => (isNew && clone?.display_fields?.length ? clone.display_fields : defaultMobileDisplayFields))
  const [saving, setSaving] = useState(false)
  const [exporting, setExporting] = useState(false)

  const load = useCallback(() => {
    if (search.data) {
      setName(search.data.name)
      setCriteria(search.data.criteria ?? [])
      setDisplayFields(search.data.display_fields?.length ? search.data.display_fields : defaultMobileDisplayFields)
    }
  }, [search.data])
  const ed = useEditMode({ isNew, canEdit: isAdmin, snapshot: { name, criteria, displayFields }, reset: load })
  useEffect(() => {
    if (!ed.editing) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load])
  // Clone onto the same route (/x/:id → /x/new keeps this page mounted): take the copy.
  useEffect(() => {
    if (isNew && clone) {
      setName(clone.name)
      setCriteria(clone.criteria ?? [])
      setDisplayFields(clone.display_fields?.length ? clone.display_fields : defaultMobileDisplayFields)
    }
  }, [isNew, clone])

  const save = async () => {
    if (!name.trim()) {
      setTab('settings')
      toast.error('Name is required')
      return false
    }
    setSaving(true)
    try {
      const body = { name: name.trim(), criteria, display_fields: displayFields }
      const out = isNew ? await api.post<AdvancedSearch>('/api/v1/mobile-device-searches', body) : await api.put<AdvancedSearch>(`/api/v1/mobile-device-searches/${id}`, body)
      await invalidate(qk.mobileSearches, qk.mobileSearch(out.id), ['mobile-search', out.id])
      toast.success(isNew ? 'Search created' : 'Search saved')
      if (isNew) navigate(`/mobile-searches/${out.id}?tab=results`, { replace: true })
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
      await api.del(`/api/v1/mobile-device-searches/${id}`)
      toast.success('Search deleted')
      await invalidate(qk.mobileSearches)
      navigate('/mobile-searches')
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }
  const exportCsv = async () => {
    setExporting(true)
    try {
      await downloadFile(`/api/v1/mobile-device-searches/${id}/export.csv`)
    } catch (e) {
      toast.error('Export failed', errorMessage(e))
    } finally {
      setExporting(false)
    }
  }

  if (!isNew && search.isPending) return <LoadingBlock />
  if (!isNew && search.error) return <Alert tone="critical">{errorMessage(search.error)}</Alert>
  const ro = ed.readOnly

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Mobile Device Searches', to: '/mobile-searches' }, { label: isNew ? 'New search' : search.data?.name ?? '' }]}
        title={isNew ? 'New mobile device search' : search.data?.name}
        meta={
          !isNew && search.data ? (
            <>
              <Badge>{criteria.length} rule{criteria.length === 1 ? '' : 's'}</Badge>
              <span className="text-xs text-ink-muted">
                Updated <RelativeTime value={search.data.updated_at} />
              </span>
            </>
          ) : undefined
        }
      />
      {!isNew && <Tabs className="mb-5" value={tab} onChange={setTab} items={[{ key: 'settings', label: 'Criteria & columns' }, { key: 'results', label: 'Results', icon: <Play />, shortcut: 'view' }]} />}

      {(tab === 'settings' || isNew) && (
        <div className="flex flex-col gap-6">
          <Card>
            <CardHeader title="General" />
            <CardBody>
              <FormGrid cols={2}>
                <Field label="Name" required>
                  <Input value={name} onChange={(e) => setName(e.target.value)} disabled={ro} placeholder="e.g. iPhones without a passcode" />
                </Field>
              </FormGrid>
            </CardBody>
          </Card>
          <Card>
            <CardHeader title="Criteria" description="Same rules as smart groups. Rules are evaluated top to bottom; use parentheses to group and/or conditions." />
            <CardBody className="flex flex-col gap-4">
              <CriteriaBuilder kind="mobile" value={criteria} onChange={setCriteria} disabled={ro} />
              {criteria.length > 0 && <p className="text-xs text-ink-muted font-mono rounded-lg bg-surface-sunken px-3 py-2">{describeCriteria(criteria, criteriaFields.data)}</p>}
            </CardBody>
          </Card>
          <Card>
            <CardHeader
              title="Display fields"
              description="Columns shown in the results table and included in the CSV export, in this order."
              actions={
                !ro && (
                  <>
                    <Button size="xs" variant="ghost" onClick={() => setDisplayFields(defaultMobileDisplayFields)}>
                      Reset to default
                    </Button>
                    <Button size="xs" variant="ghost" onClick={() => setDisplayFields((displayFieldOpts.data ?? []).map((f) => f.Key))}>
                      Select all
                    </Button>
                  </>
                )
              }
            />
            <CardBody>
              {displayFieldOpts.isPending ? (
                <LoadingBlock />
              ) : (
                <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                  {(displayFieldOpts.data ?? []).map((f) => {
                    const idx = displayFields.indexOf(f.Key)
                    return (
                      <Checkbox
                        key={f.Key}
                        checked={idx >= 0}
                        disabled={ro}
                        onChange={(e) => setDisplayFields(e.target.checked ? [...displayFields, f.Key] : displayFields.filter((k) => k !== f.Key))}
                        label={
                          <span className="inline-flex items-center gap-2">
                            {f.Label}
                            {idx >= 0 && <span className="text-[10px] text-ink-faint tabular-nums">#{idx + 1}</span>}
                          </span>
                        }
                      />
                    )
                  })}
                </div>
              )}
            </CardBody>
          </Card>
        </div>
      )}

      {tab === 'results' && !isNew && id && <ResultsTab searchId={id} dirty={ed.dirty} fallbackFields={displayFields} />}

      <ObjectBar
        ed={ed}
        isNew={isNew}
        listPath="/mobile-searches"
        noun="search"
        name={search.data?.name}
        saving={saving}
        onSave={save}
        onDelete={doDelete}
        deleteDescription="Only the saved search is removed."
        onClone={() => navigate('/mobile-searches/new', { state: { clone: { name: `Copy of ${name}`, criteria, display_fields: displayFields } } })}
        historyId={id}
        extras={
          !isNew && (
            <Button variant="ghost" icon={<Download />} loading={exporting} onClick={exportCsv} title="Download the saved search results as CSV">
              Export CSV
            </Button>
          )
        }
      />
    </>
  )
}

function ResultsTab({ searchId, dirty, fallbackFields }: { searchId: string; dirty: boolean; fallbackFields: string[] }) {
  const navigate = useNavigate()
  const fieldOpts = useMobileSearchFields()
  const run = useMobileSearchRun(searchId, { limit: 500 })
  const labels = useMemo(() => new Map((fieldOpts.data ?? []).map((f) => [f.Key, f.Label])), [fieldOpts.data])
  const keys = run.data?.search.display_fields?.length ? run.data.search.display_fields : fallbackFields.length ? fallbackFields : defaultMobileDisplayFields
  return (
    <Card>
      <CardHeader
        title={run.data ? `${run.data.count.toLocaleString()} matching ${run.data.count === 1 ? 'device' : 'devices'}` : 'Results'}
        description={dirty ? 'Showing results for the last saved version. Save your changes to re-run with the new criteria.' : 'Results reflect the saved criteria. Showing up to 500 rows; the CSV export includes everything.'}
        actions={
          <Button size="sm" icon={<Play />} loading={run.isFetching} onClick={() => run.refetch()}>
            Run again
          </Button>
        }
      />
      <Table<MobileDevice>
        rows={run.data?.mobile_devices}
        loading={run.isPending}
        error={run.error ? errorMessage(run.error) : null}
        rowKey={(c) => c.id}
        dense
        onRowClick={(c) => navigate(`/mobile-devices/${c.id}`)}
        emptyTitle="No mobile devices match these criteria"
        columns={keys.map((k) => ({ key: k, header: labels.get(k) ?? k, render: (c: MobileDevice) => renderMobileField(k, c) }))}
      />
    </Card>
  )
}
