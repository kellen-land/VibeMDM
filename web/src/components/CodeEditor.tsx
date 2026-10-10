import { lazy, Suspense } from 'react'
import type { CodeEditorProps } from './CodeEditorImpl'
import { LoadingBlock } from './ui/Spinner'

const Impl = lazy(() => import('./CodeEditorImpl'))

/** CodeMirror editor, loaded on demand so the editor bundle stays out of the main chunk. */
export function CodeEditor(props: CodeEditorProps) {
  return (
    <Suspense fallback={<LoadingBlock label="Loading editor…" className="rounded-lg border border-line" />}>
      <Impl {...props} />
    </Suspense>
  )
}
