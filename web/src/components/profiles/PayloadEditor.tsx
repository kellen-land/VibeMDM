import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Plus, X } from 'lucide-react'
import { api, errorMessage } from '../../api/client'
import type { PayloadDef, PayloadErrors, PayloadValues, ProfilePreview, SchemaValues } from '../../api/types'
import { Card, CardBody, CardHeader } from '../ui/Card'
import { Button } from '../ui/Button'
import { Badge } from '../ui/Badge'
import { Alert } from '../ui/Alert'
import { Modal } from '../ui/Modal'
import { SearchInput } from '../ui/SearchInput'
import { CodeBlock } from '../ui/CopyButton'
import { SchemaForm, PayloadRefContext, type PayloadRefTarget } from '../schema/SchemaForm'
import { pruneValues } from '../schema/schema'
import { cn } from '../../lib/cn'

/** 'general', 'xml' (uploaded profiles), or the index of a payload in the profile. */
export type PayloadSelection = 'general' | 'xml' | number

/** Payload types other payloads can point to (certificate pickers in Wi-Fi, VPN, Mail, …). */
const certTypes = ['com.apple.security.pkcs1', 'com.apple.security.root', 'com.apple.security.pem', 'com.apple.security.pkcs12', 'com.apple.security.scep', 'com.apple.security.acme', 'com.apple.ADCertificate.managed']

const newUUID = () => crypto.randomUUID().toUpperCase()

export interface PayloadEditorProps {
  catalog: PayloadDef[]
  payloads: PayloadValues[]
  onChange: (p: PayloadValues[]) => void
  errors: PayloadErrors[]
  selected: PayloadSelection
  onSelect: (s: PayloadSelection) => void
  readOnly: boolean
  /** The General card (name, category, …). */
  general: ReactNode
  /** Uploaded profiles: the XML card replaces the payload list. */
  xml?: ReactNode
  /** macOS: only payloads available at this level can be added. */
  level?: 'device' | 'user'
  /** Server endpoint that renders the XML preview, and the body to send it (payloads are added). */
  preview: { path: string; body: Record<string, unknown> }
}

/**
 * Jamf-style payload editing for configuration profiles: the profile's payloads on the left,
 * the selected one's settings on the right, and a catalog to add more.
 */
