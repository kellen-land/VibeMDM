import { useEffect, useState, type ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'
import { Modal } from './Modal'
import { Button } from './Button'
import { Input } from './Input'
import { Field } from './Field'

export interface ConfirmDialogProps {
  open: boolean
  onClose: () => void
  onConfirm: () => void | Promise<void>
  title: ReactNode
  description?: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
  /** When set, the user must type this text exactly to enable the confirm button. */
  typeToConfirm?: string
  loading?: boolean
  children?: ReactNode
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  destructive,
  typeToConfirm,
  loading,
  children,
}: ConfirmDialogProps) {
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (!open) setTyped('')
  }, [open])
  const ok = !typeToConfirm || typed === typeToConfirm
  const handle = async () => {
    setBusy(true)
    try {
      await onConfirm()
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open={open}
      onClose={onClose}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy || loading}>
            {cancelLabel}
          </Button>
          <Button variant={destructive ? 'danger' : 'primary'} onClick={handle} disabled={!ok} loading={busy || loading}>
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div className="flex gap-4">
        {destructive && (
          <div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-critical/10 text-critical">
            <AlertTriangle className="size-5" />
          </div>
        )}
        <div className="min-w-0 flex-1">
          <h2 className="text-base font-semibold text-ink">{title}</h2>
          {description && <div className="mt-1.5 text-sm text-ink-muted">{description}</div>}
          {children && <div className="mt-3">{children}</div>}
          {typeToConfirm && (
            <Field
              className="mt-4"
              label={
                <>
                  Type <span className="font-mono font-semibold text-ink">{typeToConfirm}</span> to continue
                </>
              }
            >
              <Input value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus autoComplete="off" spellCheck={false} />
            </Field>
          )}
        </div>
      </div>
    </Modal>
  )
}
