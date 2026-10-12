import { useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Copy, History, Pencil, Save, Trash2 } from 'lucide-react'
import { Button } from '../ui/Button'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { shortcutHint } from '../../lib/shortcuts'
import { useAuth } from '../../auth/AuthContext'
import type { EditMode } from './useEditMode'
import { HistoryDialog } from './HistoryDialog'

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

  const save = async () => {
    const ok = await ed.allowNavigation(onSave)
    if (ok !== false) ed.finishEdit()
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
  if (confirmDelete) {
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
        {ed.dirty && <span className="mr-auto text-xs text-ink-muted">Unsaved changes</span>}
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
        <div className="sticky bottom-3 z-20 mt-8 rounded-xl border border-line bg-surface/95 px-4 py-2.5 shadow-lg backdrop-blur">
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
