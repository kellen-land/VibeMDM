import { useEffect, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Copy, History, Lock, Pencil, Save, Trash2 } from 'lucide-react'
import { Button } from '../ui/Button'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { shortcutHint } from '../../lib/shortcuts'
import { useAuth } from '../../auth/AuthContext'
import type { EditMode } from './useEditMode'
import { HistoryDialog } from './HistoryDialog'
import { DrawnCheck, useCelebrate } from './SaveFeedback'
import { cn } from '../../lib/cn'

/** Fired after a successful save; the bar on the page (possibly a new one, after creating) plays the saved animation. */
const SAVED_EVENT = 'vibemdm:saved'

export interface ObjectBarProps {
  ed: EditMode
  isNew: boolean
  /** The list page (Done, and Cancel on a new object, go back there). */
  listPath: string
  /** Lower-case noun for messages, e.g. "policy". */
  noun: string
  name?: string
  saving?: boolean
  /** Save the object; return false when it failed (the page shows why). */
  onSave: () => Promise<boolean | void>
  /** Delete (view mode); the bar asks for a second click first. Omit when the object can't be deleted. */
  onDelete?: () => Promise<boolean | void>
  /** What deleting does besides removing the object, e.g. "It is removed from every computer." */
  deleteDescription?: string
  /** Clone (view mode): open a new object prefilled with this one. */
  onClone?: () => void
  /** Audit-log id for History (admins only). */
  historyId?: string
  /** Extra view-mode buttons (Run now, Logs, View, …), shown before Done. */
  extras?: ReactNode
  /** Singletons (settings pages) have no list to go back to. */
  noDone?: boolean
  /** Edit opens a separate page instead of unlocking this one (compliance baselines). */
  onEdit?: () => void
  /** Records edited per section (computers, mobile devices): no Edit for the whole object. */
  noEdit?: boolean
  /** Delete asks with the page's own dialog (e.g. type the name) instead of the second click. */
  onDeleteRequest?: () => void
}

/**
 * The action bar at the bottom right of every object page, as in Jamf Pro:
 *
 *	view:  History · Clone · Delete · [extras] · Done · Edit
 *	edit:  Cancel · Save
 *
 * One Save for the whole object, whatever tab you are on. Buttons carry data-shortcut so
 * the Jamf keyboard shortcuts (⌃S save, ⌃E edit, ⌃C cancel, ⌃D delete, …) press them.
 */
