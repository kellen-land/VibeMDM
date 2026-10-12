/**
 * The page a path shows. Tabs kept in the path (a device record's /computers/<id>/<tab>, an
 * onboarding workflow's /onboarding/<id>/runs) belong to one page, so switching them does
 * not remount it.
 */
export function pageKey(path: string): string {
  const m = /^\/(computers|mobile-devices|onboarding)\/[^/]+/.exec(path)
  if (m && m[0] !== '/onboarding/new') return m[0]
  return path
}

/**
 * Where unsaved edits live, for the leave-page prompt. An onboarding workflow keeps its form
 * across its tabs; a device record edits inside a tab (User and Location, Extension
 * Attributes), so its tabs count as separate places.
 */
export function editKey(path: string): string {
  const m = /^\/onboarding\/[^/]+/.exec(path)
  if (m && m[0] !== '/onboarding/new') return m[0]
  return path
}
