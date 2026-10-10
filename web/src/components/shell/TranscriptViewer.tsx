import { lazy, Suspense } from 'react'
import type { TranscriptViewerProps } from './TranscriptViewerImpl'
import { LoadingBlock } from '../ui/Spinner'

const Impl = lazy(() => import('./TranscriptViewerImpl'))

/** Lazy wrapper so the xterm bundle loads only when a transcript is viewed. */
export function TranscriptViewer(props: TranscriptViewerProps) {
  return (
    <Suspense fallback={<LoadingBlock label="Loading transcript…" className="rounded-lg border border-line" />}>
      <Impl {...props} />
    </Suspense>
  )
}
