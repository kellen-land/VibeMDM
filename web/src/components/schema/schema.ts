import type { SchemaFieldError, SchemaNode, SchemaValues } from '../../api/types'

/** Primitive kinds the form knows how to edit. */
export type Kind = 'string' | 'integer' | 'number' | 'boolean' | 'array' | 'object'

const kinds: Kind[] = ['string', 'integer', 'number', 'boolean', 'array', 'object']

export function declaredTypes(n: SchemaNode): string[] {
  if (typeof n.type === 'string') return [n.type]
  if (Array.isArray(n.type)) return n.type.filter((t): t is string => typeof t === 'string')
  return []
}

export function isNullNode(n: SchemaNode): boolean {
  const t = declaredTypes(n)
  return t.length === 1 && t[0] === 'null'
}

export function valueKind(v: unknown): Kind | null {
  if (v === undefined || v === null) return null
  if (typeof v === 'boolean') return 'boolean'
  if (typeof v === 'number') return Number.isInteger(v) ? 'integer' : 'number'
  if (Array.isArray(v)) return 'array'
  if (typeof v === 'object') return 'object'
  return 'string'
}

/** The editor kind for a node, preferring the current value's kind when the node allows several. */
export function nodeKind(n: SchemaNode, value?: unknown): Kind {
  const ts = declaredTypes(n).filter((t) => t !== 'null') as Kind[]
  const allowed = ts.filter((t) => kinds.includes(t))
  if (allowed.length > 1) {
    const vk = valueKind(value)
    if (vk && allowed.includes(vk)) return vk
    if (vk === 'integer' && allowed.includes('number')) return 'number'
  }
  if (allowed.length) return allowed[0]
  if (n.properties && Object.keys(n.properties).length) return 'object'
  if (n.items) return 'array'
  if (n.enum?.length) {
    const vk = valueKind(n.enum.find((e) => e !== null))
    if (vk) return vk === 'integer' ? 'integer' : vk
  }
  return 'string'
}

/** Alternatives of an anyOf/oneOf node (null alternatives included). */
export function alternatives(n: SchemaNode): SchemaNode[] {
  return [...(n.anyOf ?? []), ...(n.oneOf ?? [])].filter(Boolean)
}

/** Does a value plausibly belong to an alternative? Used to pick the active branch. */
export function matchesAlt(alt: SchemaNode, v: unknown): boolean {
  if (isNullNode(alt)) return v === undefined || v === null
  if (alt.enum?.length) return alt.enum.some((e) => String(e) === String(v))
  const vk = valueKind(v)
  if (!vk) return false
  const ts = declaredTypes(alt)
  if (!ts.length) return nodeKind(alt) === vk || (vk === 'integer' && nodeKind(alt) === 'number')
  return ts.includes(vk) || (vk === 'integer' && ts.includes('number'))
}

export function orderOf(n: SchemaNode): number {
  return n.property_order ?? n.propertyOrder ?? Number.MAX_SAFE_INTEGER
}

/** Property keys sorted by property_order/propertyOrder, then by key (mirrors schema.OrderedKeys). */
export function orderedKeys(n: SchemaNode): string[] {
  const props = n.properties ?? {}
  return Object.keys(props).sort((a, b) => {
    const d = orderOf(props[a]) - orderOf(props[b])
    return d !== 0 ? d : a < b ? -1 : a > b ? 1 : 0
  })
}

export function joinPath(path: string, key: string): string {
  return path ? `${path}.${key}` : key
}

export function isSet(v: unknown): boolean {
  if (v === undefined || v === null) return false
  if (typeof v === 'string') return v !== ''
  if (Array.isArray(v)) return v.length > 0
  if (typeof v === 'object') return Object.keys(v as object).length > 0
  return true
}

/** Drop unset keys recursively so only configured values are sent. Array items are kept as-is (minus unset object keys). */
export function pruneValues(v: SchemaValues): SchemaValues {
  const out: SchemaValues = {}
  for (const [k, x] of Object.entries(v)) {
    const c = pruneValue(x)
    if (isSet(c)) out[k] = c
  }
  return out
}

function pruneValue(v: unknown): unknown {
  if (Array.isArray(v)) return v.map((x) => (x && typeof x === 'object' && !Array.isArray(x) ? pruneValues(x as SchemaValues) : x))
  if (v && typeof v === 'object') return pruneValues(v as SchemaValues)
  return v
}

/** Errors at exactly this path. */
export function errorsAt(errors: SchemaFieldError[] | undefined, path: string): string[] {
  return (errors ?? []).filter((e) => e.path === path).map((e) => e.message)
}

/** Errors at this path or anywhere below it. */
export function errorsUnder(errors: SchemaFieldError[] | undefined, path: string): number {
  return (errors ?? []).filter((e) => e.path === path || e.path.startsWith(path + '.') || e.path.startsWith(path + '[')).length
}

/** Flattened lowercase text for search: keys, titles and descriptions, including nested properties. */
export function searchText(key: string, n: SchemaNode, depth = 0): string {
  const parts = [key, n.title ?? '', n.description ?? '', n.options?.infoText ?? '']
  if (depth < 4) {
    for (const [k, c] of Object.entries(n.properties ?? {})) parts.push(searchText(k, c, depth + 1))
    if (n.items) parts.push(searchText('', n.items, depth + 1))
    for (const a of alternatives(n)) parts.push(searchText('', a, depth + 1))
  }
  return parts.join(' ').toLowerCase()
}

/** Map a profile's schema_source to a schema id: "custom:<uuid>" → "<uuid>", "builtin:<domain>" stays. */
export function schemaIdFromSource(src: string | null | undefined): string | null {
  if (!src) return null
  if (src.startsWith('custom:')) return src.slice('custom:'.length) || null
  if (src.startsWith('builtin:')) return src
  return null
}

/** Profiles that a dedicated page owns; the generic schema form must not edit them. */
export const managedProfiles: Record<string, { label: string; to: string }> = {
  'com.vibemdm.super': { label: 'Software Updates → super', to: '/updates/super' },
  'com.vibemdm.xcreds': { label: 'Identity → XCreds', to: '/identity/xcreds' },
}

/** Read a File as base64 (no data: prefix). */
export function fileToBase64(f: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onerror = () => reject(r.error ?? new Error('Could not read the file'))
    r.onload = () => {
      const s = String(r.result ?? '')
      const i = s.indexOf(',')
      resolve(i >= 0 ? s.slice(i + 1) : s)
    }
    r.readAsDataURL(f)
  })
}

/** Approximate decoded size of a base64 string. */
export function base64Bytes(b64: string): number {
  const s = b64.replace(/\s+/g, '')
  if (!s) return 0
  const pad = s.endsWith('==') ? 2 : s.endsWith('=') ? 1 : 0
  return Math.max(0, Math.floor((s.length * 3) / 4) - pad)
}

/** Client-side structural check of an uploaded schema before it goes to the server. */
export function checkSchemaText(text: string): { schema?: Record<string, unknown>; error?: string } {
  if (!text.trim()) return { error: 'Paste or upload a JSON schema.' }
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch (e) {
    return { error: `Not valid JSON: ${e instanceof Error ? e.message : String(e)}` }
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return { error: 'The schema must be a JSON object.' }
  const props = (parsed as { properties?: unknown }).properties
  if (!props || typeof props !== 'object' || Array.isArray(props) || Object.keys(props).length === 0) return { error: 'The schema needs a non-empty "properties" object.' }
  return { schema: parsed as Record<string, unknown> }
}
