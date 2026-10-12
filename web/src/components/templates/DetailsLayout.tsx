import type { ReactNode } from 'react'
import { Card, CardHeader } from '../ui/Card'
import { cn } from '../../lib/cn'

/**
 * The Details template's body (Polaris "details"): the object's settings in the main column
 * and a narrow summary column (status, scope, when it changed). On narrow screens the summary
 * comes first, unless asideLast (for side columns holding form fields). breakpoint picks when
 * the two columns appear: '2xl' inside another side-nav layout such as the payload editor.
 */
export function DetailsLayout({ aside, children, asideLast, breakpoint = 'xl' }: { aside?: ReactNode; children: ReactNode; asideLast?: boolean; breakpoint?: 'lg' | 'xl' | '2xl' }) {
  if (!aside) return <div className="flex min-w-0 flex-col gap-6">{children}</div>
  const grid = { lg: 'lg:grid-cols-[minmax(0,1fr)_18rem]', xl: 'xl:grid-cols-[minmax(0,1fr)_18rem]', '2xl': '2xl:grid-cols-[minmax(0,1fr)_18rem]' }[breakpoint]
  const first = { lg: 'lg:order-1', xl: 'xl:order-1', '2xl': '2xl:order-1' }[breakpoint]
  const second = { lg: 'lg:order-2', xl: 'xl:order-2', '2xl': '2xl:order-2' }[breakpoint]
  return (
    <div className={cn('grid gap-6', grid)}>
      <div className={cn('flex min-w-0 flex-col gap-6', asideLast ? 'order-1' : 'order-2', first)}>{children}</div>
      <aside className={cn('flex flex-col gap-6', asideLast ? 'order-2' : 'order-1', second)}>{aside}</aside>
    </div>
  )
}

/** Label/value rows for the Details summary column. Empty values are left out. */
export function SummaryCard({ title = 'Summary', rows }: { title?: string; rows: { label: string; value: ReactNode }[] }) {
  const shown = rows.filter((r) => r.value !== undefined && r.value !== null && r.value !== '' && r.value !== false)
  return (
    <Card>
      <CardHeader title={title} />
      <dl className="divide-y divide-line">
        {shown.map((r) => (
          <div key={r.label} className="flex items-start justify-between gap-3 px-4 py-2.5">
            <dt className="text-xs text-ink-muted">{r.label}</dt>
            <dd className="min-w-0 text-right text-[13px] text-ink">{r.value}</dd>
          </div>
        ))}
      </dl>
    </Card>
  )
}
