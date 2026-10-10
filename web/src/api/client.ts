export class ApiError extends Error {
  status: number
  /** Parsed JSON error body, when the server sent one (e.g. {error, errors:[{path,message}]}). */
  body?: unknown
  constructor(status: number, message: string, body?: unknown) {
    super(message)
    this.status = status
    this.name = 'ApiError'
    this.body = body
  }
}

/** Field-level validation errors carried by a 400 response, if any. */
export function fieldErrors(err: unknown): { path: string; message: string }[] {
  if (!(err instanceof ApiError)) return []
  const b = err.body as { errors?: unknown } | undefined
  if (!b || !Array.isArray(b.errors)) return []
  return b.errors.filter((e): e is { path: string; message: string } => !!e && typeof e === 'object' && typeof (e as { path?: unknown }).path === 'string')
}

export const PASSWORD_CHANGE_REQUIRED = 'password change required'

let passwordChangeHandler: (() => void) | null = null

/** Called whenever any request fails with 403 "password change required". */
export function setPasswordChangeHandler(fn: (() => void) | null) {
  passwordChangeHandler = fn
}

/** True for the 403 the server returns while the signed-in account must choose a new password. */
export function isPasswordChangeRequired(err: unknown): boolean {
  return err instanceof ApiError && err.status === 403 && err.message === PASSWORD_CHANGE_REQUIRED
}

type Query = Record<string, string | number | boolean | undefined | null | string[]>

export function buildQuery(params?: Query): string {
  if (!params) return ''
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    if (Array.isArray(v)) {
      if (v.length) sp.set(k, v.join(','))
      continue
    }
    sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

const AUTH_PATHS = ['/api/v1/auth/me', '/api/v1/auth/login']

function handleUnauthorized(path: string) {
  if (AUTH_PATHS.includes(path)) return
  if (window.location.pathname === '/login') return
  const next = window.location.pathname + window.location.search
  window.location.assign(`/login?next=${encodeURIComponent(next)}`)
}

async function parseErrorBody(res: Response): Promise<{ message: string; body?: unknown }> {
  try {
    const body = (await res.json()) as { error?: string }
    if (body && typeof body.error === 'string') return { message: body.error, body }
    return { message: res.statusText || `Request failed (${res.status})`, body }
  } catch {
    /* ignore */
  }
  return { message: res.statusText || `Request failed (${res.status})` }
}

async function parseError(res: Response): Promise<string> {
  return (await parseErrorBody(res)).message
}

export interface RequestOptions {
  query?: Query
  signal?: AbortSignal
}

async function request<T>(method: string, path: string, body?: unknown, opts?: RequestOptions): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  const init: RequestInit = { method, credentials: 'same-origin', headers, signal: opts?.signal }
  if (method !== 'GET' && method !== 'HEAD') {
    headers['X-Requested-With'] = 'fetch'
  }
  if (body !== undefined) {
    if (body instanceof FormData) {
      init.body = body
    } else {
      headers['Content-Type'] = 'application/json'
      init.body = JSON.stringify(body)
    }
  }
  const res = await fetch(path + buildQuery(opts?.query), init)
  if (res.status === 401) {
    handleUnauthorized(path)
    throw new ApiError(401, 'Not authenticated')
  }
  if (!res.ok) {
    const { message, body: errBody } = await parseErrorBody(res)
    if (res.status === 403 && message === PASSWORD_CHANGE_REQUIRED) passwordChangeHandler?.()
    throw new ApiError(res.status, message, errBody)
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}

export const api = {
  get: <T>(path: string, opts?: RequestOptions) => request<T>('GET', path, undefined, opts),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('POST', path, body, opts),
  put: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PUT', path, body, opts),
  patch: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PATCH', path, body, opts),
  del: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('DELETE', path, body, opts),
}

/** Fetch a binary/text download (POST or GET) and trigger a browser download. */
export async function downloadFile(path: string, opts?: { method?: 'GET' | 'POST'; body?: unknown; filename?: string }) {
  const method = opts?.method ?? 'GET'
  const headers: Record<string, string> = {}
  const init: RequestInit = { method, credentials: 'same-origin', headers }
  if (method === 'POST') {
    headers['X-Requested-With'] = 'fetch'
    if (opts?.body !== undefined) {
      headers['Content-Type'] = 'application/json'
      init.body = JSON.stringify(opts.body)
    }
  }
  const res = await fetch(path, init)
  if (res.status === 401) {
    handleUnauthorized(path)
    throw new ApiError(401, 'Not authenticated')
  }
  if (!res.ok) throw new ApiError(res.status, await parseError(res))
  const blob = await res.blob()
  let filename = opts?.filename
  if (!filename) {
    const cd = res.headers.get('Content-Disposition') ?? ''
    const m = /filename="?([^";]+)"?/.exec(cd)
    filename = m?.[1] ?? 'download'
  }
  saveBlob(blob, filename)
}

export function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 2000)
}

export function saveText(text: string, filename: string, type = 'text/plain') {
  saveBlob(new Blob([text], { type }), filename)
}

/** Upload with progress via XHR. */
export function uploadWithProgress<T>(path: string, form: FormData, onProgress: (pct: number) => void, signal?: AbortSignal): Promise<T> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', path)
    xhr.withCredentials = true
    xhr.setRequestHeader('X-Requested-With', 'fetch')
    xhr.setRequestHeader('Accept', 'application/json')
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onerror = () => reject(new ApiError(0, 'Network error during upload'))
    xhr.onabort = () => reject(new ApiError(0, 'Upload cancelled'))
    xhr.onload = () => {
      if (xhr.status === 401) {
        handleUnauthorized(path)
        reject(new ApiError(401, 'Not authenticated'))
        return
      }
      let body: unknown = null
      try {
        body = xhr.responseText ? JSON.parse(xhr.responseText) : null
      } catch {
        body = null
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(body as T)
      } else {
        const msg = (body as { error?: string } | null)?.error ?? xhr.statusText ?? 'Upload failed'
        reject(new ApiError(xhr.status, msg))
      }
    }
    if (signal) signal.addEventListener('abort', () => xhr.abort())
    xhr.send(form)
  })
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  if (err instanceof Error) return err.message
  return String(err)
}
