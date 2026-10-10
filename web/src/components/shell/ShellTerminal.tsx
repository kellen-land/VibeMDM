import { lazy, Suspense } from 'react'
import type { ShellTerminalProps } from './ShellTerminalImpl'
import { LoadingBlock } from '../ui/Spinner'

const Impl = lazy(() => import('./ShellTerminalImpl'))

/** Lazy wrapper so the xterm bundle loads only when a shell is opened. */
export function ShellTerminal(props: ShellTerminalProps) {
  return (
    <Suspense fallback={<LoadingBlock label="Loading terminal…" className="rounded-xl border border-line" />}>
      <Impl {...props} />
    </Suspense>
  )
}
