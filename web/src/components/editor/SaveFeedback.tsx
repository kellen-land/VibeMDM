import { createContext, useCallback, useContext, useRef, type ReactNode } from 'react'

/** A check that draws itself (stroke animation); static with reduced motion. */
export function DrawnCheck({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.6} strokeLinecap="round" strokeLinejoin="round" className={`animate-check-draw ${className ?? ''}`} aria-hidden="true">
      <path d="M5 12.5l4.5 4.5L19 7.5" />
    </svg>
  )
}

const SaveFeedbackContext = createContext<() => void>(() => {})

/**
 * Marks the page as "just saved" for a moment: CSS ([data-saved] [data-card]) pulses each
 * card green, top to bottom, as the object locks back to read-only.
 */
export function SaveFeedbackProvider({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  const timer = useRef<number | undefined>(undefined)
  const celebrate = useCallback(() => {
    const el = ref.current
    if (!el) return
    el.removeAttribute('data-saved')
    void el.offsetWidth // restart the animation when saving twice in a row
    el.setAttribute('data-saved', '')
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => el.removeAttribute('data-saved'), 1300)
  }, [])
  return (
    <SaveFeedbackContext.Provider value={celebrate}>
      <div ref={ref}>{children}</div>
    </SaveFeedbackContext.Provider>
  )
}

/** Celebrate a successful save on the current page. */
export function useCelebrate() {
  return useContext(SaveFeedbackContext)
}
