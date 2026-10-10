import type { ITheme } from '@xterm/xterm'

/** xterm themes tuned to the app surfaces — dark teal-tinted, and a clean light variant. */
export function xtermTheme(dark: boolean): ITheme {
  if (dark) {
    return {
      background: '#0d1215',
      foreground: '#e6edf0',
      cursor: '#19c3b8',
      cursorAccent: '#0d1215',
      selectionBackground: 'rgba(15,181,174,0.30)',
      black: '#11171a',
      red: '#ef4444',
      green: '#22c55e',
      yellow: '#f0a92a',
      blue: '#3b9edb',
      magenta: '#c084fc',
      cyan: '#19c3b8',
      white: '#c7d0d6',
      brightBlack: '#5d6a72',
      brightRed: '#f87171',
      brightGreen: '#4ade80',
      brightYellow: '#fbbf24',
      brightBlue: '#60b6e6',
      brightMagenta: '#d8b4fe',
      brightCyan: '#5ee0d6',
      brightWhite: '#e6edf0',
    }
  }
  return {
    background: '#ffffff',
    foreground: '#0d1518',
    cursor: '#0c8f8a',
    cursorAccent: '#ffffff',
    selectionBackground: 'rgba(12,143,138,0.22)',
    black: '#0d1518',
    red: '#dc2626',
    green: '#15803d',
    yellow: '#b45309',
    blue: '#1d6fb8',
    magenta: '#9333ea',
    cyan: '#0c8f8a',
    white: '#596067',
    brightBlack: '#8a959b',
    brightRed: '#ef4444',
    brightGreen: '#16a34a',
    brightYellow: '#d97706',
    brightBlue: '#2563eb',
    brightMagenta: '#a855f7',
    brightCyan: '#0fb5ae',
    brightWhite: '#0d1518',
  }
}

const enc = new TextEncoder()

export function encodeFrameData(s: string): string {
  return bytesToBase64(enc.encode(s))
}

export function bytesToBase64(bytes: Uint8Array): string {
  let bin = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode(...bytes.subarray(i, i + chunk))
  }
  return btoa(bin)
}

export function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}
