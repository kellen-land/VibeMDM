import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { KeyRound } from 'lucide-react'
import { api, errorMessage } from '../api/client'
import type { LAPSReveal } from '../api/types'
import { Alert } from './ui/Alert'
import { Button } from './ui/Button'
import { ConfirmDialog } from './ui/ConfirmDialog'
import { CopyButton } from './ui/CopyButton'
import { Field } from './ui/Field'
import { Input } from './ui/Input'
import { Modal } from './ui/Modal'
import { RelativeTime } from './ui/RelativeTime'
import { useToast } from './ui/Toast'

/**
 * "Show password" for one Mac: asks for a reason, records the view on the server, and shows the
 * password in a dialog. The password lives only in this component's state, never in the query cache.
 */
export function LapsRevealButton({ computerId, computerName, onDone }: { computerId: string; computerName?: string; onDone?: () => void }) {
  const toast = useToast()
  const [ask, setAsk] = useState(false)
  const [reason, setReason] = useState('')
  const [shown, setShown] = useState<LAPSReveal | null>(null)
  const reveal = useMutation<LAPSReveal, Error, string>({
    mutationFn: (r) => api.post<LAPSReveal>(`/api/v1/computers/${computerId}/laps/reveal`, { reason: r }),
    gcTime: 0,
  })

  const close = () => {
    setShown(null)
    onDone?.()
  }
  const go = async () => {
    try {
      const r = await reveal.mutateAsync(reason.trim())
      setShown(r)
      setAsk(false)
      setReason('')
      reveal.reset()
    } catch (e) {
      toast.error('Could not show the password', errorMessage(e))
    }
  }

  return (
    <>
      <Button size="xs" variant="ghost" icon={<KeyRound />} onClick={() => setAsk(true)}>
        Show password
      </Button>
      <ConfirmDialog
        open={ask}
        onClose={() => {
          setAsk(false)
          setReason('')
        }}
        onConfirm={go}
        loading={reveal.isPending}
        confirmDisabled={!reason.trim()}
        title="Show the local admin password?"
        description="This is recorded under your name with the reason below. The password changes automatically a while after it is viewed."
        confirmLabel="Show password"
      >
        <Field label="Reason" required hint="For example a ticket number.">
          <Input value={reason} onChange={(e) => setReason(e.target.value)} autoFocus autoComplete="off" placeholder="e.g. HELP-1234" />
        </Field>
      </ConfirmDialog>
      <Modal open={!!shown} onClose={close} title={computerName ? `Local admin password · ${computerName}` : 'Local admin password'} size="md" footer={<Button onClick={close}>Done</Button>}>
        {shown && (
          <div className="flex flex-col gap-4">
            <div>
              <p className="micro-label">Account</p>
              <p className="mt-1 font-mono text-sm text-ink">{shown.account}</p>
            </div>
            <div>
              <p className="micro-label">Password</p>
              <p className="mt-1 break-all font-mono text-xl tracking-wide text-ink select-all">{shown.password || '—'}</p>
              {shown.password && (
                <div className="mt-2">
                  <CopyButton text={shown.password} />
                </div>
              )}
            </div>
            {shown.unconfirmed_password && (
              <Alert tone="warning" title="A newer password was never confirmed">
                <p>The Mac may already be using this one if the password above does not work:</p>
                <p className="mt-1 break-all font-mono text-ink select-all">{shown.unconfirmed_password}</p>
              </Alert>
            )}
            {shown.status.rotate_after && (
              <p className="text-xs text-ink-muted">
                It changes automatically <RelativeTime value={shown.status.rotate_after} /> because it was viewed.
              </p>
            )}
          </div>
        )}
      </Modal>
    </>
  )
}
