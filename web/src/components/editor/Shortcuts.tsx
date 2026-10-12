import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Modal } from '../ui/Modal'
import { actionKeys, hasModifier, isMac, isTyping, keyOf, modLabel, navKeys } from '../../lib/shortcuts'

/** The topmost open dialog, else the page: shortcuts act there. */
function scope(): ParentNode {
  const dialogs = document.querySelectorAll('[role="dialog"]')
  return dialogs.length ? dialogs[dialogs.length - 1] : document
}

/** A visible, enabled button (or link) for the action. */
function target(action: string): HTMLElement | null {
  for (const el of scope().querySelectorAll<HTMLElement>(`[data-shortcut="${action}"]`)) {
    if ((el as HTMLButtonElement).disabled || el.getAttribute('aria-disabled') === 'true') continue
    if (el.offsetParent === null && getComputedStyle(el).position !== 'fixed') continue
    return el
  }
  return null
}

/**
 * Jamf Pro's keyboard shortcuts, app-wide. Buttons opt in with data-shortcut="save" (edit,
 * cancel, delete, clone, new, history, logs, view, back); the handler clicks the one in the
 * open dialog, or on the page. While typing in a field only Save works, so the field's own
 * Control-key editing (Control-E, Control-D, … on a Mac) is left alone. "?" lists them.
 */
export function ShortcutHandler() {
  const navigate = useNavigate()
  const [help, setHelp] = useState(false)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.repeat) return
      const k = keyOf(e)
      // Command-S on a Mac saves too (instead of the browser's "Save page").
      const cmdS = isMac && e.metaKey && !e.ctrlKey && !e.altKey && !e.shiftKey && k === 's'
      if (!cmdS && !hasModifier(e)) {
        if (e.key === '?' && !isTyping(e.target) && !document.querySelector('[role="dialog"]')) {
          e.preventDefault()
          setHelp(true)
        }
        return
      }
      const a = cmdS ? actionKeys.find((x) => x.action === 'save') : actionKeys.find((x) => x.key === k && !!x.shift === e.shiftKey)
      if (a) {
        if (isTyping(e.target) && a.action !== 'save') return
        const el = target(a.action)
        if (el) {
          e.preventDefault()
          el.click()
        } else if (a.action === 'save') {
          e.preventDefault() // never the browser's "Save page"
        } else if (a.action === 'back' && !document.querySelector('[role="dialog"]')) {
          e.preventDefault()
          navigate(-1)
        }
        return
      }
      const n = navKeys.find((x) => x.key === k && !!x.shift === e.shiftKey)
      if (n && !isTyping(e.target) && !document.querySelector('[role="dialog"]')) {
        e.preventDefault()
        navigate(n.to)
      }
    }
    // Capture phase: before fields and editors see the keys.
    document.addEventListener('keydown', onKey, true)
    return () => document.removeEventListener('keydown', onKey, true)
  }, [navigate])
  return <ShortcutHelp open={help} onClose={() => setHelp(false)} />
}

const shift = isMac ? '⇧' : 'Shift+'

/** The list of shortcuts ("?" or the account menu). */
export function ShortcutHelp({ open, onClose }: { open: boolean; onClose: () => void }) {
  const row = (keys: string, label: string) => (
    <div key={keys + label} className="flex items-center justify-between gap-4 py-1.5">
      <span className="text-sm text-ink">{label}</span>
      <kbd className="rounded border border-line bg-surface-sunken px-1.5 py-0.5 font-mono text-xs text-ink-muted">{keys}</kbd>
    </div>
  )
  return (
    <Modal open={open} onClose={onClose} title="Keyboard shortcuts" size="md">
      <div className="grid gap-6 sm:grid-cols-2">
        <section>
          <h3 className="micro-label mb-1">Objects</h3>
          <div className="divide-y divide-line">{actionKeys.map((a) => row(`${modLabel}${a.shift ? shift : ''}${a.key.toUpperCase()}`, a.label))}</div>
        </section>
        <section>
          <h3 className="micro-label mb-1">Go to</h3>
          <div className="divide-y divide-line">
            {navKeys.map((n) => row(`${modLabel}${n.shift ? shift : ''}${n.key.toUpperCase()}`, n.label))}
            {row(`${modLabel}[`, 'Collapse or expand the sidebar')}
            {row('?', 'This list')}
          </div>
        </section>
      </div>
      <p className="mt-4 text-xs text-ink-muted">
        The same shortcuts as Jamf Pro: Control on a Mac (Command-S also saves), Alt on Windows and Linux. While you type in a field, only Save works.
      </p>
    </Modal>
  )
}
