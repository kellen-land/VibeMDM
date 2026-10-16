import { useState, type FormEvent } from 'react'
import { Modal } from '../ui/Modal'
import { Button } from '../ui/Button'
import { Field } from '../ui/Field'
import { Input } from '../ui/Input'
import { useToast } from '../ui/Toast'
import { api, errorMessage } from '../../api/client'

export function ChangePasswordDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const toast = useToast()
  const reset = () => {
    setCurrent('')
    setNext('')
    setConfirm('')
  }
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (next !== confirm) {
      toast.error('Passwords do not match')
      return
    }
    setBusy(true)
    try {
      await api.post('/api/v1/auth/password', { current_password: current, new_password: next })
      toast.success('Password changed')
      reset()
      onClose()
    } catch (err) {
      toast.error('Could not change password', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open={open} onClose={onClose} title="Change password" size="sm">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field label="Current password">
          <Input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" required />
        </Field>
        <Field label="New password" hint="At least 12 characters.">
          <Input type="password" value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" minLength={12} required />
        </Field>
        <Field label="Confirm new password">
          <Input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" required />
        </Field>
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={busy}>
            Change password
          </Button>
        </div>
      </form>
    </Modal>
  )
}
