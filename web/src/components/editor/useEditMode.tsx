import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { useRegisterGuard } from './EditGuard'
import { editKey } from '../../lib/pageKey'

export interface EditMode {
  /** In edit mode (always for a new object). */
  editing: boolean
  /** Fields are read-only: view mode, or a viewer. */
  readOnly: boolean
  /** Edits not saved yet. */
  dirty: boolean
  startEdit: () => void
  /** Leave edit mode, discarding changes (the page's reset puts the saved values back). */
  cancelEdit: () => void
  /** Leave edit mode after a successful save. */
  finishEdit: () => void
  /** Run fn (a save that navigates, a delete) without the unsaved-changes prompt. */
  allowNavigation: <T>(fn: () => Promise<T>) => Promise<T>
  /** Nothing to render any more: the prompt is app-wide (EditGuardProvider). */
  guard: React.ReactNode
}

/**
 * Jamf Pro's object lifecycle: an existing object opens read-only, Edit unlocks it, Save or
 * Cancel return to view mode; a new object starts in edit mode. snapshot is everything the
 * page would save (compared as JSON to tell whether there are unsaved changes); reset puts
 * the saved values back on Cancel. Leaving the page with unsaved changes asks first.
 */
export function useEditMode({ isNew, canEdit, snapshot, reset }: { isNew: boolean; canEdit: boolean; snapshot: unknown; reset: () => void }): EditMode {
  const location = useLocation()
  const [editing, setEditing] = useState(isNew && canEdit)
  const snap = JSON.stringify(snapshot ?? null)
  const baseline = useRef(snap)
  const bypass = useRef(false)

  // A different object (or new ↔ existing) starts over.
  const key = editKey(location.pathname)
  const lastKey = useRef(key)
  useEffect(() => {
    if (lastKey.current !== key) {
      lastKey.current = key
      setEditing(isNew && canEdit)
      baseline.current = snap
    }
  }, [key, isNew, canEdit, snap])
  useEffect(() => {
    if (isNew && canEdit) setEditing(true)
  }, [isNew, canEdit])
  // While viewing, whatever the server sends is the saved state.
  useEffect(() => {
    if (!editing) baseline.current = snap
  }, [editing, snap])

  const dirty = editing && snap !== baseline.current
  const dirtyRef = useRef(dirty)
  dirtyRef.current = dirty

  useRegisterGuard(() => dirtyRef.current && !bypass.current)

  const startEdit = useCallback(() => {
    if (!canEdit) return
    baseline.current = snap
    setEditing(true)
  }, [canEdit, snap])
  const cancelEdit = useCallback(() => {
    reset()
    setEditing(false)
  }, [reset])
  const finishEdit = useCallback(() => setEditing(isNew), [isNew])
  const allowNavigation = useCallback(async <T,>(fn: () => Promise<T>) => {
    bypass.current = true
    try {
      return await fn()
    } finally {
      bypass.current = false
    }
  }, [])

  // The dialog is app-wide (EditGuardProvider); kept for pages that render ed.guard.
  const guard = null

  return { editing, readOnly: !canEdit || !editing, dirty, startEdit, cancelEdit, finishEdit, allowNavigation, guard }
}

/** The object to prefill a new one with, when it was opened by Clone. */
export function useCloneSource<T>(): T | undefined {
  const location = useLocation()
  return (location.state as { clone?: T } | null)?.clone
}
