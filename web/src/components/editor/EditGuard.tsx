import { createContext, useCallback, useContext, useEffect, useRef, type ReactNode } from 'react'
import { useBlocker } from 'react-router-dom'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { editKey } from '../../lib/pageKey'

type Check = () => boolean

const GuardContext = createContext<(check: Check) => () => void>(() => () => {})

/**
 * One unsaved-changes guard for the whole app (React Router allows a single blocker). Every
 * edit form registers "do I have unsaved changes?"; leaving the page, or closing the tab,
 * asks first when any of them does.
 */
export function EditGuardProvider({ children }: { children: ReactNode }) {
  const checks = useRef(new Set<Check>())
  const dirty = () => [...checks.current].some((c) => c())
  const register = useCallback((check: Check) => {
    checks.current.add(check)
    return () => checks.current.delete(check)
  }, [])
  const blocker = useBlocker(({ currentLocation, nextLocation }) => editKey(currentLocation.pathname) !== editKey(nextLocation.pathname) && dirty())
  useEffect(() => {
    const onUnload = (e: BeforeUnloadEvent) => {
      if (dirty()) e.preventDefault()
    }
    window.addEventListener('beforeunload', onUnload)
    return () => window.removeEventListener('beforeunload', onUnload)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  return (
    <GuardContext.Provider value={register}>
      {children}
      <ConfirmDialog
        open={blocker.state === 'blocked'}
        onClose={() => blocker.reset?.()}
        onConfirm={() => blocker.proceed?.()}
        destructive
        title="Discard unsaved changes?"
        description="You have changes on this page that are not saved. Leave without saving them?"
        confirmLabel="Discard changes"
        cancelLabel="Keep editing"
      />
    </GuardContext.Provider>
  )
}

/** Register an unsaved-changes check for as long as the component is mounted. */
export function useRegisterGuard(check: Check) {
  const register = useContext(GuardContext)
  const ref = useRef(check)
  ref.current = check
  useEffect(() => register(() => ref.current()), [register])
}
