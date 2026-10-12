/**
 * Keyboard shortcuts, the same set as Jamf Pro: a modifier plus a letter. The modifier is
 * Control on a Mac (so Command-C/V/S keep their usual meaning) and Alt on Windows and Linux.
 * On a Mac, Command-S also saves.
 */
export type ShortcutAction = 'new' | 'edit' | 'back' | 'logs' | 'cancel' | 'delete' | 'save' | 'view' | 'history' | 'clone'

export const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent)

/** "⌃" on a Mac, "Alt+" elsewhere. */
export const modLabel = isMac ? '⌃' : 'Alt+'

/** Button actions: [key, shift, action]. */
export const actionKeys: { key: string; shift?: boolean; action: ShortcutAction; label: string }[] = [
  { key: 'n', action: 'new', label: 'New (from a list)' },
  { key: 'e', action: 'edit', label: 'Edit' },
  { key: 's', action: 'save', label: 'Save' },
  { key: 'c', action: 'cancel', label: 'Cancel' },
  { key: 'd', action: 'delete', label: 'Delete' },
  { key: 'n', shift: true, action: 'clone', label: 'Clone' },
  { key: 'h', action: 'history', label: 'History' },
  { key: 'l', action: 'logs', label: 'Logs' },
  { key: 'v', action: 'view', label: 'View (group members, search results)' },
  { key: 'b', action: 'back', label: 'Back' },
]

/** Navigation: [key, shift, path]. */
export const navKeys: { key: string; shift?: boolean; to: string; label: string }[] = [
  { key: '1', to: '/', label: 'Dashboard' },
  { key: '2', to: '/computers', label: 'Computers' },
  { key: '3', to: '/mobile-devices', label: 'Mobile Devices' },
  { key: '5', to: '/settings', label: 'Settings' },
  { key: '9', to: '/health', label: 'Health (notifications)' },
  { key: 'l', shift: true, to: '/logs', label: 'Server logs' },
]

/** Does the event use the shortcut modifier (and nothing else)? */
export function hasModifier(e: KeyboardEvent): boolean {
  return isMac ? e.ctrlKey && !e.metaKey && !e.altKey : e.altKey && !e.ctrlKey && !e.metaKey
}

/** The letter or digit pressed, independent of the keyboard layout and the Alt/Option key. */
export function keyOf(e: KeyboardEvent): string {
  if (e.code.startsWith('Key')) return e.code.slice(3).toLowerCase()
  if (e.code.startsWith('Digit')) return e.code.slice(5)
  return e.key.toLowerCase()
}

/** Tooltip text for a button, e.g. "Save (⌃S)". */
export function shortcutHint(action: ShortcutAction, base: string): string {
  const k = actionKeys.find((a) => a.action === action)
  if (!k) return base
  return `${base} (${modLabel}${k.shift ? (isMac ? '⇧' : 'Shift+') : ''}${k.key.toUpperCase()})`
}

/** Is the user typing in a field (where only Save is honoured, so text editing keys keep working)? */
export function isTyping(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el) return false
  const tag = el.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable || !!el.closest('.cm-editor, .monaco-editor')
}
