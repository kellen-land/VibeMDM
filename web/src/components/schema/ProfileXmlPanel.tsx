import { useMemo, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useSchemaPreview } from '../../api/queries'
import { errorMessage } from '../../api/client'
import type { SchemaValues } from '../../api/types'
import { useDebounced } from '../../lib/hooks'
import { cn } from '../../lib/cn'
import { Card } from '../ui/Card'
import { CopyButton } from '../ui/CopyButton'
import { Spinner } from '../ui/Spinner'
import { CodeEditor } from '../CodeEditor'
import { pruneValues } from './schema'

/** Debounced live preview of the profile a schema form would produce. */
export function useLivePreview(schemaId: string | null | undefined, name: string, values: SchemaValues, enabled = true) {
  const body = useMemo(() => ({ name: name.trim(), values: pruneValues(values) }), [name, values])
  const debounced = useDebounced(body, 500)
  return useSchemaPreview(enabled ? schemaId : null, enabled ? debounced : null)
}

type PreviewQuery = ReturnType<typeof useLivePreview>

/** Collapsible "Profile XML" panel fed by useLivePreview. */
export function ProfileXmlPanel({ preview, className, defaultOpen = false }: { preview: PreviewQuery; className?: string; defaultOpen?: boolean }) {
  const [open, setOpen] = useState(defaultOpen)
  const data = preview.data
  const nErr = data?.errors?.length ?? 0
  return (
    <Card className={className}>
      <div className={cn('flex items-center gap-3 px-4 py-3', open && 'border-b border-line')}>
        <button type="button" onClick={() => setOpen((o) => !o)} aria-expanded={open} className="flex min-w-0 flex-1 items-center gap-2 text-left focus-ring rounded">
          <ChevronRight className={cn('size-4 shrink-0 text-ink-faint transition-transform', open && 'rotate-90')} aria-hidden="true" />
          <span className="min-w-0">
            <span className="block text-[13px] font-semibold text-ink leading-5">Profile XML</span>
            <span className="block text-xs text-ink-muted">Live preview of the configuration profile, refreshed as you edit.</span>
          </span>
        </button>
        <span className="flex shrink-0 items-center gap-2 text-xs text-ink-muted">
          {preview.isFetching && <Spinner className="size-3.5" />}
          {nErr > 0 ? (
            <span className="text-critical">
              {nErr} {nErr === 1 ? 'problem' : 'problems'}
            </span>
          ) : data?.payload ? (
            <span>Valid</span>
          ) : null}
          {open && data?.payload && <CopyButton text={data.payload} size="xs" variant="ghost" />}
        </span>
      </div>
      {open && (
        <div className="p-4">
          {preview.error ? (
            <p className="text-sm text-critical">Preview unavailable: {errorMessage(preview.error)}</p>
          ) : !data ? (
            <p className="text-sm text-ink-muted">Building preview…</p>
          ) : nErr > 0 || !data.payload ? (
            <p className="text-sm text-ink-muted">Fix the values marked in the form to see the profile.</p>
          ) : (
            <CodeEditor language="xml" value={data.payload} readOnly editable={false} height="380px" />
          )}
        </div>
      )}
    </Card>
  )
}