export function PayloadEditor({ catalog, payloads, onChange, errors, selected, onSelect, readOnly, general, xml, level, preview }: PayloadEditorProps) {
  const defs = useMemo(() => new Map(catalog.map((d) => [d.type, d])), [catalog])
  const [adding, setAdding] = useState(false)
  const errorsFor = (i: number) => errors.find((e) => e.index === i)?.errors ?? []

  // Certificates need a UUID before other payloads can point to them (older profiles lack one).
  useEffect(() => {
    if (payloads.some((p) => !p.uuid && certTypes.includes(defs.get(p.type)?.payload_type ?? p.type))) {
      onChange(payloads.map((p) => (!p.uuid && certTypes.includes(defs.get(p.type)?.payload_type ?? p.type) ? { ...p, uuid: newUUID() } : p)))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [payloads, defs])

  const refTargets: PayloadRefTarget[] = payloads
    .filter((p) => p.uuid)
    .map((p) => ({ uuid: p.uuid!, type: defs.get(p.type)?.payload_type ?? p.type, label: `${defs.get(p.type)?.title ?? p.type}${payloadHint(p) ? ` · ${payloadHint(p)}` : ''}` }))

  const add = (d: PayloadDef) => {
    onChange([...payloads, { type: d.type, uuid: newUUID(), values: {} }])
    onSelect(payloads.length)
    setAdding(false)
  }
  const remove = (i: number) => {
    onChange(payloads.filter((_, j) => j !== i))
    onSelect('general')
  }

  const sel = typeof selected === 'number' ? payloads[selected] : undefined
  const selDef = sel ? defs.get(sel.type) : undefined
  const wrongLevel = (d: PayloadDef | undefined) => !!(level && d?.channels?.length && !d.channels.includes(level))

  return (
    <div className="grid gap-6 lg:grid-cols-[15rem_1fr]">
      <nav aria-label="Payloads" className="flex flex-col gap-1">
        <SideItem active={selected === 'general'} onClick={() => onSelect('general')} label="General" />
        {xml ? (
          <SideItem active={selected === 'xml'} onClick={() => onSelect('xml')} label="Profile XML" />
        ) : (
          payloads.map((p, i) => (
            <SideItem
              key={p.uuid ?? i}
              active={selected === i}
              onClick={() => onSelect(i)}
              label={defs.get(p.type)?.title ?? p.type}
              hint={payloadHint(p)}
              error={errorsFor(i).length > 0 || wrongLevel(defs.get(p.type))}
            />
          ))
        )}
        {!xml && !readOnly && (
          <Button size="sm" variant="ghost" icon={<Plus />} className="mt-2 justify-start" onClick={() => setAdding(true)}>
            Add payload
          </Button>
        )}
      </nav>

      <div className="flex min-w-0 flex-col gap-6">
        {selected === 'general' && (
          <>
            {general}
            {!xml && payloads.length === 0 && !readOnly && (
              <div className="flex items-center justify-between rounded-lg border border-dashed border-line px-4 py-3">
                <p className="text-sm text-ink-muted">Next, add the settings this profile carries.</p>
                <Button size="sm" icon={<Plus />} onClick={() => setAdding(true)}>
                  Add payload
                </Button>
              </div>
            )}
          </>
        )}
        {selected === 'xml' && xml}
        {sel && (
          <Card>
            <CardHeader
              title={selDef?.title ?? sel.type}
              description={selDef?.description}
              actions={
                !readOnly && (
                  <Button size="sm" variant="ghost" icon={<X />} onClick={() => remove(selected as number)}>
                    Remove
                  </Button>
                )
              }
            />
            <CardBody className="flex flex-col gap-4">
              {selDef && <PayloadNotes def={selDef} />}
              {wrongLevel(selDef) && <Alert tone="warning">This payload can't be used at this level. Remove it or change the profile's level.</Alert>}
              {selDef ? (
                <PayloadRefContext.Provider value={refTargets.filter((t) => t.uuid !== sel.uuid)}>
                  <SchemaForm
                    key={sel.uuid ?? selected}
                    schema={selDef.schema}
                    value={sel.values}
                    onChange={(v: SchemaValues) => onChange(payloads.map((p, i) => (i === selected ? { ...p, values: v } : p)))}
                    errors={errorsFor(selected as number)}
                    disabled={readOnly}
                  />
                </PayloadRefContext.Provider>
              ) : (
                <Alert tone="warning">This payload type has no editor in this version of VibeMDM.</Alert>
              )}
            </CardBody>
          </Card>
        )}
        {!xml && payloads.length > 0 && <XmlPreview path={preview.path} body={{ ...preview.body, payloads: payloads.map((p) => ({ type: p.type, uuid: p.uuid, values: pruneValues(p.values) })) }} />}
      </div>

      <Modal open={adding} onClose={() => setAdding(false)} title="Add a payload" size="lg">
        <PayloadCatalog defs={catalog} used={payloads.map((p) => p.type)} level={level} onPick={add} />
      </Modal>
    </div>
  )
}

function SideItem({ active, onClick, label, hint, error }: { active: boolean; onClick: () => void; label: string; hint?: string; error?: boolean }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? 'page' : undefined}
      className={cn('rounded-md px-2 py-1.5 text-left text-[13px] focus-ring', active ? 'bg-accent-500/12 font-medium text-ink' : 'text-ink-muted hover:bg-line/40 hover:text-ink')}
    >
      <span className="flex items-center gap-2">
        <span className="truncate">{label}</span>
        {error && <span className="size-1.5 shrink-0 rounded-full bg-critical" aria-label="needs attention" />}
      </span>
      {hint && <span className="block truncate text-xs text-ink-faint">{hint}</span>}
    </button>
  )
}

/** A short hint so several payloads of one type can be told apart. */
export function payloadHint(p: PayloadValues): string | undefined {
  const v = p.values as Record<string, unknown>
  for (const k of ['PayloadDisplayName', 'SSID_STR', 'UserDefinedName', 'Label', 'PayloadCertificateFileName', 'EmailAccountDescription', 'Host', 'Name']) {
    if (typeof v[k] === 'string' && v[k]) return v[k] as string
  }
  return undefined
}

function defNotes(d: PayloadDef): string[] {
  return [
    d.supervised && 'Supervised',
    ...(d.requires ?? []),
    d.os,
    d.platform === 'macos' && d.channels?.length === 1 && (d.channels[0] === 'user' ? 'User level only' : 'Computer level only'),
  ].filter(Boolean) as string[]
}

function PayloadNotes({ def }: { def: PayloadDef }) {
  const notes = defNotes(def)
  if (!notes.length && !def.jamf && !def.deprecated) return null
  return (
    <p className="flex flex-wrap items-center gap-1.5 text-xs text-ink-muted">
      {notes.map((n) => (
        <span key={n} className="rounded border border-line px-1.5 py-px">
          {n}
        </span>
      ))}
      {def.deprecated && <span className="rounded border border-warning/40 px-1.5 py-px text-warning">Deprecated in {def.deprecated}</span>}
      {def.jamf && <span className="text-ink-faint">Jamf Pro: {def.jamf}</span>}
    </p>
  )
}

