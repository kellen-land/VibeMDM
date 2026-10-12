import type { ITheme } from '@xterm/xterm'

/** xterm themes tuned to the app surfaces — ink-black with an indigo cast (cyan cursor, purple selection), and a clean light variant. */
export function xtermTheme(dark: boolean): ITheme {
  if (dark) {
    return {
      background: '#0b0913',
      foreground: '#ece9f8',
      cursor: '#22d3ee',
      cursorAccent: '#0b0913',
      selectionBackground: 'rgba(139,92,246,0.32)',
      black: '#14111f',
      red: '#ef4444',
      green: '#22c55e',
      yellow: '#f0a92a',
      blue: '#3b9edb',
      magenta: '#b794f6',
      cyan: '#22d3ee',
      white: '#cfcbe3',
      brightBlack: '#67618a',
      brightRed: '#f87171',
      brightGreen: '#4ade80',
      brightYellow: '#fbbf24',
      brightBlue: '#60b6e6',
      brightMagenta: '#d8b4fe',
      brightCyan: '#67e8f9',
      brightWhite: '#ece9f8',
    }
  }
  return {
    background: '#ffffff',
    foreground: '#120f24',
    cursor: '#0891b2',
    cursorAccent: '#ffffff',
    selectionBackground: 'rgba(124,58,237,0.20)',
    black: '#120f24',
    red: '#dc2626',
    green: '#15803d',
    yellow: '#b45309',
    blue: '#1d6fb8',
    magenta: '#7c3aed',
    cyan: '#0891b2',
    white: '#5b5677',
    brightBlack: '#8f89aa',
    brightRed: '#ef4444',
    brightGreen: '#16a34a',
    brightYellow: '#d97706',
    brightBlue: '#2563eb',
    brightMagenta: '#a855f7',
    brightCyan: '#06b6d4',
    brightWhite: '#120f24',
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
