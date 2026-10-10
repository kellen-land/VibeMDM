import { useState } from 'react'
import { Download, Eye, EyeOff, RefreshCw } from 'lucide-react'
import { qk, useInvalidate, useSCEP } from '../../api/queries'
import { api, errorMessage, saveText } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { SCEPStatus } from '../../api/types'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Alert } from '../../components/ui/Alert'
import { DescriptionList } from '../../components/ui/DescriptionList'
import { LoadingBlock } from '../../components/ui/Spinner'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { CodeBlock, CopyButton } from '../../components/ui/CopyButton'
import { Input } from '../../components/ui/Input'
import { Field } from '../../components/ui/Field'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'
import { absoluteTime } from '../../lib/format'

export function SCEPSettings() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const scep = useSCEP()
  const [show, setShow] = useState(false)
  const [regen, setRegen] = useState(false)
  const [busy, setBusy] = useState(false)
  const d = scep.data

  const regenerate = async () => {
    setBusy(true)
    try {
      await api.post<SCEPStatus>('/api/v1/settings/scep/regenerate', { confirm: 'REGENERATE' })
      toast.success('SCEP CA regenerated', 'All devices must re-enroll to obtain new identity certificates.')
      await invalidate(qk.scep, qk.enrollmentStatus)
      setRegen(false)
    } catch (e) {
      toast.error('Regeneration failed', errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  if (scep.isPending) return <LoadingBlock />
  if (scep.error || !d) return <Alert tone="critical">{scep.error ? errorMessage(scep.error) : 'Unavailable'}</Alert>

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader
          title="Certificate authority"
          description="The built-in SCEP CA issues an identity certificate to each device during MDM enrollment. Devices use it to authenticate every MDM request."
          actions={
            <Button icon={<Download />} onClick={() => saveText(d.ca_pem, 'vibemdm-scep-ca.pem', 'application/x-pem-file')}>
              Download CA certificate
            </Button>
          }
        />
        <CardBody className="flex flex-col gap-5">
          <DescriptionList
            cols={3}
            items={[
              { label: 'Subject', value: d.subject },
              { label: 'Valid from', value: absoluteTime(d.not_before) },
              {
                label: 'Expires',
                value: (
                  <>
                    {absoluteTime(d.not_after)} (<RelativeTime value={d.not_after} />)
                  </>
                ),
              },
              { label: 'Certificates issued', value: d.issued.toLocaleString() },
              { label: 'Active identities', value: d.active.toLocaleString() },
            ]}
          />
          {isAdmin && d.challenge !== undefined && (
            <Field label="SCEP challenge password" hint="Embedded in enrollment profiles; devices present it when requesting a certificate.">
              <div className="flex flex-wrap items-center gap-2">
                <Input readOnly mono type={show ? 'text' : 'password'} value={d.challenge} className="flex-1 min-w-[240px]" />
                <Button icon={show ? <EyeOff /> : <Eye />} onClick={() => setShow((s) => !s)} aria-label={show ? 'Hide' : 'Show'} />
                <CopyButton text={d.challenge} />
              </div>
            </Field>
          )}
          <details>
            <summary className="cursor-pointer text-xs font-medium text-ink-muted hover:text-ink">Show CA certificate PEM</summary>
            <CodeBlock text={d.ca_pem} filename="vibemdm-scep-ca.pem" className="mt-2" />
          </details>
        </CardBody>
      </Card>

      {isAdmin && (
        <Card className="border-critical/40">
          <CardHeader title="Danger zone" />
          <CardBody>
            <Alert tone="critical" title="Regenerate the certificate authority" actions={<Button variant="danger" icon={<RefreshCw />} onClick={() => setRegen(true)}>Regenerate CA</Button>}>
              Creates a brand-new CA key and certificate and rotates the SCEP challenge. Every enrolled device's identity certificate becomes invalid; devices must re-enroll before they can receive commands again. Only do this if the CA key has been compromised.
            </Alert>
          </CardBody>
        </Card>
      )}

      <ConfirmDialog
        open={regen}
        onClose={() => setRegen(false)}
        onConfirm={regenerate}
        destructive
        loading={busy}
        title="Regenerate the SCEP CA?"
        description="This invalidates every enrolled device identity. All devices will need to re-enroll. This cannot be undone."
        confirmLabel="Regenerate CA"
        typeToConfirm="REGENERATE"
      />
    </div>
  )
}
