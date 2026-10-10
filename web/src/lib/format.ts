const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

export function parseDate(v: string | Date | null | undefined): Date | null {
  if (!v) return null
  const d = v instanceof Date ? v : new Date(v)
  if (Number.isNaN(d.getTime())) return null
  // Go zero time
  if (d.getTime() < 0 || d.getFullYear() <= 1) return null
  return d
}

export function relativeTime(v: string | Date | null | undefined, now = Date.now()): string {
  const d = parseDate(v)
  if (!d) return 'never'
  const diff = d.getTime() - now
  const abs = Math.abs(diff)
  const sec = Math.round(diff / 1000)
  if (abs < 45_000) return diff <= 0 ? 'just now' : 'in a moment'
  const min = Math.round(sec / 60)
  if (abs < 3_600_000) return rtf.format(min, 'minute')
  const hr = Math.round(min / 60)
  if (abs < 86_400_000) return rtf.format(hr, 'hour')
  const day = Math.round(hr / 24)
  if (abs < 30 * 86_400_000) return rtf.format(day, 'day')
  const month = Math.round(day / 30)
  if (abs < 365 * 86_400_000) return rtf.format(month, 'month')
  return rtf.format(Math.round(day / 365), 'year')
}

export function absoluteTime(v: string | Date | null | undefined): string {
  const d = parseDate(v)
  if (!d) return ''
  return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'medium' })
}

/** Time of day with milliseconds, e.g. "14:03:22.417" (inspector timelines). */
export function timeWithMs(v: string | Date | null | undefined): string {
  const d = parseDate(v)
  if (!d) return ''
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false, fractionalSecondDigits: 3 })
}

export function shortDate(v: string | Date | null | undefined): string {
  const d = parseDate(v)
  if (!d) return '—'
  return d.toLocaleDateString(undefined, { dateStyle: 'medium' })
}

export function formatBytes(n: number | null | undefined, decimals = 1): string {
  if (n === null || n === undefined || Number.isNaN(n)) return '—'
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 ? 0 : decimals)} ${units[i]}`
}

export function formatMB(mb: number | null | undefined): string {
  if (mb === null || mb === undefined) return '—'
  return formatBytes(mb * 1024 * 1024)
}

export function formatDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined) return '—'
  const s = Math.floor(seconds)
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  const parts: string[] = []
  if (d) parts.push(`${d}d`)
  if (h) parts.push(`${h}h`)
  if (m || parts.length === 0) parts.push(`${m}m`)
  return parts.join(' ')
}

export function formatMs(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return '—'
  if (ms < 1000) return `${ms} ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)} s`
  return formatDuration(s)
}

export function formatNumber(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  return n.toLocaleString()
}

export function compactNumber(n: number): string {
  if (Math.abs(n) < 10_000) return n.toLocaleString()
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(n)
}

export function shortHash(h: string | null | undefined, n = 12): string {
  if (!h) return '—'
  return h.slice(0, n)
}

export function titleCase(s: string): string {
  return s.replace(/[_-]+/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

export function yesNo(v: boolean | null | undefined): string {
  if (v === null || v === undefined) return 'Unknown'
  return v ? 'Yes' : 'No'
}

export function dash(v: string | number | null | undefined): string {
  if (v === null || v === undefined || v === '') return '—'
  return String(v)
}
