import { useMemo, useState } from 'react'
import { Download, Radio, RefreshCw } from 'lucide-react'
import { buildQuery, downloadFile, errorMessage } from '../../api/client'
import { useLogComponents } from '../../api/queries'
import { useAuth } from '../../auth/AuthContext'
import type { LogLevel, LogQuery, LogSource } from '../../api/types'
import { useDebounced } from '../../lib/hooks'
import { cn } from '../../lib/cn'
import { Button } from '../ui/Button'
import { Input, Select } from '../ui/Input'
import { SearchInput } from '../ui/SearchInput'
import { Toggle } from '../ui/Checkbox'
import { useToast } from '../ui/Toast'
import { ComputerSelect } from '../pickers/ComputerSelect'
import { LogTable } from './LogTable'
import { MAX_ROWS, useLogStream } from './useLogStream'

type Range = 'all' | '15m' | '1h' | '24h' | '7d' | 'custom'
const rangeMs: Record<Exclude<Range, 'all' | 'custom'>, number> = { '15m': 15 * 60_000, '1h': 3_600_000, '24h': 86_400_000, '7d': 7 * 86_400_000 }

const levelOptions: { value: LogLevel; label: string }[] = [
  { value: 'debug', label: 'Debug and above' },
  { value: 'info', label: 'Info and above' },
  { value: 'warn', label: 'Warn and above' },
  { value: 'error', label: 'Error only' },
]

/** RFC3339 without fractional seconds (what the Go handler parses). */
function rfc3339(ms: number): string {
  return new Date(ms).toISOString().replace(/\.\d{3}Z$/, 'Z')
}

/** datetime-local value ("2026-10-10T14:03") to RFC3339, or undefined. */
function fromLocalInput(v: string): string | undefined {
  if (!v) return undefined
  const t = new Date(v).getTime()
  return Number.isNaN(t) ? undefined : rfc3339(t)
}

/**
 * Search, filter and tail server or agent logs. With `computerId` the view is fixed to that Mac's agent logs and the
 * computer filter is hidden.
 */
export function LogExplorer({ source, computerId, height }: { source: LogSource; computerId?: string; height?: string }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const [level, setLevel] = useState<LogLevel>('debug')
  const [component, setComponent] = useState('')
  const [computer, setComputer] = useState('')
  const [search, setSearch] = useState('')
  const dq = useDebounced(search.trim(), 350)
  const [range, setRange] = useState<Range>('all')
  const [anchor, setAnchor] = useState(() => Date.now())
  const [customFrom, setCustomFrom] = useState('')
  const [customTo, setCustomTo] = useState('')
  const [live, setLive] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const [exporting, setExporting] = useState(false)
  const components = useLogComponents(source)

  const query = useMemo<LogQuery>(() => {
    let from: string | undefined
    let to: string | undefined
    if (range === 'custom') {
      from = fromLocalInput(customFrom)
      to = fromLocalInput(customTo)
    } else if (range !== 'all') {
      from = rfc3339(anchor - rangeMs[range])
    }
    return {
      source,
      level: level === 'debug' ? undefined : level,
      component: component || undefined,
      computer_id: source === 'agent' ? computerId || computer || undefined : undefined,
      q: dq || undefined,
      from,
      to,
    }
  }, [source, level, component, computer, computerId, dq, range, anchor, customFrom, customTo])

  const bounded = !!query.to
  const tail = live && !bounded
  const stream = useLogStream(query, tail, reloadKey)

  const refresh = () => {
    setAnchor(Date.now())
    setReloadKey((k) => k + 1)
  }

  const exportLogs = async () => {
    setExporting(true)
    try {
      await downloadFile(`/api/v1/logs/export${buildQuery({ ...query })}`)
      toast.success('Log export downloaded', 'Exports are recorded in the audit log.')
    } catch (e) {
      toast.error('Export failed', errorMessage(e))
    } finally {
      setExporting(false)
    }
  }

  const compOptions = [{ value: '', label: 'All components' }, ...(components.data ?? []).map((c) => ({ value: c, label: c }))]
  if (component && !(components.data ?? []).includes(component)) compOptions.push({ value: component, label: component })

  return (
    <div>
      <div className="flex flex-wrap items-center gap-2 border-b border-line px-4 py-3">
        <SearchInput value={search} onChange={setSearch} placeholder="Search messages and attributes…" className="w-full sm:w-72" aria-label="Search logs" />
        <Select className="w-44" value={level} onChange={(e) => setLevel(e.target.value as LogLevel)} options={levelOptions} aria-label="Minimum level" />
        <Select className="w-44" value={component} onChange={(e) => setComponent(e.target.value)} options={compOptions} aria-label="Component" />
        {source === 'agent' && !computerId && <ComputerSelect value={computer} onChange={setComputer} className="w-56" />}
        <Select
          className="w-36"
          value={range}
          onChange={(e) => {
            setRange(e.target.value as Range)
            setAnchor(Date.now())
          }}
          options={[
            { value: 'all', label: 'Any time' },
            { value: '15m', label: 'Last 15 min' },
            { value: '1h', label: 'Last hour' },
            { value: '24h', label: 'Last 24 hours' },
            { value: '7d', label: 'Last 7 days' },
            { value: 'custom', label: 'Custom…' },
          ]}
          aria-label="Time range"
        />
        {range === 'custom' && (
          <span className="inline-flex items-center gap-1.5 text-xs text-ink-muted">
            <Input type="datetime-local" step={1} value={customFrom} onChange={(e) => setCustomFrom(e.target.value)} className="w-[13.5rem]" aria-label="From" />
            to
            <Input type="datetime-local" step={1} value={customTo} onChange={(e) => setCustomTo(e.target.value)} className="w-[13.5rem]" aria-label="To" />
          </span>
        )}
        <div className="ml-auto flex items-center gap-2">
          <span title={bounded ? 'Live tail is unavailable when the time range has an end' : `Poll for new entries every 2 seconds (keeps the newest ${MAX_ROWS.toLocaleString()})`}>
            <Toggle
              size="sm"
              checked={tail}
              disabled={bounded}
              onChange={setLive}
              label={
                <span className="inline-flex items-center gap-1.5 text-[13px]">
                  <Radio className={cn('size-3.5', tail ? 'text-accent-400 animate-pulse' : 'text-ink-faint')} />
                  Live tail
                </span>
              }
            />
          </span>
          <Button size="sm" variant="ghost" icon={<RefreshCw />} onClick={refresh} aria-label="Refresh" title="Reload from the newest entry" />
          {isAdmin && (
            <Button size="sm" icon={<Download />} loading={exporting} onClick={exportLogs} title="Download every matching entry as gzipped NDJSON. Exports are audited.">
              Export
            </Button>
          )}
        </div>
      </div>
      <LogTable stream={stream} q={dq} showComputer={source === 'agent' && !computerId} live={tail} height={height} />
    </div>
  )
}
