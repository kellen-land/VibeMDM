import { useCallback, useEffect, useRef, useState } from 'react'
import { api, errorMessage } from '../../api/client'
import type { LogEntry, LogPage, LogQuery } from '../../api/types'

const PAGE = 200
const TAIL_LIMIT = 500
/** Client-side cap; the oldest rows are dropped (and stay reachable through "Load older"). */
export const MAX_ROWS = 2000
const TAIL_MS = 2_000

export interface LogStream {
  /** Newest first. */
  rows: LogEntry[]
  loading: boolean
  error: string | null
  hasOlder: boolean
  loadingOlder: boolean
  loadOlder: () => void
  /** Increments on every live-tail prepend; `prepended` is that batch's size. */
  prependSeq: number
  prepended: number
}

function useDocumentVisible(): boolean {
  const [visible, setVisible] = useState(() => document.visibilityState !== 'hidden')
  useEffect(() => {
    const on = () => setVisible(document.visibilityState !== 'hidden')
    document.addEventListener('visibilitychange', on)
    return () => document.removeEventListener('visibilitychange', on)
  }, [])
  return visible
}

/**
 * Loads a page of logs for `query`, pages back with before_id, and (when `live`) tails with since_id every 2s
 * while the browser tab is visible. `reloadKey` forces a fresh first page.
 */
export function useLogStream(query: LogQuery, live: boolean, reloadKey = 0): LogStream {
  const key = JSON.stringify(query)
  const [rows, setRows] = useState<LogEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [olderCursor, setOlderCursor] = useState<number | null>(null)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const [prepend, setPrepend] = useState({ seq: 0, n: 0 })
  const lastId = useRef(0)
  /** Bumped on every query change so late responses for an old query are dropped. */
  const gen = useRef(0)
  const visible = useDocumentVisible()

  useEffect(() => {
    const g = ++gen.current
    const ctl = new AbortController()
    setLoading(true)
    setError(null)
    setRows([])
    setOlderCursor(null)
    lastId.current = 0
    api
      .get<LogPage>('/api/v1/logs', { query: { ...(JSON.parse(key) as LogQuery), limit: PAGE }, signal: ctl.signal })
      .then((p) => {
        if (g !== gen.current) return
        const items = p.items ?? []
        setRows(items)
        lastId.current = p.last_id ?? 0
        setOlderCursor(items.length >= PAGE && p.next_before_id ? p.next_before_id : null)
      })
      .catch((e: unknown) => {
        if (g !== gen.current || ctl.signal.aborted) return
        setError(errorMessage(e))
      })
      .finally(() => {
        if (g === gen.current) setLoading(false)
      })
    return () => ctl.abort()
  }, [key, reloadKey])

  const loadOlder = useCallback(() => {
    if (olderCursor === null || loadingOlder) return
    const g = gen.current
    setLoadingOlder(true)
    api
      .get<LogPage>('/api/v1/logs', { query: { ...(JSON.parse(key) as LogQuery), limit: PAGE, before_id: olderCursor } })
      .then((p) => {
        if (g !== gen.current) return
        const items = p.items ?? []
        setRows((r) => [...r, ...items])
        setOlderCursor(items.length >= PAGE && p.next_before_id ? p.next_before_id : null)
      })
      .catch((e: unknown) => {
        if (g === gen.current) setError(errorMessage(e))
      })
      .finally(() => {
        if (g === gen.current) setLoadingOlder(false)
      })
  }, [key, olderCursor, loadingOlder])

  const tailing = live && visible && !loading
  useEffect(() => {
    if (!tailing) return
    let stopped = false
    let timer: ReturnType<typeof setTimeout> | undefined
    const g = gen.current
    const tick = async () => {
      try {
        const since = lastId.current
        const p = await api.get<LogPage>('/api/v1/logs', { query: { ...(JSON.parse(key) as LogQuery), limit: TAIL_LIMIT, since_id: since || undefined } })
        if (stopped || g !== gen.current) return
        const items = p.items ?? []
        // With since_id the server returns oldest first; without one (nothing seen yet) it returns newest first.
        const newest = since ? [...items].reverse() : items
        if (p.last_id) lastId.current = p.last_id
        if (newest.length) {
          setRows((r) => {
            const merged = [...newest, ...r]
            if (merged.length <= MAX_ROWS) return merged
            const kept = merged.slice(0, MAX_ROWS)
            setOlderCursor(kept[kept.length - 1].id)
            return kept
          })
          setPrepend((s) => ({ seq: s.seq + 1, n: newest.length }))
        }
        setError(null)
      } catch (e) {
        if (!stopped) setError(errorMessage(e))
      }
      if (!stopped) timer = setTimeout(tick, TAIL_MS)
    }
    void tick()
    return () => {
      stopped = true
      clearTimeout(timer)
    }
  }, [tailing, key])

  return { rows, loading, error, hasOlder: olderCursor !== null, loadingOlder, loadOlder, prependSeq: prepend.seq, prepended: prepend.n }
}
