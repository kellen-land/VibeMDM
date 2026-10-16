import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { pageKey } from '../../lib/pageKey'

const APP = 'VibeMDM'

/** The browser tab title for a page heading ("Policies · VibeMDM"). */
export function pageTitle(heading: string | null | undefined): string {
  const h = heading?.trim()
  return h ? `${h} · ${APP}` : APP
}

/**
 * After moving to another page, sets the browser title from the page's h1 and announces it to
 * screen readers (a single-page app gives no load event of its own). Focus is left where it is,
 * so keyboard users keep their place in the sidebar; the skip link takes them to the content.
 */
export function RouteAnnouncer() {
  const loc = useLocation()
  const key = pageKey(loc.pathname)
  const first = useRef(true)
  const [message, setMessage] = useState('')

  useEffect(() => {
    let tries = 0
    let timer: number
    const read = () => {
      // Pages load lazily: wait (up to ~2 s) for the new heading to appear.
      const h1 = document.querySelector('main h1')
      if (!h1 && tries++ < 20) {
        timer = window.setTimeout(read, 100)
        return
      }
      const text = h1?.textContent?.trim() ?? ''
      document.title = pageTitle(text)
      if (first.current) {
        first.current = false
        return
      }
      setMessage(text ? `${text} page` : 'Page loaded')
    }
    timer = window.setTimeout(read, 50)
    return () => window.clearTimeout(timer)
  }, [key])

  return (
    <div role="status" aria-live="polite" aria-atomic="true" className="sr-only">
      {message}
    </div>
  )
}

/** "Skip to content": the first stop on every page, visible only when focused. */
export function SkipLink() {
  return (
    <a
      href="#main"
      onClick={(e) => {
        e.preventDefault()
        const main = document.getElementById('main')
        main?.focus()
        main?.scrollIntoView({ block: 'start' })
      }}
      className="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-50 focus:rounded-md focus:bg-surface-raised focus:px-3 focus:py-2 focus:text-sm focus:font-medium focus:text-ink focus:shadow-lg focus-ring"
    >
      Skip to content
    </a>
  )
}
