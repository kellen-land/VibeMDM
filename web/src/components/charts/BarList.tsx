import { lazy, Suspense } from 'react'
import type { Count } from '../../api/types'
import { LoadingBlock } from '../ui/Spinner'

const Impl = lazy(() => import('./BarListImpl'))

export interface BarListProps {
  data: Count[]
  maxItems?: number
  emptyLabel?: string
}

/** Lazy wrapper so the charting library is only downloaded on the Dashboard. */
export function BarList(props: BarListProps) {
  return (
    <Suspense fallback={<LoadingBlock className="py-8" />}>
      <Impl {...props} />
    </Suspense>
  )
}