export function ObjectBar({ ed, isNew, listPath, noun, name, saving, onSave, onDelete, deleteDescription, onClone, historyId, extras, noDone, onEdit, noEdit, onDeleteRequest }: ObjectBarProps) {
  const navigate = useNavigate()
  const { isAdmin } = useAuth()
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [discard, setDiscard] = useState(false)
  const [history, setHistory] = useState(false)
  const [justSaved, setJustSaved] = useState(false)
  const celebrate = useCelebrate()

  // "Saved · locked in": green check on the bar and a pulse over the page's cards. Listening
  // to an event lets the bar of a just-created object (a new page) play it too.
  useEffect(() => {
    let t: number | undefined
    const on = () => {
      setJustSaved(true)
      celebrate()
      window.clearTimeout(t)
      t = window.setTimeout(() => setJustSaved(false), 1600)
    }
    window.addEventListener(SAVED_EVENT, on)
    return () => {
      window.removeEventListener(SAVED_EVENT, on)
      window.clearTimeout(t)
    }
  }, [celebrate])

  const save = async () => {
    const ok = await ed.allowNavigation(onSave)
    if (ok !== false) {
      ed.finishEdit()
      // After the page (or the new object's page) has rendered in view mode.
      window.setTimeout(() => window.dispatchEvent(new Event(SAVED_EVENT)), 30)
    }
  }
  const cancel = () => {
    if (ed.dirty) return setDiscard(true)
    doCancel()
  }
  const doCancel = () => {
    if (isNew) void ed.allowNavigation(async () => navigate(listPath))
    else ed.cancelEdit()
  }
  const del = async () => {
    setDeleting(true)
    try {
      await ed.allowNavigation(onDelete!)
    } finally {
      setDeleting(false)
      setConfirmDelete(false)
    }
  }

  let content: ReactNode
  if (justSaved && !ed.editing) {
    content = (
      <>
        <span className="animate-lock-fade mr-auto inline-flex items-center gap-1.5 text-xs font-medium text-good">
          <Lock className="size-3.5" aria-hidden="true" />
          {name ? `${name} saved` : `Saved`}
        </span>
        <span role="status" className="animate-saved-pop inline-flex h-[34px] items-center gap-2 rounded-md bg-good-strong px-3.5 text-[13px] font-semibold text-white">
          <DrawnCheck className="size-4" />
          Saved
        </span>
      </>
    )
  } else if (confirmDelete) {
    content = (
      <>
        <p className="mr-auto min-w-0 text-sm text-ink">
          Delete {name ? <b>{name}</b> : `this ${noun}`}?<span className="text-ink-muted"> {deleteDescription ?? 'This cannot be undone.'}</span>
        </p>
        <Button onClick={() => setConfirmDelete(false)} data-shortcut="cancel" title={shortcutHint('cancel', 'Cancel')}>
          Cancel
        </Button>
        <Button variant="danger" icon={<Trash2 />} loading={deleting} onClick={del} data-shortcut="delete" title={shortcutHint('delete', 'Delete')} autoFocus>
          Delete
        </Button>
      </>
    )
  } else if (ed.editing) {
    content = (
      <>
        {ed.dirty && (
          <span className="mr-auto inline-flex items-center gap-2 text-xs font-medium text-ink">
            <span className="animate-dirty-dot size-2 rounded-full bg-accent-500" aria-hidden="true" />
            Unsaved changes
          </span>
        )}
        <Button onClick={cancel} data-shortcut="cancel" title={shortcutHint('cancel', 'Cancel')}>
          Cancel
        </Button>
        <Button variant="primary" icon={<Save />} loading={saving} onClick={save} data-shortcut="save" title={shortcutHint('save', 'Save')}>
          Save
        </Button>
      </>
    )
  } else {
    content = (
      <>
        {historyId && isAdmin && (
          <Button variant="ghost" icon={<History />} onClick={() => setHistory(true)} data-shortcut="history" title={shortcutHint('history', 'History')} className="mr-auto">
            History
          </Button>
        )}
        {onClone && isAdmin && (
          <Button variant="ghost" icon={<Copy />} onClick={onClone} data-shortcut="clone" title={shortcutHint('clone', 'Clone')}>
            Clone
          </Button>
        )}
        {(onDelete || onDeleteRequest) && isAdmin && (
          <Button variant="ghost" icon={<Trash2 />} onClick={() => (onDeleteRequest ? onDeleteRequest() : setConfirmDelete(true))} data-shortcut="delete" title={shortcutHint('delete', 'Delete')}>
            Delete
          </Button>
        )}
        {extras}
        {!noDone && (
          <Button onClick={() => navigate(listPath)} data-shortcut="back" title={shortcutHint('back', 'Done')}>
            Done
          </Button>
        )}
        {isAdmin && !noEdit && (
          <Button variant="primary" icon={<Pencil />} onClick={onEdit ?? ed.startEdit} data-shortcut="edit" title={shortcutHint('edit', 'Edit')}>
            Edit
          </Button>
        )}
      </>
    )
  }

  // A viewer on a page with nothing to go back to has no buttons at all.
  const empty = !isAdmin && !ed.editing && !confirmDelete && noDone && !extras
  return (
    <>
      {!empty && (
        <div
          className={cn(
            'sticky bottom-3 z-20 mt-8 overflow-hidden rounded-xl border bg-surface/95 px-4 py-2.5 shadow-lg backdrop-blur transition-colors duration-300',
            justSaved && !ed.editing ? 'border-good/50' : ed.dirty ? 'border-accent-500/60' : 'border-line',
          )}
        >
          {justSaved && !ed.editing && <span aria-hidden="true" className="animate-bar-sweep absolute inset-x-0 top-0 h-0.5 bg-good" />}
          <div className="flex flex-wrap items-center justify-end gap-2">{content}</div>
        </div>
      )}
      {ed.guard}
      <ConfirmDialog
        open={discard}
        onClose={() => setDiscard(false)}
        onConfirm={() => {
          setDiscard(false)
          doCancel()
        }}
        destructive
        title="Discard your changes?"
        description={`The ${noun} goes back to how it was last saved.`}
        confirmLabel="Discard changes"
        cancelLabel="Keep editing"
      />
      {historyId && <HistoryDialog open={history} onClose={() => setHistory(false)} targetId={historyId} title={name ?? noun} />}
    </>
  )
}
