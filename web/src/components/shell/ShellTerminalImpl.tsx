import { useCallback, useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Plug, PlugZap, RotateCw } from 'lucide-react'
import { useTheme } from '../../theme/ThemeContext'
import { Button } from '../ui/Button'
import { cn } from '../../lib/cn'
import { base64ToBytes, encodeFrameData, xtermTheme } from './xtermTheme'

type Status = 'connecting' | 'connected' | 'offline' | 'closed' | 'error'

const statusMeta: Record<Status, { label: string; dot: string; text: string }> = {
  connecting: { label: 'Connecting…', dot: 'bg-warning animate-pulse', text: 'text-ink-muted' },
  connected: { label: 'Connected', dot: 'bg-good', text: 'text-ink' },
  offline: { label: 'Agent offline', dot: 'bg-ink-faint', text: 'text-ink-muted' },
  closed: { label: 'Session closed', dot: 'bg-ink-faint', text: 'text-ink-muted' },
  error: { label: 'Error', dot: 'bg-critical', text: 'text-critical' },
}

export interface ShellTerminalProps { computerId: string; computerName: string }
export default function ShellTerminalImpl({ computerId, computerName }: ShellTerminalProps) {
  const { theme } = useTheme()
  const dark = theme === 'dark'
  const hostRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const endedRef = useRef(false)
  const [status, setStatus] = useState<Status>('connecting')
  const [message, setMessage] = useState<string | null>(null)
  const [nonce, setNonce] = useState(0) // bump to reconnect

  const sendResize = useCallback(() => {
    const term = termRef.current
    const ws = wsRef.current
    if (term && ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ t: 'r', c: term.cols, w: term.rows }))
    }
  }, [])

  // Create the terminal once.
  useEffect(() => {
    if (!hostRef.current) return
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'var(--font-mono), monospace',
      fontSize: 13,
      scrollback: 5000,
      theme: xtermTheme(dark),
      allowProposedApi: true,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(hostRef.current)
    try {
      fit.fit()
    } catch {
      /* not visible yet */
    }
    termRef.current = term
    fitRef.current = fit

    const onData = term.onData((data) => {
      const ws = wsRef.current
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ t: 'd', b: encodeFrameData(data) }))
    })
    const ro = new ResizeObserver(() => {
      try {
        fit.fit()
        sendResize()
      } catch {
        /* ignore */
      }
    })
    ro.observe(hostRef.current)

    return () => {
      onData.dispose()
      ro.disconnect()
      term.dispose()
      termRef.current = null
      fitRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Keep the theme in sync.
  useEffect(() => {
    if (termRef.current) termRef.current.options.theme = xtermTheme(dark)
  }, [dark])

  // (Re)connect the websocket.
  useEffect(() => {
    const term = termRef.current
    if (!term) return
    endedRef.current = false
    setMessage(null)
    setStatus('connecting')
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = `${proto}//${window.location.host}/api/v1/computers/${computerId}/shell`
    let ws: WebSocket
    try {
      ws = new WebSocket(url)
    } catch {
      setStatus('error')
      setMessage('Could not open a connection.')
      return
    }
    wsRef.current = ws

    ws.onopen = () => {
      setStatus('connected')
      try {
        fitRef.current?.fit()
      } catch {
        /* ignore */
      }
      ws.send(JSON.stringify({ t: 'r', c: term.cols, w: term.rows }))
      term.focus()
    }
    ws.onmessage = (ev) => {
      let frame: { t: string; b?: string; msg?: string; code?: number }
      try {
        frame = JSON.parse(typeof ev.data === 'string' ? ev.data : '')
      } catch {
        return
      }
      if (frame.t === 'd' && frame.b) {
        term.write(base64ToBytes(frame.b))
      } else if (frame.t === 'e') {
        const msg = frame.msg || 'Error'
        const offline = /agent did not connect|agent.*offline|no agent/i.test(msg)
        endedRef.current = true
        setStatus(offline ? 'offline' : 'error')
        setMessage(msg)
        term.writeln(`\r\n\x1b[31m${msg}\x1b[0m`)
      } else if (frame.t === 'x') {
        endedRef.current = true
        setStatus('closed')
        term.writeln(`\r\n\x1b[90m[session ended${frame.code !== undefined ? `, exit ${frame.code}` : ''}]\x1b[0m`)
      }
    }
    ws.onerror = () => {
      if (!endedRef.current) {
        setStatus('error')
        setMessage('Connection error.')
      }
    }
    ws.onclose = () => {
      if (!endedRef.current) {
        endedRef.current = true
        setStatus((s) => (s === 'connecting' ? 'error' : 'closed'))
      }
    }

    return () => {
      endedRef.current = true
      try {
        ws.close()
      } catch {
        /* ignore */
      }
      if (wsRef.current === ws) wsRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [computerId, nonce])

  const disconnect = () => {
    endedRef.current = true
    try {
      wsRef.current?.close()
    } catch {
      /* ignore */
    }
    setStatus('closed')
  }
  const reconnect = () => {
    termRef.current?.clear()
    setNonce((n) => n + 1)
  }

  const meta = statusMeta[status]
  const live = status === 'connected' || status === 'connecting'

  return (
    <div className="flex flex-col overflow-hidden rounded-xl border border-line bg-[#0d1215] dark:bg-[#0d1215]" style={{ background: dark ? '#0d1215' : '#ffffff' }}>
      <div className="flex items-center justify-between gap-3 border-b border-line bg-surface px-3 py-2">
        <div className="flex items-center gap-2 min-w-0">
          <span className={cn('size-2 rounded-full shrink-0', meta.dot)} aria-hidden="true" />
          <span className={cn('text-[13px] font-medium', meta.text)}>{meta.label}</span>
          <span className="truncate text-xs text-ink-muted">· root@{computerName}</span>
          {message && status !== 'connected' && <span className="truncate text-xs text-ink-muted">— {message}</span>}
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {live ? (
            <Button size="xs" variant="ghost" icon={<Plug />} onClick={disconnect}>
              Disconnect
            </Button>
          ) : (
            <Button size="xs" variant="secondary" icon={<PlugZap />} onClick={reconnect}>
              Reconnect
            </Button>
          )}
          <Button size="xs" variant="ghost" icon={<RotateCw />} aria-label="Restart session" onClick={reconnect} title="Restart session" />
        </div>
      </div>
      <div ref={hostRef} className="min-h-[420px] flex-1 p-2" style={{ height: '58vh' }} />
    </div>
  )
}
