import { Fragment, useState } from 'react'
import { Link } from 'react-router-dom'
import { useCommandSearch } from '../../api/queries'
import { errorMessage } from '../../api/client'
import type { CommandSearchHit } from '../../api/types'
import { useDebounced } from '../../lib/hooks'
import { Badge, StatusBadge } from '../ui/Badge'
import { RelativeTime } from '../ui/RelativeTime'
import { EmptyState } from '../ui/EmptyState'
import { LoadingBlock } from '../ui/Spinner'
import { CommandInspectorModal } from './CommandInspector'

/** Wrap case-insensitive matches of `q` in <mark>. */
export function Highlight({ text, q }: { text: string; q: string }) {
  const needle = q.trim()
  if (!needle) return <>{text}</>
  const parts = text.split(new RegExp(`(${needle.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'ig'))
  return (
    <>
      {parts.map((p, i) =>
        i % 2 === 1 ? (
          <mark key={i} className="rounded-sm bg-accent-500/25 px-0.5 text-ink">
            {p}
          </mark>
        ) : (
          <Fragment key={i}>{p}</Fragment>
        ),
      )}
    </>
  )
}

/** Results for a command plist search; matching runs on redacted text. */
export function CommandSearchResults({ query, computerId }: { query: string; computerId?: string }) {
  const q = useDebounced(query.trim(), 300)
  const res = useCommandSearch({ q, computer_id: computerId })
  const [open, setOpen] = useState<string | null>(null)
  const hits = res.data?.items ?? []
  return (
    <>
      <p className="border-b border-line px-4 py-2 text-xs text-ink-muted">
        Searching request and response plists. Matching runs on the <span className="text-ink">redacted</span> text, so redacted secrets can&apos;t be found or confirmed.
        {res.data && q.length >= 2 && (
          <span>
            {' '}
            {res.data.total} {res.data.total === 1 ? 'match' : 'matches'}.
          </span>
        )}
      </p>
      {res.error && <p className="px-5 py-4 text-sm text-critical">{errorMessage(res.error)}</p>}
      {q.length < 2 ? (
        <EmptyState compact title="Type at least 2 characters" />
      ) : res.isPending ? (
        <LoadingBlock />
      ) : hits.length === 0 ? (
        <EmptyState compact title="No commands match" description="Try a request type, payload identifier, bundle ID or error text." />
      ) : (
        <ul className="divide-y divide-line">
          {hits.map((h: CommandSearchHit) => (
            <li key={`${h.command_uuid}-${h.matched_in}`}>
              <button type="button" onClick={() => setOpen(h.command_uuid)} className="flex w-full flex-col gap-1.5 px-4 py-3 text-left hover:bg-line/20 focus-ring">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium text-ink">{h.request_type}</span>
                  <StatusBadge status={h.status} />
                  <Badge>{h.matched_in === 'result' ? 'in response' : 'in request'}</Badge>
                  {!computerId && h.computer_id && (
                    <Link to={`/computers/${h.computer_id}`} onClick={(e) => e.stopPropagation()} className="text-xs text-ink-muted hover:text-ink hover:underline">
                      {h.computer_name || h.computer_id}
                    </Link>
                  )}
                  <span className="ml-auto text-xs text-ink-muted">
                    <RelativeTime value={h.created_at} />
                  </span>
                </div>
                <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-md border border-line bg-surface-sunken px-2.5 py-1.5 font-mono text-xs text-ink-muted">
                  <Highlight text={h.snippet} q={q} />
                </pre>
              </button>
            </li>
          ))}
        </ul>
      )}
      <CommandInspectorModal uuid={open} onClose={() => setOpen(null)} />
    </>
  )
}
