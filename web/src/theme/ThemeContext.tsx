import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

/** The colours in use right now. */
export type Theme = 'dark' | 'light'
/** What the user chose. "system" follows the operating system's appearance, live. */
export type ThemePreference = 'system' | Theme

// A new key: the old "vibemdm-theme" was always written (even when nobody chose a theme), so it
// cannot tell a deliberate choice from the old dark default. Must match public/theme-init.js.
const STORAGE_KEY = 'vibemdm-theme-preference'
const query = '(prefers-color-scheme: dark)'

interface ThemeApi {
  theme: Theme
  preference: ThemePreference
  setPreference: (p: ThemePreference) => void
  /** System → Light → Dark → System */
  cycle: () => void
}

const ThemeContext = createContext<ThemeApi | null>(null)

function readPreference(): ThemePreference {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* storage unavailable: follow the system */
  }
  return 'system'
}

function systemTheme(): Theme {
  return typeof window !== 'undefined' && window.matchMedia?.(query).matches === false ? 'light' : 'dark'
}

function apply(theme: Theme) {
  const el = document.documentElement
  el.dataset.theme = theme
  el.style.colorScheme = theme
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(readPreference)
  const [system, setSystem] = useState<Theme>(systemTheme)

  // Follow the OS appearance as it changes (e.g. macOS "Auto" switching at sunset).
  useEffect(() => {
    const mq = window.matchMedia?.(query)
    if (!mq) return
    const onChange = () => setSystem(mq.matches ? 'dark' : 'light')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const theme: Theme = preference === 'system' ? system : preference

  useEffect(() => apply(theme), [theme])

  const setPreference = useCallback((p: ThemePreference) => {
    setPreferenceState(p)
    try {
      localStorage.setItem(STORAGE_KEY, p)
    } catch {
      /* the choice still applies for this visit */
    }
  }, [])
  const cycle = useCallback(() => setPreference(preference === 'system' ? 'light' : preference === 'light' ? 'dark' : 'system'), [preference, setPreference])
  const value = useMemo<ThemeApi>(() => ({ theme, preference, setPreference, cycle }), [theme, preference, setPreference, cycle])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeApi {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider')
  return ctx
}
