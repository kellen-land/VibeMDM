import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { CheckCircle2, AlertCircle, Info, X, AlertTriangle } from 'lucide-react'
import { cn } from '../../lib/cn'

export type ToastKind = 'success' | 'error' | 'info' | 'warning'

interface ToastItem {
  id: number
  kind: ToastKind
  title: string
  description?: string
}

interface ToastApi {
  toast: (kind: ToastKind, title: string, description?: string) => void
  success: (title: string, description?: string) => void
  error: (title: string, description?: string) => void
  info: (title: string, description?: string) => void
  warning: (title: string, description?: string) => void
}

const ToastContext = createContext<ToastApi | null>(null)

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  const counter = useRef(0)
  const remove = useCallback((id: number) => setItems((l) => l.filter((t) => t.id !== id)), [])
  const toast = useCallback(
    (kind: ToastKind, title: string, description?: string) => {
      const id = ++counter.current
      setItems((l) => [...l.slice(-4), { id, kind, title, description }])
      setTimeout(() => remove(id), kind === 'error' ? 8000 : 4500)
    },
    [remove],
  )
  const api = useMemo<ToastApi>(
    () => ({
      toast,
      success: (t, d) => toast('success', t, d),
      error: (t, d) => toast('error', t, d),
      info: (t, d) => toast('info', t, d),
      warning: (t, d) => toast('warning', t, d),
    }),
    [toast],
  )
  return (
    <ToastContext.Provider value={api}>
      {children}
      {createPortal(
        <div className="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-full max-w-sm flex-col gap-2">
          {items.map((t) => (
            <div
              key={t.id}
              role="status"
              className={cn(
                'pointer-events-auto animate-toast-in flex items-start gap-3 rounded-lg border bg-surface-raised px-3.5 py-3 shadow-xl ring-1 ring-black/30',
                t.kind === 'error' ? 'border-critical/35' : t.kind === 'success' ? 'border-good/35' : t.kind === 'warning' ? 'border-warning/45' : 'border-line-strong/60',
              )}
            >
              <span
                className={cn(
                  'mt-0.5 shrink-0 [&>svg]:size-4',
                  t.kind === 'success' && 'text-good',
                  t.kind === 'error' && 'text-critical',
                  t.kind === 'warning' && 'text-[#b37a00]',
                  t.kind === 'info' && 'text-accent-600',
                )}
              >
                {t.kind === 'success' ? <CheckCircle2 /> : t.kind === 'error' ? <AlertCircle /> : t.kind === 'warning' ? <AlertTriangle /> : <Info />}
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-ink">{t.title}</p>
                {t.description && <p className="mt-0.5 text-xs text-ink-muted break-words">{t.description}</p>}
              </div>
              <button type="button" onClick={() => remove(t.id)} className="text-ink-faint hover:text-ink [&>svg]:size-4" aria-label="Dismiss">
                <X />
              </button>
            </div>
          ))}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  )
}
