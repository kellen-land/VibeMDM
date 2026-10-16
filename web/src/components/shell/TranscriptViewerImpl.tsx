import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { useTheme } from '../../theme/ThemeContext'
import { safeLinkHandler, xtermTheme } from './xtermTheme'

/** Read-only render of a recorded shell transcript into a detached xterm so ANSI is preserved. */
export interface TranscriptViewerProps { transcript: string; height?: string }
export default function TranscriptViewerImpl({ transcript, height = "50vh" }: TranscriptViewerProps) {
  const { theme } = useTheme()
  const dark = theme === 'dark'
  const hostRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!hostRef.current) return
    const term = new Terminal({
      disableStdin: true,
      cursorBlink: false,
      cursorStyle: 'underline',
      fontFamily: 'var(--font-mono), monospace',
      fontSize: 13,
      scrollback: 100000,
      theme: xtermTheme(dark),
      convertEol: true,
      allowProposedApi: true,
      linkHandler: safeLinkHandler,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(hostRef.current)
    try {
      fit.fit()
    } catch {
      /* ignore */
    }
    term.write(transcript || '\x1b[90m(empty transcript)\x1b[0m')
    const ro = new ResizeObserver(() => {
      try {
        fit.fit()
      } catch {
        /* ignore */
      }
    })
    ro.observe(hostRef.current)
    return () => {
      ro.disconnect()
      term.dispose()
    }
  }, [transcript, dark])

  return <div ref={hostRef} className="overflow-hidden rounded-lg border border-line p-2" style={{ height, background: dark ? '#0d1215' : '#ffffff' }} />
}
