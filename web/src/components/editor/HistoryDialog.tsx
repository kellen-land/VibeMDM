import { useAudit } from '../../api/queries'
import { errorMessage } from '../../api/client'
import { Modal } from '../ui/Modal'
import { Spinner } from '../ui/Spinner'
import { RelativeTime } from '../ui/RelativeTime'

/** An object's history from the audit log (Jamf Pro's History button): who changed it and when. */
export function HistoryDialog({ open, onClose, targetId, title }: { open: boolean; onClose: () => void; targetId: string; title: string }) {
  const q = useAudit({ target_id: targetId, limit: 100 }, open)
  const rows = q.data?.items ?? []
  return (
    <Modal open={open} onClose={onClose} title={`History: ${title}`} size="lg">
      {q.isPending && (
        <p className="flex items-center gap-2 text-sm text-ink-muted">
          <Spinner className="size-4" /> Loading…
        </p>
      )}
      {q.error && <p className="text-sm text-critical">{errorMessage(q.error)}</p>}
      {!q.isPending && !q.error && rows.length === 0 && <p className="text-sm text-ink-muted">No changes recorded yet.</p>}
      {rows.length > 0 && (
        <ol className="divide-y divide-line">
          {rows.map((a) => {
            const details = Object.entries(a.details ?? {}).filter(([, v]) => v !== '' && v !== null && typeof v !== 'object')
            return (
              <li key={a.id} className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 py-2.5 text-sm">
                <span className="font-medium text-ink">{a.action.split('.').slice(1).join(' ').replace(/_/g, ' ') || a.action}</span>
                <span className="text-ink-muted">by {a.actor || 'system'}</span>
                <RelativeTime value={a.at} className="ml-auto text-xs text-ink-faint" />
                {details.length > 0 && (
                  <span className="w-full text-xs text-ink-muted">
                    {details
                      .slice(0, 4)
                      .map(([k, v]) => `${k.replace(/_/g, ' ')}: ${String(v)}`)
                      .join(' · ')}
                  </span>
                )}
              </li>
            )
          })}
        </ol>
      )}
    </Modal>
  )
}
