import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { Count } from '../../api/types'
import { EmptyState } from '../ui/EmptyState'

/**
 * Single-series horizontal bar chart for magnitude comparisons (sequential/one hue).
 * Follows the dataviz mark spec: bars <= 24px, 4px rounded data-end, hairline grid, hover tooltip.
 */
export default function BarListImpl({ data, maxItems = 10, emptyLabel = 'No data yet' }: { data: Count[]; maxItems?: number; emptyLabel?: string }) {
  const rows = data.slice(0, maxItems)
  if (rows.length === 0) return <EmptyState title={emptyLabel} compact />
  const height = Math.max(120, rows.length * 32 + 24)
  return (
    <div style={{ height }} className="w-full text-xs">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={rows} layout="vertical" margin={{ top: 4, right: 36, bottom: 4, left: 4 }} barCategoryGap={8}>
          <CartesianGrid horizontal={false} stroke="var(--chart-grid)" strokeWidth={1} />
          <XAxis type="number" allowDecimals={false} tickLine={false} axisLine={{ stroke: 'var(--chart-axis)' }} tick={{ fill: 'var(--chart-muted)', fontSize: 11 }} />
          <YAxis type="category" dataKey="label" width={140} tickLine={false} axisLine={false} tick={{ fill: 'var(--ink-muted)', fontSize: 12 }} interval={0} />
          <Tooltip
            cursor={{ fill: 'var(--line)', opacity: 0.4 }}
            content={({ active, payload }) => {
              if (!active || !payload?.length) return null
              const p = payload[0]?.payload as Count
              return (
                <div className="rounded-lg border border-line bg-surface-raised px-3 py-2 shadow-lg text-xs">
                  <p className="font-medium text-ink">{p.label}</p>
                  <p className="text-ink-muted tabular-nums">
                    {p.count.toLocaleString()} {p.count === 1 ? 'computer' : 'computers'}
                  </p>
                </div>
              )
            }}
          />
          <Bar
            dataKey="count"
            fill="var(--chart-series)"
            maxBarSize={20}
            radius={[0, 4, 4, 0]}
            isAnimationActive={false}
            label={{ position: 'right', fill: 'var(--ink-muted)', fontSize: 11, formatter: (v: unknown) => (typeof v === 'number' ? v.toLocaleString() : String(v)) }}
          />
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}
