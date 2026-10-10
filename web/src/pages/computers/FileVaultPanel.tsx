import { useEffect, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { EyeOff, KeyRound, Wrench } from 'lucide-react'
import { api, errorMessage } from '../../api/client'
import { qk, useInvalidate } from '../../api/queries'
import { useAuth } from '../../auth/AuthContext'
import type { ComputerDetail, FileVaultReveal } from '../../api/types'
import { CardBody, CardHeader } from '../../components/ui/Card'
import { Badge, BoolBadge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { CopyButton } from '../../components/ui/CopyButton'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { DescriptionList } from '../../components/ui/DescriptionList'
import { Field } from '../../components/ui/Field'
import { Input } from '../../components/ui/Input'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'
import { needsRemediation } from '../FileVaultPage'

/** FileVault escrow state for one Mac, with audited key reveal and Escrow Buddy remediation (admin). */
export function FileVaultPanel({ c }: { c: ComputerDetail }) {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const fv = c.filevault
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [reason, setReason] = useState('')
  const [key, setKey] = useState<string | null>(null)
  const [remediating, setRemediating] = useState(false)

  // One-off mutation: the key lives only in this component's state, never in the query cache.
  const reveal = useMutation<FileVaultReveal, Error, string>({
    mutationFn: (r) => api.post<FileVaultReveal>(`/api/v1/computers/${c.id}/filevault/reveal`, { reason: r }),
    gcTime: 0,
  })
  const { reset } = reveal
  useEffect(() => () => {
    setKey(null)
    reset()
  }, [reset])

  const doReveal = async () => {
    try {
      const r = await reveal.mutateAsync(reason.trim())
      setKey(r.key)
      reveal.reset()
      setConfirmOpen(false)
      setReason('')
    } catch (e) {
      toast.error('Could not reveal key', errorMessage(e))
    }
  }

  const remediate = async () => {
    setRemediating(true)
    try {
      await api.post(`/api/v1/computers/${c.id}/filevault/remediate`)
      toast.success('Escrow Buddy remediation queued', 'A new recovery key will be generated and escrowed at the next login.')
      await invalidate(qk.computer(c.id), qk.computerTasks(c.id), qk.filevault)
    } catch (e) {
      toast.error('Remediation failed', errorMessage(e))
    } finally {
      setRemediating(false)
    }
  }

  const canRemediate = isAdmin && needsRemediation({ filevault_enabled: c.filevault_enabled, escrowed: !!fv?.escrowed })

  return (
    <>
      <CardHeader
        title="FileVault recovery key escrow"
        description="Personal recovery key escrowed through the FDERecoveryKeyEscrow profile and SecurityInfo."
        className="border-t border-line"
        actions={
          <>
            {canRemediate && (
              <Button size="sm" icon={<Wrench />} loading={remediating} onClick={remediate}>
                Remediate missing key
              </Button>
            )}
            {isAdmin && fv?.escrowed && !key && (
              <Button size="sm" icon={<KeyRound />} onClick={() => setConfirmOpen(true)}>
                Show recovery key
              </Button>
            )}
          </>
        }
      />
      <CardBody className="flex flex-col gap-4">
        <DescriptionList
          cols={3}
          items={[
            { label: 'FileVault', value: <BoolBadge value={c.filevault_enabled} yes="On" no="Off" /> },
            {
              label: 'Recovery key',
              value: fv?.escrowed ? (
                <Badge tone="good" dot>
                  Escrowed
                </Badge>
              ) : c.filevault_enabled ? (
                <Badge tone="warning" dot>
                  Missing key
                </Badge>
              ) : (
                <Badge>Not escrowed</Badge>
              ),
            },
            { label: 'Fingerprint', value: fv?.fingerprint, mono: true },
            { label: 'Escrowed', value: fv?.escrowed_at ? <RelativeTime value={fv.escrowed_at} /> : '' },
            { label: 'Times rotated', value: fv ? String(fv.rotated_count) : '' },
            { label: 'Escrow profile', value: fv?.escrow_profile_status ? <StatusBadge status={fv.escrow_profile_status} /> : <span className="text-ink-faint">Not scoped</span> },
            ...(fv?.source ? [{ label: 'Source', value: fv.source }] : []),
          ]}
        />
        {key && (
          <div className="rounded-xl border border-warning/40 bg-warning/5 p-4">
            <p className="micro-label">Personal recovery key</p>
            <p className="mt-2 break-all font-mono text-xl tracking-wider text-ink select-all sm:text-2xl">{key}</p>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <CopyButton text={key} />
              <Button size="sm" variant="ghost" icon={<EyeOff />} onClick={() => setKey(null)}>
                Hide
              </Button>
              <span className="text-xs text-ink-muted">This view was recorded in the audit log.</span>
            </div>
          </div>
        )}
      </CardBody>
      <ConfirmDialog
        open={confirmOpen}
        onClose={() => {
          setConfirmOpen(false)
          setReason('')
        }}
        onConfirm={doReveal}
        loading={reveal.isPending}
        confirmDisabled={!reason.trim()}
        title="Show the FileVault recovery key?"
        description="Viewing a recovery key is audited under your name with the reason below, and notifies configured webhooks (FileVaultKeyViewed)."
        confirmLabel="Show key"
      >
        <Field label="Reason" required hint="For example a ticket number.">
          <Input value={reason} onChange={(e) => setReason(e.target.value)} autoFocus autoComplete="off" placeholder="e.g. HELP-1234" />
        </Field>
      </ConfirmDialog>
    </>
  )
}