function PayloadCatalog({ defs, used, level, onPick }: { defs: PayloadDef[]; used: string[]; level?: 'device' | 'user'; onPick: (d: PayloadDef) => void }) {
  const [q, setQ] = useState('')
  const needle = q.trim().toLowerCase()
  const shown = defs.filter((d) => !needle || `${d.title} ${d.description} ${d.jamf ?? ''} ${d.payload_type}`.toLowerCase().includes(needle))
  const categories = [...new Set(shown.map((d) => d.category))]
  return (
    <div className="flex flex-col gap-5">
      <SearchInput value={q} onChange={setQ} placeholder={`Search ${defs.length} payloads…`} aria-label="Search payloads" autoFocus />
      {categories.length === 0 && <p className="text-sm text-ink-muted">No payloads match.</p>}
      {categories.map((c) => (
        <section key={c}>
          <h3 className="mb-1.5 text-xs font-medium uppercase tracking-wide text-ink-faint">{c}</h3>
          <div className="divide-y divide-line rounded-lg border border-line">
            {shown
              .filter((d) => d.category === c)
              .map((d) => {
                const taken = !d.multiple && used.includes(d.type)
                const offLevel = !!(level && d.channels?.length && !d.channels.includes(level))
                return (
                  <button
                    key={d.type}
                    type="button"
                    disabled={taken || offLevel}
                    onClick={() => onPick(d)}
                    className="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-surface-sunken disabled:cursor-default disabled:opacity-50 disabled:hover:bg-transparent"
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block text-sm font-medium text-ink">{d.title}</span>
                      <span className="block text-xs text-ink-muted">{d.description}</span>
                    </span>
                    {taken ? <Badge>Added</Badge> : offLevel ? <Badge>{level === 'user' ? 'Computer level only' : 'User level only'}</Badge> : d.supervised ? <Badge>Supervised</Badge> : null}
                  </button>
                )
              })}
          </div>
        </section>
      ))}
    </div>
  )
}

/** Folded-away preview of the generated .mobileconfig, built by the server on demand. */
function XmlPreview({ path, body }: { path: string; body: Record<string, unknown> }) {
  const [open, setOpen] = useState(false)
  const [preview, setPreview] = useState<ProfilePreview | null>(null)
  const json = JSON.stringify(body)
  useEffect(() => {
    if (!open) return
    const t = setTimeout(() => {
      api
        .post<ProfilePreview>(path, JSON.parse(json))
        .then(setPreview)
        .catch((e) => setPreview({ payload: '', error: errorMessage(e), payload_errors: [] }))
    }, 400)
    return () => clearTimeout(t)
  }, [open, json, path])
  return (
    <details className="rounded-lg border border-line" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}>
      <summary className="cursor-pointer select-none px-4 py-2.5 text-sm font-medium text-ink-muted hover:text-ink">Show profile XML</summary>
      {open && (
        <div className="px-4 pb-4">
          {preview?.error && <Alert tone="warning">{preview.error}</Alert>}
          {preview && preview.payload_errors.length > 0 && <Alert tone="warning">Some values are invalid. Fix them to see the XML.</Alert>}
          {preview?.payload && <CodeBlock text={preview.payload} filename="profile.mobileconfig" maxHeight="28rem" />}
        </div>
      )}
    </details>
  )
}

/** Jamf-style payload variables, filled in per device when the profile is sent. */
export const payloadVariables = ['$COMPUTERNAME', '$DEVICENAME', '$SERIALNUMBER', '$UDID', '$MACADDRESS', '$ASSETTAG', '$USERNAME', '$FULLNAME', '$EMAIL', '$PHONE', '$POSITION', '$DEPARTMENTNAME', '$BUILDINGNAME', '$ROOM', '$SITENAME']

/** A quiet, folded note listing the payload variables. */
export function VariablesHint() {
  return (
    <details className="text-xs text-ink-muted">
      <summary className="cursor-pointer select-none hover:text-ink">Payload variables</summary>
      <p className="mt-1.5 leading-relaxed">
        Text settings may contain variables that are filled in for each device when the profile is sent:{' '}
        {payloadVariables.map((v, i) => (
          <span key={v}>
            <code className="font-mono">{v}</code>
            {i < payloadVariables.length - 1 ? ', ' : '.'}
          </span>
        ))}{' '}
        Values come from the device's inventory and its User and Location details.
      </p>
    </details>
  )
}
