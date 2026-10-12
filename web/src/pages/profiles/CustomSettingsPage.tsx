import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Boxes, FileJson, Pencil, Plus, Trash2, Upload } from 'lucide-react'
import { qk, schemaPath, useCustomSchema, useCustomSchemas, useInvalidate } from '../../api/queries'
import { api, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { CustomSchemaDetail, CustomSchemaSummary } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card } from '../../components/ui/Card'
import { Table } from '../../components/ui/Table'
import { Button } from '../../components/ui/Button'
import { Badge } from '../../components/ui/Badge'
import { Modal } from '../../components/ui/Modal'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Textarea } from '../../components/ui/Input'
import { Alert } from '../../components/ui/Alert'
import { LoadingBlock } from '../../components/ui/Spinner'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { SearchInput } from '../../components/ui/SearchInput'
import { useToast } from '../../components/ui/Toast'
import { checkSchemaText } from '../../components/schema/schema'

export const customSettingsCrumb = { label: 'Application & Custom Settings', to: '/profiles/custom-settings' }

export function CustomSettingsPage() {
  const { isAdmin } = useAuth()
  const schemas = useCustomSchemas()
  const toast = useToast()
  const invalidate = useInvalidate()
  const [q, setQ] = useState('')
  const [dialog, setDialog] = useState<{ id?: string } | null>(null)
  const [del, setDel] = useState<CustomSchemaSummary | null>(null)

  const rows = useMemo(() => {
    const s = q.trim().toLowerCase()
    const all = (schemas.data ?? []).filter((x) => !s || x.name.toLowerCase().includes(s) || x.preference_domain.toLowerCase().includes(s) || (x.title ?? '').toLowerCase().includes(s))
    // Built-ins first, then custom by name.
    return [...all].sort((a, b) => (a.builtin === b.builtin ? a.name.localeCompare(b.name) : a.builtin ? -1 : 1))
  }, [schemas.data, q])

  const doDelete = async () => {
    if (!del) return
    try {
      await api.del(schemaPath(del.id))
      toast.success('Schema deleted', del.profiles ? 'Profiles built from it keep their payload but can no longer be edited with the form.' : undefined)
      await invalidate(qk.customSchemas, qk.profiles)
      setDel(null)
    } catch (e) {
      toast.error('Delete failed', errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader
        crumbs={[{ label: 'Configuration Profiles', to: '/profiles' }, { label: 'Application & Custom Settings' }]}
        title="Application & Custom Settings"
        description="Build managed-preference profiles for apps from a JSON schema. VibeMDM renders the schema as a form and generates the profile."
        actions={
          isAdmin && (
            <Button variant="primary" icon={<Plus />} onClick={() => setDialog({})} data-shortcut="new">
              Add schema
            </Button>
          )
        }
      />
      <Card>
        <div className="flex items-center gap-3 border-b border-line px-5 py-3">
          <SearchInput value={q} onChange={setQ} placeholder="Filter schemas…" className="w-full sm:w-72" />
        </div>
        <Table<CustomSchemaSummary>
          rows={rows}
          loading={schemas.isPending}
          error={schemas.error ? errorMessage(schemas.error) : null}
          rowKey={(s) => s.id}
          emptyTitle={q ? 'No schemas match' : 'No schemas yet'}
          emptyDescription={q ? undefined : 'Add a Jamf-style external application schema to build a profile from a form.'}
          columns={[
            {
              key: 'name',
              header: 'Schema',
              render: (s) => (
                <span className="flex items-start gap-2.5">
                  {s.builtin ? <Boxes className="mt-0.5 size-4 shrink-0 text-ink-faint" aria-hidden="true" /> : <FileJson className="mt-0.5 size-4 shrink-0 text-ink-faint" aria-hidden="true" />}
                  <span className="min-w-0">
                    <span className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-ink">{s.name}</span>
                      {s.builtin && <Badge tone="accent">Built-in</Badge>}
                    </span>
                    <span className="block max-w-md truncate font-mono text-xs text-ink-muted">{s.preference_domain}</span>
                    {s.description && (
                      <span className="mt-0.5 block max-w-xl text-xs text-ink-muted line-clamp-2" title={s.description}>
                        {s.description}
                      </span>
                    )}
                  </span>
                </span>
              ),
            },
            { key: 'fields', header: 'Fields', align: 'right', render: (s) => <span className="tabular-nums text-ink-muted">{s.properties}</span> },
            {
              key: 'profiles',
              header: 'Profiles',
              align: 'right',
              render: (s) => <span className="tabular-nums text-ink-muted">{s.profiles}</span>,
            },
            { key: 'updated', header: 'Updated', render: (s) => (s.builtin ? <span className="text-xs text-ink-faint">Bundled</span> : <RelativeTime value={s.updated_at} className="text-ink-muted" />) },
            {
              key: 'actions',
              header: '',
              align: 'right',
              render: (s) =>
                isAdmin ? (
                  <span className="flex items-center justify-end gap-1">
                    <Link to={`/profiles/custom-settings/${encodeURIComponent(s.id)}/new`}>
                      <Button size="xs" icon={<Plus />}>
                        New profile
                      </Button>
                    </Link>
                    {!s.builtin && (
                      <>
                        <Button size="xs" variant="ghost" icon={<Pencil />} aria-label={`Edit ${s.name}`} title="Edit schema" onClick={() => setDialog({ id: s.id })} />
                        <Button size="xs" variant="ghost" icon={<Trash2 />} aria-label={`Delete ${s.name}`} title="Delete schema" onClick={() => setDel(s)} />
                      </>
                    )}
                  </span>
                ) : null,
            },
          ]}
        />
      </Card>
      <p className="mt-3 text-xs text-ink-muted">
        Schemas use the Jamf Pro &ldquo;Application &amp; Custom Settings&rdquo; format. super and XCreds also have dedicated pages under{' '}
        <Link to="/updates/super" className="text-ink hover:underline">
          Software Updates
        </Link>{' '}
        and{' '}
        <Link to="/identity/xcreds" className="text-ink hover:underline">
          Identity
        </Link>
        .
      </p>
      {dialog && <SchemaDialog id={dialog.id} onClose={() => setDialog(null)} />}
      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={doDelete}
        destructive
        title={`Delete "${del?.name}"?`}
        description={
          del?.profiles
            ? `${del.profiles} ${del.profiles === 1 ? 'profile was' : 'profiles were'} built from this schema. They keep their payload and stay deployed, but can no longer be edited with the form.`
            : 'No profiles use this schema.'
        }
        confirmLabel="Delete schema"
      />
    </>
  )
}

/** Add (no id) or edit a custom schema. */
function SchemaDialog({ id, onClose }: { id?: string; onClose: () => void }) {
  const editing = !!id
  const existing = useCustomSchema(id)
  const toast = useToast()
  const invalidate = useInvalidate()
  const fileRef = useRef<HTMLInputElement>(null)
  const [name, setName] = useState('')
  const [domain, setDomain] = useState('')
  const [text, setText] = useState('')
  const [touched, setTouched] = useState(false)
  const [serverError, setServerError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (existing.data) {
      setName(existing.data.name)
      setDomain(existing.data.preference_domain)
      setText(JSON.stringify(existing.data.schema, null, 2))
    }
  }, [existing.data])

  const check = useMemo(() => checkSchemaText(text), [text])
  const parsed = check.schema as { title?: string; __preferencedomain?: string } | undefined
  const effectiveDomain = domain.trim() || parsed?.__preferencedomain || ''

  const onFile = async (f: File | undefined) => {
    if (!f) return
    const t = await f.text()
    setText(t)
    setTouched(true)
    setServerError(null)
    if (fileRef.current) fileRef.current.value = ''
  }

  const save = async () => {
    setTouched(true)
    if (check.error) return
    if (!effectiveDomain) {
      setServerError('Enter a preference domain; this schema has no __preferencedomain.')
      return
    }
    setBusy(true)
    setServerError(null)
    try {
      const body = { name: name.trim(), preference_domain: domain.trim(), schema: check.schema }
      const out = editing ? await api.put<CustomSchemaDetail>(schemaPath(id!), body) : await api.post<CustomSchemaDetail>('/api/v1/custom-schemas', body)
      await invalidate(qk.customSchemas, qk.customSchema(out.id))
      toast.success(editing ? 'Schema saved' : 'Schema added', editing ? 'Existing profiles keep their payload until you save them again.' : `${out.name} is ready for new profiles.`)
      onClose()
    } catch (e) {
      setServerError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      size="lg"
      title={editing ? 'Edit schema' : 'Add schema'}
      description="Paste or upload a Jamf-style external application JSON schema."
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy} data-shortcut="cancel">
            Cancel
          </Button>
          <Button variant="primary" onClick={save} loading={busy} disabled={editing && existing.isPending} data-shortcut="save">
            {editing ? 'Save schema' : 'Add schema'}
          </Button>
        </>
      }
    >
      {editing && existing.isPending ? (
        <LoadingBlock />
      ) : (
        <div className="flex flex-col gap-4">
          {serverError && <Alert tone="critical">{serverError}</Alert>}
          <FormGrid cols={2}>
            <Field label="Name" hint="Defaults to the schema's title.">
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={parsed?.title || 'e.g. Company Portal settings'} />
            </Field>
            <Field label="Preference domain" hint={parsed?.__preferencedomain && !domain ? 'From the schema’s __preferencedomain.' : 'The app’s bundle identifier, e.g. com.example.app.'}>
              <Input mono value={domain} onChange={(e) => setDomain(e.target.value)} placeholder={parsed?.__preferencedomain || 'com.example.app'} spellCheck={false} />
            </Field>
          </FormGrid>
          <Field
            label="Schema JSON"
            error={touched && check.error ? check.error : undefined}
            hint={check.schema ? `${Object.keys((check.schema as { properties: object }).properties).length} top-level settings` : undefined}
          >
            <Textarea
              mono
              rows={14}
              value={text}
              onChange={(e) => {
                setText(e.target.value)
                setServerError(null)
              }}
              onBlur={() => setTouched(true)}
              placeholder={'{\n  "title": "Example",\n  "__preferencedomain": "com.example.app",\n  "properties": { … }\n}'}
              spellCheck={false}
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                if (e.dataTransfer.files?.[0]) {
                  e.preventDefault()
                  onFile(e.dataTransfer.files[0])
                }
              }}
            />
          </Field>
          <div className="flex items-center gap-2">
            <input ref={fileRef} type="file" accept=".json,application/json" className="hidden" onChange={(e) => onFile(e.target.files?.[0])} />
            <Button size="sm" icon={<Upload />} onClick={() => fileRef.current?.click()}>
              Upload .json
            </Button>
            <span className="text-xs text-ink-muted">or drop a file on the text area.</span>
          </div>
        </div>
      )}
    </Modal>
  )
}
