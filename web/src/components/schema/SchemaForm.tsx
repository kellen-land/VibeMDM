import { createContext, useContext, useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { AlertCircle, Eye, EyeOff, ExternalLink, FileUp, Info, Plus, Trash2, X } from 'lucide-react'
import type { SchemaFieldError, SchemaNode, SchemaValues } from '../../api/types'
import { Button } from '../ui/Button'
import { Input, Select } from '../ui/Input'
import { Toggle } from '../ui/Checkbox'
import { SearchInput } from '../ui/SearchInput'
import { Alert } from '../ui/Alert'
import { EmptyState } from '../ui/EmptyState'
import { cn } from '../../lib/cn'
import { formatBytes } from '../../lib/format'
import {
  alternatives,
  base64Bytes,
  errorsAt,
  errorsUnder,
  fileToBase64,
  isNullNode,
  isSet,
  joinPath,
  matchesAlt,
  nodeKind,
  orderedKeys,
  searchText,
} from './schema'

/** A payload elsewhere in the same profile that a "payload-ref" field (e.g. a Wi-Fi network's certificate) can point to. */
export interface PayloadRefTarget {
  uuid: string
  type: string
  label: string
}

/** Payload editors provide the profile's other payloads to reference fields. */
export const PayloadRefContext = createContext<PayloadRefTarget[]>([])

export interface SchemaFormProps {
  schema: SchemaNode
  value: SchemaValues
  onChange: (v: SchemaValues) => void
  errors?: SchemaFieldError[]
  disabled?: boolean
  /** Top-level keys rendered elsewhere on the page. */
  exclude?: string[]
  /** Search + "Show only configured" bar; defaults to on for forms with more than 8 fields. */
  toolbar?: boolean
  className?: string
}

/**
 * Renders a Jamf-style external application schema as a form. Every field starts "Not set";
 * unset keys are removed from the value object so only configured settings are sent.
 */
export function SchemaForm({ schema, value, onChange, errors, disabled, exclude, toolbar, className }: SchemaFormProps) {
  const [q, setQ] = useState('')
  const keys = useMemo(() => orderedKeys(schema).filter((k) => !exclude?.includes(k)), [schema, exclude])
  // Long forms that already have values open on what is configured.
  const [onlySet, setOnlySet] = useState(() => keys.length > 12 && keys.some((k) => isSet(value[k])))
  const index = useMemo(() => Object.fromEntries(keys.map((k) => [k, searchText(k, schema.properties![k])])), [keys, schema])
  const needle = q.trim().toLowerCase()
  const shown = keys.filter((k) => (!needle || index[k].includes(needle)) && (!onlySet || isSet(value[k])))
  const configured = keys.filter((k) => isSet(value[k])).length
  const showBar = toolbar ?? keys.length > 8
  const required = schema.required ?? []
  const errs = errors ?? []

  const setKey = (k: string, v: unknown) => {
    const next = { ...value }
    if (isSet(v)) next[k] = v
    else delete next[k]
    onChange(next)
  }

  return (
    <div className={cn('flex flex-col gap-4', className)}>
      {showBar && (
        <div className="flex flex-wrap items-center gap-3">
          <SearchInput value={q} onChange={setQ} placeholder={`Search ${keys.length} settings…`} className="w-full sm:w-72" aria-label="Search settings" />
          <Toggle size="sm" checked={onlySet} onChange={setOnlySet} label="Show only configured" />
          <span className="ml-auto text-xs text-ink-muted tabular-nums">
            {configured} of {keys.length} configured
          </span>
        </div>
      )}
      {errs.length > 0 && (
        <Alert tone="critical" title={`${errs.length} ${errs.length === 1 ? 'value needs' : 'values need'} attention`}>
          <ul className="space-y-0.5">
            {errs.map((e, i) => (
              <li key={i}>
                <span className="font-mono text-xs">{e.path || '(profile)'}</span> {e.message}
              </li>
            ))}
          </ul>
        </Alert>
      )}
      {shown.length === 0 ? (
        <EmptyState compact title={onlySet && !needle ? 'Nothing configured yet' : 'No settings match'} description={onlySet ? 'Turn off "Show only configured" to see every setting.' : 'Try a different search.'} />
      ) : (
        <div className="divide-y divide-line rounded-lg border border-line">
          {shown.map((k) => (
            <FieldRow
              key={k}
              name={k}
              node={schema.properties![k]}
              value={value[k]}
              onChange={(v) => setKey(k, v)}
              path={k}
              errors={errs}
              disabled={disabled}
              required={required.includes(k)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

interface RowProps {
  name: string
  node: SchemaNode
  value: unknown
  onChange: (v: unknown) => void
  path: string
  errors: SchemaFieldError[]
  disabled?: boolean
  required?: boolean
}

/** Is this field's editor wide (rendered below the label rather than beside it)? */
function isWide(node: SchemaNode, value: unknown): boolean {
  const alts = alternatives(node).filter((a) => !isNullNode(a))
  const n = alts.length === 1 ? { ...node, ...alts[0] } : node
  const k = nodeKind(n, value)
  if (k === 'object') return true
  if (k === 'array') {
    const ik = n.items ? nodeKind(n.items) : 'string'
    return ik === 'object' || ik === 'array'
  }
  return false
}

function displayValue(node: SchemaNode, v: unknown): string {
  if (node.enum && node.options?.enum_titles) {
    const i = node.enum.findIndex((e) => String(e) === String(v))
    if (i >= 0 && node.options.enum_titles[i]) return node.options.enum_titles[i]
  }
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

function FieldRow({ name, node, value, onChange, path, errors, disabled, required }: RowProps) {
  const set = isSet(value)
  const own = errorsAt(errors, path)
  const nested = errorsUnder(errors, path) - own.length
  const wide = isWide(node, value)
  const hasDefault = node.default !== undefined && node.default !== null
  const label = (
    <FieldLabel name={name} node={node} required={required} set={set} nestedErrors={nested} disabled={disabled} onClear={() => onChange(undefined)} />
  )
  const editor = (
    <div className="flex min-w-0 flex-col gap-1.5">
      <ValueEditor node={node} value={value} onChange={onChange} path={path} errors={errors} disabled={disabled} label={node.title || name} />
      {own.map((m) => (
        <FieldError key={m} message={m} />
      ))}
      {hasDefault && !set && (
        <p className="flex flex-wrap items-center gap-1.5 text-xs text-ink-faint">
          <span>
            Default: <span className="font-mono text-ink-muted">{displayValue(node, node.default)}</span>
          </span>
          {!disabled && (
            <button type="button" onClick={() => onChange(node.default)} className="text-accent-700 hover:underline dark:text-accent-300 focus-ring rounded">
              Use default
            </button>
          )}
        </p>
      )}
    </div>
  )
  return (
    <div className={cn('px-4 py-3.5', own.length > 0 && 'bg-critical/[0.03]')}>
      {wide ? (
        <div className="flex flex-col gap-3">
          {label}
          {editor}
        </div>
      ) : (
        <div className="grid gap-x-6 gap-y-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)] md:items-start">
          {label}
          {editor}
        </div>
      )}
    </div>
  )
}

function FieldLabel({
  name,
  node,
  required,
  set,
  nestedErrors,
  disabled,
  onClear,
}: {
  name: string
  node: SchemaNode
  required?: boolean
  set: boolean
  nestedErrors: number
  disabled?: boolean
  onClear: () => void
}) {
  const info = node.options?.infoText
  return (
    <div className="min-w-0">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span className="text-[13px] font-medium text-ink">
          {node.title || name}
          {required && (
            <span className="ml-0.5 text-critical" aria-label="required">
              *
            </span>
          )}
        </span>
        {node.title && node.title !== name && <code className="rounded bg-surface-sunken px-1 py-px font-mono text-[11px] text-ink-muted">{name}</code>}
        <FieldNotes node={node} />
        {info && (
          <span className="inline-flex cursor-help text-ink-faint hover:text-ink-muted" title={info} aria-label={info} tabIndex={0} role="note">
            <Info className="size-3.5" aria-hidden="true" />
          </span>
        )}
        {set ? (
          !disabled && (
            <button type="button" onClick={onClear} className="inline-flex items-center gap-0.5 rounded text-[11px] text-ink-faint hover:text-ink focus-ring" title="Reset to not set">
              <X className="size-3" aria-hidden="true" />
              Clear
            </button>
          )
        ) : (
          <span className="text-[11px] text-ink-faint">Not set</span>
        )}
        {nestedErrors > 0 && (
          <span className="inline-flex items-center gap-1 text-[11px] text-critical">
            <AlertCircle className="size-3" aria-hidden="true" />
            {nestedErrors} {nestedErrors === 1 ? 'problem' : 'problems'} inside
          </span>
        )}
      </div>
      {node.description && <Description text={node.description} />}
      {node.links && node.links.length > 0 && (
        <p className="mt-1 flex flex-wrap gap-x-3 text-xs">
          {node.links
            .filter((l) => l && typeof l.href === 'string' && /^https?:\/\//i.test(l.href))
            .map((l, i) => (
              <a key={i} href={l.href} target="_blank" rel="noreferrer noopener" title={l.href} className="inline-flex items-center gap-1 text-accent-700 hover:underline dark:text-accent-300">
                More info
                <ExternalLink className="size-3" aria-hidden="true" />
              </a>
            ))}
        </p>
      )}
    </div>
  )
}

/** Quiet notes from Apple's schema: supervision, the OS version a key needs, deprecation, level. */
function FieldNotes({ node }: { node: SchemaNode }) {
  const notes = [
    node['x-supervised'] && 'Supervised',
    node['x-os'],
    node['x-channel'] === 'user' && 'User level only',
    node['x-channel'] === 'device' && 'Computer level only',
  ].filter(Boolean) as string[]
  if (!notes.length && !node['x-deprecated']) return null
  return (
    <>
      {notes.map((n) => (
        <span key={n} className="rounded border border-line px-1 py-px text-[10px] leading-tight text-ink-muted">
          {n}
        </span>
      ))}
      {node['x-deprecated'] && (
        <span className="rounded border border-warning/40 px-1 py-px text-[10px] leading-tight text-warning" title={`Deprecated by Apple in ${node['x-deprecated']}`}>
          Deprecated
        </span>
      )}
    </>
  )
}

/** Descriptions longer than two lines are clamped, with "More" to read the rest. */
function Description({ text }: { text: string }) {
  const [open, setOpen] = useState(false)
  const long = text.length > 180 || text.split('\n').length > 2
  return (
    <div className="mt-1 text-xs leading-relaxed text-ink-muted">
      <p className={cn('whitespace-pre-line', long && !open && 'line-clamp-2')}>{text}</p>
      {long && (
        <button type="button" onClick={() => setOpen(!open)} className="rounded text-[11px] text-accent-700 hover:underline focus-ring dark:text-accent-300">
          {open ? 'Less' : 'More'}
        </button>
      )}
    </div>
  )
}

function FieldError({ message }: { message: string }) {
  return (
    <p className="flex items-start gap-1 text-xs text-critical">
      <AlertCircle className="mt-px size-3.5 shrink-0" aria-hidden="true" />
      <span>{message}</span>
    </p>
  )
}

interface EditorProps {
  node: SchemaNode
  value: unknown
  onChange: (v: unknown) => void
  path: string
  errors: SchemaFieldError[]
  disabled?: boolean
  label: string
  /** Inside a configured anyOf branch or a list item: booleans need no "Not set" choice. */
  noUnset?: boolean
}

function ValueEditor(props: EditorProps) {
  const { node } = props
  if (alternatives(node).length > 0) return <AltEditor {...props} />
  if (node.enum && node.enum.length > 0) return <EnumEditor {...props} />
  switch (nodeKind(node, props.value)) {
    case 'boolean':
      return <TriState value={typeof props.value === 'boolean' ? props.value : undefined} onChange={props.onChange} disabled={props.disabled} label={props.label} noUnset={props.noUnset} />
    case 'integer':
    case 'number':
      return <NumberEditor {...props} />
    case 'array':
      return <ArrayEditor {...props} />
    case 'object': {
      const add = typeof node.additionalProperties === 'object' ? node.additionalProperties : undefined
      if (add) return <MapEditor {...props} entry={add} />
      return node.properties && Object.keys(node.properties).length > 0 ? <ObjectEditor {...props} /> : <KeyValueEditor {...props} />
    }
    default:
      if (node.format === 'data') return <DataEditor {...props} />
      if (node.format === 'payload-ref') return <RefEditor {...props} />
      if (node.format === 'date-time') return <DateEditor {...props} />
      return <StringEditor {...props} />
  }
}

/** anyOf/oneOf, including the [{title:"Not Configured", type:"null"}, {title:"Configured", ...}] idiom. */
function AltEditor({ node, value, onChange, path, errors, disabled, label }: EditorProps) {
  const alts = alternatives(node)
  const real = alts.map((a, i) => ({ a, i })).filter((x) => !isNullNode(x.a))
  const nullAlt = alts.find(isNullNode)
  const match = () => (value === undefined || value === null ? -1 : (real.find((x) => matchesAlt(x.a, value))?.i ?? real[0]?.i ?? -1))
  const [chosen, setChosen] = useState<number>(match)
  useEffect(() => {
    if (value !== undefined && value !== null && !(chosen >= 0 && matchesAlt(alts[chosen], value))) setChosen(match())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])
  const { anyOf: _a, oneOf: _o, ...base } = node
  void _a
  void _o
  const branch: SchemaNode | null = chosen >= 0 ? { ...base, ...alts[chosen], title: undefined, description: undefined } : null
  const branchHelp = chosen >= 0 ? alts[chosen].description : undefined
  return (
    <div className="flex flex-col gap-2">
      <Select
        value={String(chosen)}
        disabled={disabled}
        aria-label={`${label}: configuration`}
        onChange={(e) => {
          const i = Number(e.target.value)
          setChosen(i)
          if (i < 0 || (value !== undefined && !matchesAlt(alts[i], value))) onChange(undefined)
        }}
      >
        <option value="-1">{nullAlt?.title || (nullAlt ? 'Not configured' : 'Not set')}</option>
        {real.map(({ a, i }) => (
          <option key={i} value={i}>
            {a.title || (real.length === 1 ? 'Configured' : nodeKind(a))}
          </option>
        ))}
      </Select>
      {branch && (
        <div className="border-l-2 border-line pl-3">
          {branchHelp && <p className="mb-1.5 text-xs text-ink-muted">{branchHelp}</p>}
          <ValueEditor node={branch} value={value} onChange={onChange} path={path} errors={errors} disabled={disabled} label={label} noUnset />
        </div>
      )}
    </div>
  )
}

function EnumEditor({ node, value, onChange, disabled, label }: EditorProps) {
  const opts = node.enum!.filter((e) => e !== null)
  const titles = node.options?.enum_titles ?? []
  const idx = value === undefined ? -1 : opts.findIndex((e) => String(e) === String(value))
  return (
    <Select value={String(idx)} disabled={disabled} aria-label={label} onChange={(e) => onChange(Number(e.target.value) < 0 ? undefined : opts[Number(e.target.value)])}>
      <option value="-1">Not set</option>
      {opts.map((o, i) => {
        const t = titles[node.enum!.indexOf(o)] ?? titles[i]
        return (
          <option key={i} value={i}>
            {t ? (String(o) === t ? t : `${t} (${String(o)})`) : String(o)}
          </option>
        )
      })}
      {idx < 0 && value !== undefined && <option value="-2">{String(value)} (not allowed)</option>}
    </Select>
  )
}

/** Not set / True / False. Text labels carry the state; the active option is also raised. */
export function TriState({ value, onChange, disabled, label, noUnset, labels = ['True', 'False'] }: { value: boolean | undefined; onChange: (v: boolean | undefined) => void; disabled?: boolean; label: string; noUnset?: boolean; labels?: [string, string] }) {
  const opts: { v: boolean | undefined; t: string }[] = [...(noUnset ? [] : [{ v: undefined, t: 'Not set' }]), { v: true, t: labels[0] }, { v: false, t: labels[1] }]
  return (
    <div role="radiogroup" aria-label={label} className="inline-flex w-fit rounded-md border border-line bg-surface-sunken p-0.5">
      {opts.map((o) => {
        const active = value === o.v
        return (
          <button
            key={o.t}
            type="button"
            role="radio"
            aria-checked={active}
            disabled={disabled}
            onClick={() => onChange(o.v)}
            className={cn(
              'min-w-16 rounded px-2.5 py-1 text-xs font-medium transition-colors focus-ring disabled:cursor-not-allowed',
              active ? 'bg-surface-raised text-ink shadow-sm ring-1 ring-line-strong/70' : 'text-ink-muted hover:text-ink',
            )}
          >
            {o.t}
          </button>
        )
      })}
    </div>
  )
}

function rangeHint(node: SchemaNode, unit = ''): string {
  const parts: string[] = []
  if (node.minimum !== undefined && node.maximum !== undefined) parts.push(`${node.minimum}–${node.maximum}${unit}`)
  else if (node.minimum !== undefined) parts.push(`at least ${node.minimum}${unit}`)
  else if (node.maximum !== undefined) parts.push(`at most ${node.maximum}${unit}`)
  return parts.join(' · ')
}

function NumberEditor({ node, value, onChange, disabled, label }: EditorProps) {
  const integer = nodeKind(node, value) === 'integer'
  const hint = [integer ? 'Whole number' : 'Number', rangeHint(node)].filter(Boolean).join(' · ')
  return (
    <div className="flex flex-col gap-1">
      <Input
        type="number"
        inputMode={integer ? 'numeric' : 'decimal'}
        step={integer ? 1 : 'any'}
        min={node.minimum}
        max={node.maximum}
        value={value === undefined || value === null ? '' : String(value)}
        disabled={disabled}
        aria-label={label}
        placeholder={node.options?.inputAttributes?.placeholder ?? 'Not set'}
        className="max-w-48"
        onChange={(e) => {
          const raw = e.target.value
          if (raw.trim() === '') return onChange(undefined)
          const n = Number(raw)
          onChange(Number.isFinite(n) ? n : raw)
        }}
      />
      <p className="text-[11px] text-ink-faint">{hint}</p>
    </div>
  )
}

/** format "payload-ref": pick another payload in this profile (its PayloadUUID is stored). */
function RefEditor({ node, value, onChange, disabled, label }: EditorProps) {
  const targets = useContext(PayloadRefContext).filter((t) => !node['x-ref-types']?.length || node['x-ref-types'].includes(t.type))
  const v = typeof value === 'string' ? value : ''
  const known = targets.some((t) => t.uuid.toUpperCase() === v.toUpperCase())
  return (
    <div className="flex flex-col gap-1">
      <Select value={known ? targets.find((t) => t.uuid.toUpperCase() === v.toUpperCase())!.uuid : v ? '__missing' : ''} disabled={disabled} aria-label={label} onChange={(e) => onChange(e.target.value || undefined)}>
        <option value="">Not set</option>
        {targets.map((t) => (
          <option key={t.uuid} value={t.uuid}>
            {t.label}
          </option>
        ))}
        {v && !known && <option value="__missing">Removed payload</option>}
      </Select>
      {targets.length === 0 && <p className="text-[11px] text-ink-faint">Add a certificate payload (Certificate, SCEP or ACME) to this profile first.</p>}
    </div>
  )
}

/** format "date-time": a local date and time, sent as an ISO timestamp. */
function DateEditor({ value, onChange, disabled, label }: EditorProps) {
  const v = typeof value === 'string' ? value.slice(0, 16) : ''
  return (
    <Input
      type="datetime-local"
      value={v}
      disabled={disabled}
      aria-label={label}
      className="max-w-60"
      onChange={(e) => onChange(e.target.value ? new Date(e.target.value).toISOString() : undefined)}
    />
  )
}

function StringEditor({ node, value, onChange, disabled, label }: EditorProps) {
  const [reveal, setReveal] = useState(false)
  const secret = !!node['x-secret']
  const hints: string[] = []
  if (node.minLength !== undefined && node.maxLength !== undefined) hints.push(`${node.minLength}–${node.maxLength} characters`)
  else if (node.minLength !== undefined) hints.push(`at least ${node.minLength} characters`)
  else if (node.maxLength !== undefined) hints.push(`at most ${node.maxLength} characters`)
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-1.5">
        <Input
          type={secret && !reveal ? 'password' : 'text'}
          autoComplete={secret ? 'new-password' : undefined}
          value={typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value)}
          disabled={disabled}
          aria-label={label}
          maxLength={node.maxLength}
          placeholder={node.options?.inputAttributes?.placeholder ?? 'Not set'}
          spellCheck={false}
          onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
        />
        {secret && !disabled && (
          <button type="button" onClick={() => setReveal(!reveal)} className="inline-flex size-8 shrink-0 items-center justify-center rounded-md text-ink-faint hover:bg-line/40 hover:text-ink focus-ring" aria-label={reveal ? 'Hide' : 'Show'}>
            {reveal ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
          </button>
        )}
      </div>
      {(hints.length > 0 || node.pattern) && (
        <p className="text-[11px] text-ink-faint">
          {hints.join(' · ')}
          {node.pattern && (
            <>
              {hints.length > 0 && ' · '}
              Format <code className="font-mono">{node.pattern}</code>
            </>
          )}
        </p>
      )}
    </div>
  )
}

/** format: "data" — pick a file, store it base64-encoded. */
function DataEditor({ value, onChange, disabled, label }: EditorProps) {
  const ref = useRef<HTMLInputElement>(null)
  const [name, setName] = useState('')
  const [err, setErr] = useState('')
  const b64 = typeof value === 'string' ? value : ''
  const onFile = async (f: File | undefined) => {
    if (!f) return
    setErr('')
    try {
      onChange(await fileToBase64(f))
      setName(f.name)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      if (ref.current) ref.current.value = ''
    }
  }
  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-2">
        <input ref={ref} type="file" className="hidden" onChange={(e) => onFile(e.target.files?.[0])} aria-label={`${label}: file`} />
        {!disabled && (
          <Button size="sm" icon={<FileUp />} onClick={() => ref.current?.click()}>
            {b64 ? 'Replace file…' : 'Choose file…'}
          </Button>
        )}
        {b64 ? (
          <>
            <span className="text-xs text-ink-muted">
              {name ? <span className="text-ink">{name}</span> : 'File loaded'} · {formatBytes(base64Bytes(b64))}
            </span>
            {!disabled && (
              <Button
                size="xs"
                variant="ghost"
                icon={<X />}
                onClick={() => {
                  onChange(undefined)
                  setName('')
                }}
              >
                Clear
              </Button>
            )}
          </>
        ) : (
          <span className="text-xs text-ink-faint">No file</span>
        )}
      </div>
      <p className="text-[11px] text-ink-faint">The file is base64-encoded and delivered as plist data.</p>
      {err && <FieldError message={err} />}
    </div>
  )
}

function ObjectEditor({ node, value, onChange, path, errors, disabled }: EditorProps) {
  const obj = value && typeof value === 'object' && !Array.isArray(value) ? (value as SchemaValues) : {}
  const required = node.required ?? []
  const keys = orderedKeys(node)
  // Big groups (EAP settings, IKEv2 parameters, …) stay folded until something in them is set.
  const [open, setOpen] = useState(() => keys.length <= 4 || Object.keys(obj).length > 0 || errorsUnder(errors, path) > 0)
  if (!open)
    return (
      <div>
        <Button size="sm" variant="ghost" onClick={() => setOpen(true)}>
          Show {keys.length} settings
        </Button>
      </div>
    )
  const setChild = (k: string, v: unknown) => {
    const next = { ...obj }
    if (v === undefined || (typeof v === 'string' && v === '')) delete next[k]
    else next[k] = v
    onChange(Object.keys(next).length ? next : undefined)
  }
  return (
    <fieldset className="divide-y divide-line rounded-lg border border-line bg-surface-sunken/30">
      {orderedKeys(node).map((k) => (
        <FieldRow key={k} name={k} node={node.properties![k]} value={obj[k]} onChange={(v) => setChild(k, v)} path={joinPath(path, k)} errors={errors} disabled={disabled} required={required.includes(k)} />
      ))}
    </fieldset>
  )
}

function listHint(node: SchemaNode): string {
  if (node.minItems !== undefined && node.maxItems !== undefined) return `${node.minItems}–${node.maxItems} items`
  if (node.minItems !== undefined) return `At least ${node.minItems} ${node.minItems === 1 ? 'item' : 'items'}`
  if (node.maxItems !== undefined) return `At most ${node.maxItems} ${node.maxItems === 1 ? 'item' : 'items'}`
  return ''
}

function ArrayEditor(props: EditorProps) {
  const items = props.node.items ?? { type: 'string' }
  const ik = alternatives(items).length ? 'complex' : nodeKind(items)
  if (ik === 'string' || ik === 'integer' || ik === 'number') {
    if (items.format !== 'data') return <TagListEditor {...props} items={items} />
  }
  return <RepeatEditor {...props} items={items} />
}

/** Arrays of strings/numbers as removable chips plus an add box (Enter, comma or paste lines). */
function TagListEditor({ node, value, onChange, path, errors, disabled, label, items }: EditorProps & { items: SchemaNode }) {
  const list = Array.isArray(value) ? value : []
  const [draft, setDraft] = useState('')
  const numeric = nodeKind(items) !== 'string'
  const full = node.maxItems !== undefined && list.length >= node.maxItems
  const emit = (l: unknown[]) => onChange(l.length ? l : undefined)
  const add = (raw: string) => {
    const parts = raw
      .split(/[\n,]/)
      .map((s) => s.trim())
      .filter(Boolean)
    if (!parts.length) return
    emit([...list, ...parts.map((p) => (numeric && Number.isFinite(Number(p)) ? Number(p) : p))])
    setDraft('')
  }
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      add(draft)
    } else if (e.key === 'Backspace' && !draft && list.length) {
      emit(list.slice(0, -1))
    }
  }
  const hint = listHint(node)
  return (
    <div className="flex flex-col gap-1.5">
      {list.length > 0 && (
        <ul className="flex flex-wrap gap-1.5" aria-label={`${label}: items`}>
          {list.map((v, i) => {
            const bad = errorsAt(errors, `${path}[${i}]`)
            return (
              <li
                key={i}
                className={cn('inline-flex max-w-full items-center gap-1 rounded-md border px-1.5 py-0.5 font-mono text-xs', bad.length ? 'border-critical/50 text-critical' : 'border-line-strong/70 bg-surface-raised text-ink')}
                title={bad.join('; ') || undefined}
              >
                {bad.length > 0 && <AlertCircle className="size-3 shrink-0" aria-label="invalid" />}
                <span className="truncate">{items.enum && items.options?.enum_titles ? displayValue(items, v) : String(v)}</span>
                {!disabled && (
                  <button type="button" onClick={() => emit(list.filter((_, j) => j !== i))} className="text-ink-faint hover:text-ink" aria-label={`Remove ${String(v)}`}>
                    <X className="size-3" />
                  </button>
                )}
              </li>
            )
          })}
        </ul>
      )}
      {!disabled &&
        !full &&
        (items.enum && items.enum.length ? (
          <Select
            value=""
            aria-label={`${label}: add item`}
            onChange={(e) => {
              const i = Number(e.target.value)
              if (e.target.value !== '') emit([...list, items.enum![i]])
            }}
          >
            <option value="">Add…</option>
            {items.enum.map((o, i) => (
              <option key={i} value={i} disabled={list.some((x) => String(x) === String(o))}>
                {items.options?.enum_titles?.[i] ?? String(o)}
              </option>
            ))}
          </Select>
        ) : (
          <Input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKey}
            onBlur={() => add(draft)}
            onPaste={(e) => {
              const t = e.clipboardData.getData('text')
              if (/[\n,]/.test(t)) {
                e.preventDefault()
                add(draft + t)
              }
            }}
            placeholder={items.options?.inputAttributes?.placeholder ?? (list.length ? 'Add another…' : 'Type a value and press Enter')}
            aria-label={`${label}: add item`}
            inputMode={numeric ? 'decimal' : undefined}
            spellCheck={false}
          />
        ))}
      {(hint || list.length > 0) && (
        <p className="text-[11px] text-ink-faint">
          {[list.length > 0 && `${list.length} ${list.length === 1 ? 'item' : 'items'}`, hint].filter(Boolean).join(' · ')}
        </p>
      )}
    </div>
  )
}

function emptyItem(items: SchemaNode): unknown {
  if (items.default !== undefined) return items.default
  switch (nodeKind(items)) {
    case 'object':
      return {}
    case 'array':
      return []
    case 'boolean':
      return false
    default:
      return undefined
  }
}

/** Arrays of objects (or other complex items) as repeatable groups. */
function RepeatEditor({ node, value, onChange, path, errors, disabled, label, items }: EditorProps & { items: SchemaNode }) {
  const list = Array.isArray(value) ? value : []
  const emit = (l: unknown[]) => onChange(l.length ? l : undefined)
  const full = node.maxItems !== undefined && list.length >= node.maxItems
  const noun = items.title || 'Item'
  const hint = listHint(node)
  return (
    <div className="flex flex-col gap-2">
      {list.map((v, i) => (
        <div key={i} className="rounded-lg border border-line bg-surface-sunken/30">
          <div className="flex items-center justify-between gap-2 border-b border-line px-3 py-1.5">
            <span className="micro-label">
              {noun} {i + 1}
            </span>
            {errorsUnder(errors, `${path}[${i}]`) > 0 && (
              <span className="mr-auto inline-flex items-center gap-1 text-[11px] text-critical">
                <AlertCircle className="size-3" aria-hidden="true" /> Needs attention
              </span>
            )}
            {!disabled && (
              <Button size="xs" variant="ghost" icon={<Trash2 />} onClick={() => emit(list.filter((_, j) => j !== i))} aria-label={`Remove ${noun.toLowerCase()} ${i + 1}`}>
                Remove
              </Button>
            )}
          </div>
          <div className={cn(nodeKind(items) === 'object' && items.properties ? '' : 'p-3')}>
            {nodeKind(items) === 'object' && items.properties ? (
              <ObjectItem node={items} value={v} onChange={(nv) => emit(list.map((x, j) => (j === i ? (nv ?? {}) : x)))} path={`${path}[${i}]`} errors={errors} disabled={disabled} />
            ) : (
              <>
                <ValueEditor node={items} value={v} onChange={(nv) => emit(list.map((x, j) => (j === i ? nv : x)))} path={`${path}[${i}]`} errors={errors} disabled={disabled} label={`${label} ${i + 1}`} noUnset />
                {errorsAt(errors, `${path}[${i}]`).map((m) => (
                  <FieldError key={m} message={m} />
                ))}
              </>
            )}
          </div>
        </div>
      ))}
      <div className="flex items-center gap-3">
        {!disabled && !full && (
          <Button size="sm" icon={<Plus />} onClick={() => emit([...list, emptyItem(items)])}>
            Add {noun.toLowerCase()}
          </Button>
        )}
        {(hint || list.length === 0) && <span className="text-[11px] text-ink-faint">{[list.length === 0 && 'No items yet', hint].filter(Boolean).join(' · ')}</span>}
      </div>
    </div>
  )
}

/** An object inside a list: like ObjectEditor but keeps an empty object rather than unsetting it. */
function ObjectItem({ node, value, onChange, path, errors, disabled }: { node: SchemaNode; value: unknown; onChange: (v: SchemaValues | undefined) => void; path: string; errors: SchemaFieldError[]; disabled?: boolean }) {
  const obj = value && typeof value === 'object' && !Array.isArray(value) ? (value as SchemaValues) : {}
  const required = node.required ?? []
  return (
    <div className="divide-y divide-line">
      {orderedKeys(node).map((k) => (
        <FieldRow
          key={k}
          name={k}
          node={node.properties![k]}
          value={obj[k]}
          onChange={(v) => {
            const next = { ...obj }
            if (v === undefined) delete next[k]
            else next[k] = v
            onChange(next)
          }}
          path={joinPath(path, k)}
          errors={errors}
          disabled={disabled}
          required={required.includes(k)}
        />
      ))}
    </div>
  )
}

type KVType = 'string' | 'number' | 'boolean'
interface KVRow {
  id: number
  key: string
  type: KVType
  value: string
}

function toRows(v: unknown, next: () => number): KVRow[] {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return []
  return Object.entries(v as SchemaValues).map(([key, x]) => ({
    id: next(),
    key,
    type: typeof x === 'number' ? 'number' : typeof x === 'boolean' ? 'boolean' : 'string',
    value: typeof x === 'string' ? x : typeof x === 'object' ? JSON.stringify(x) : String(x),
  }))
}

/** Free-form object (no properties): key / type / value rows. */
function KeyValueEditor({ value, onChange, disabled, label }: EditorProps) {
  const counter = useRef(0)
  const next = () => ++counter.current
  const [rows, setRows] = useState<KVRow[]>(() => toRows(value, next))
  const baseId = useId()
  useEffect(() => {
    if (value === undefined) setRows((r) => (r.some((x) => x.key) ? [] : r))
  }, [value])
  const emit = (rs: KVRow[]) => {
    setRows(rs)
    const out: SchemaValues = {}
    for (const r of rs) {
      if (!r.key.trim()) continue
      out[r.key.trim()] = r.type === 'number' ? (Number.isFinite(Number(r.value)) && r.value.trim() !== '' ? Number(r.value) : r.value) : r.type === 'boolean' ? r.value === 'true' : r.value
    }
    onChange(Object.keys(out).length ? out : undefined)
  }
  const patch = (id: number, p: Partial<KVRow>) => emit(rows.map((r) => (r.id === id ? { ...r, ...p } : r)))
  return (
    <div className="flex flex-col gap-2">
      {rows.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <div className="hidden grid-cols-[minmax(0,1fr)_7rem_minmax(0,1.3fr)_2rem] gap-2 sm:grid">
            <span className="micro-label">Key</span>
            <span className="micro-label">Type</span>
            <span className="micro-label">Value</span>
          </div>
          {rows.map((r, i) => (
            <div key={r.id} className="grid grid-cols-[minmax(0,1fr)_7rem_minmax(0,1.3fr)_2rem] items-center gap-2">
              <Input mono value={r.key} onChange={(e) => patch(r.id, { key: e.target.value })} disabled={disabled} placeholder="Key" aria-label={`${label}: key ${i + 1}`} id={`${baseId}-k${r.id}`} />
              <Select value={r.type} onChange={(e) => patch(r.id, { type: e.target.value as KVType, value: e.target.value === 'boolean' ? 'true' : r.value })} disabled={disabled} aria-label={`${label}: type ${i + 1}`}>
                <option value="string">Text</option>
                <option value="number">Number</option>
                <option value="boolean">Boolean</option>
              </Select>
              {r.type === 'boolean' ? (
                <Select value={r.value === 'true' ? 'true' : 'false'} onChange={(e) => patch(r.id, { value: e.target.value })} disabled={disabled} aria-label={`${label}: value ${i + 1}`}>
                  <option value="true">True</option>
                  <option value="false">False</option>
                </Select>
              ) : (
                <Input value={r.value} onChange={(e) => patch(r.id, { value: e.target.value })} disabled={disabled} inputMode={r.type === 'number' ? 'decimal' : undefined} placeholder="Value" aria-label={`${label}: value ${i + 1}`} />
              )}
              {!disabled ? (
                <button type="button" onClick={() => emit(rows.filter((x) => x.id !== r.id))} className="inline-flex size-8 items-center justify-center rounded-md text-ink-faint hover:bg-line/40 hover:text-ink focus-ring" aria-label={`Remove key ${r.key || i + 1}`}>
                  <Trash2 className="size-4" />
                </button>
              ) : (
                <span />
              )}
            </div>
          ))}
        </div>
      )}
      {!disabled ? (
        <div>
          <Button size="sm" icon={<Plus />} onClick={() => setRows([...rows, { id: next(), key: '', type: 'string', value: '' }])}>
            Add key
          </Button>
        </div>
      ) : (
        rows.length === 0 && <span className="text-xs text-ink-faint">No keys</span>
      )}
    </div>
  )
}

/** Objects keyed by names you choose (team IDs, bundle IDs, domains), each value edited by the entry schema. */
function MapEditor({ node, value, onChange, path, errors, disabled, label, entry }: EditorProps & { entry: SchemaNode }) {
  const obj = value && typeof value === 'object' && !Array.isArray(value) ? (value as SchemaValues) : {}
  const [draft, setDraft] = useState('')
  const keys = Object.keys(obj)
  const emit = (next: SchemaValues) => onChange(Object.keys(next).length ? next : undefined)
  const add = () => {
    const k = draft.trim()
    if (!k || k in obj) return
    emit({ ...obj, [k]: emptyItem(entry) ?? (nodeKind(entry) === 'string' ? '' : undefined) })
    setDraft('')
  }
  const keyName = entry.title && entry.title !== node.title ? entry.title : 'Key'
  return (
    <div className="flex flex-col gap-2">
      {keys.map((k) => (
        <div key={k} className="rounded-lg border border-line bg-surface-sunken/30">
          <div className="flex items-center justify-between gap-2 border-b border-line px-3 py-1.5">
            <code className="truncate font-mono text-xs text-ink">{k}</code>
            {!disabled && (
              <Button
                size="xs"
                variant="ghost"
                icon={<Trash2 />}
                onClick={() => {
                  const next = { ...obj }
                  delete next[k]
                  emit(next)
                }}
                aria-label={`Remove ${k}`}
              >
                Remove
              </Button>
            )}
          </div>
          <div className={cn(nodeKind(entry) === 'object' && entry.properties ? '' : 'p-3')}>
            {nodeKind(entry) === 'object' && entry.properties ? (
              <ObjectItem node={entry} value={obj[k]} onChange={(nv) => emit({ ...obj, [k]: nv ?? {} })} path={joinPath(path, k)} errors={errors} disabled={disabled} />
            ) : (
              <ValueEditor node={entry} value={obj[k]} onChange={(nv) => emit({ ...obj, [k]: nv })} path={joinPath(path, k)} errors={errors} disabled={disabled} label={`${label} ${k}`} noUnset />
            )}
          </div>
        </div>
      ))}
      {!disabled && (
        <div className="flex items-center gap-2">
          <Input value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), add())} placeholder={`${keyName}, for example a team ID`} aria-label={`${label}: new key`} className="max-w-72" spellCheck={false} />
          <Button size="sm" icon={<Plus />} onClick={add} disabled={!draft.trim()}>
            Add
          </Button>
        </div>
      )}
      {keys.length === 0 && disabled && <span className="text-xs text-ink-faint">None</span>}
    </div>
  )
}
