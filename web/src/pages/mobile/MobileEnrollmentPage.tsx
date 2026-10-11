import { useState } from 'react'
import { Link } from 'react-router-dom'
import { QrCode } from 'lucide-react'
import { qk, useEnrollmentStatus, useInvalidate } from '../../api/queries'
import { api, buildQuery, errorMessage } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { InvitationCreated } from '../../api/types'
import { PageHeader } from '../../components/ui/PageHeader'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Alert } from '../../components/ui/Alert'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Input } from '../../components/ui/Input'
import { CopyButton } from '../../components/ui/CopyButton'
import { LoadingBlock } from '../../components/ui/Spinner'
import { useToast } from '../../components/ui/Toast'

function Steps({ items }: { items: React.ReactNode[] }) {
  return (
    <ol className="flex flex-col gap-2.5">
      {items.map((it, i) => (
        <li key={i} className="flex gap-3 text-sm text-ink-muted">
          <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-accent-500/15 text-[11px] font-semibold text-ink">{i + 1}</span>
          <span>{it}</span>
        </li>
      ))}
    </ol>
  )
}

/** How to enroll an iPhone or iPad: Apple Configurator (supervised) or Safari (not supervised). */
export function MobileEnrollmentPage() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const q = useEnrollmentStatus()
  const s = q.data
  const [link, setLink] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  if (q.isPending) return <LoadingBlock />
  if (q.error || !s) return <Alert tone="critical">{q.error ? errorMessage(q.error) : 'Enrollment status unavailable'}</Alert>

  const openLink = `${s.enroll_url}?device=mobile`
  const shown = s.open_enrollment ? openLink : link
  const createLink = async () => {
    setCreating(true)
    try {
      const r = await api.post<InvitationCreated>('/api/v1/enrollment/invitations', { name: 'iPhone/iPad (QR)', max_uses: 1, expires_days: 7 })
      setLink(`${r.enroll_url}&device=mobile`)
      await invalidate(qk.invitations, qk.enrollmentStatus)
    } catch (e) {
      toast.error('Could not create a link', errorMessage(e))
    } finally {
      setCreating(false)
    }
  }

  return (
    <>
      <PageHeader title="Mobile Device Enrollment" description="Two ways to enroll an iPhone or iPad without Apple Business Manager." />
      {s.issues.length > 0 && (
        <Alert tone="warning" title="Enrollment is not ready" className="mb-6" actions={<Link to="/enrollment"><Button size="sm">Details</Button></Link>}>
          {s.issues[0]}
        </Alert>
      )}
      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Apple Configurator" actions={<Badge tone="accent">Supervised</Badge>} />
          <CardBody className="flex flex-col gap-4">
            <p className="text-sm text-ink-muted">
              Best for devices the organization owns. Supervision unlocks Lost Mode, the Activation Lock bypass code, silent app installs and most restrictions, and the
              user cannot remove management. <strong className="text-ink">Preparing erases the device.</strong>
            </p>
            <Steps
              items={[
                'On a Mac, install Apple Configurator from the App Store and connect the iPhone or iPad with a cable.',
                <>Select the device and choose <strong className="text-ink">Prepare</strong> → <strong className="text-ink">Manual Configuration</strong>.</>,
                <>Turn on <strong className="text-ink">Supervise devices</strong> (and, if you like, <em>Allow devices to pair with other computers</em>).</>,
                <>At <em>Enroll in MDM Server</em>, choose <strong className="text-ink">New server</strong> and paste the URL below.</>,
                'Create or pick a supervising organization, then finish. The device erases, restarts and enrolls during setup.',
              ]}
            />
            {isAdmin && s.configurator_url ? (
              <div className="flex items-center gap-2">
                <Input readOnly mono value={s.configurator_url} className="flex-1" aria-label="Configurator enrollment URL" />
                <CopyButton text={s.configurator_url} />
              </div>
            ) : (
              <p className="text-xs text-ink-muted">The Configurator URL contains an enrollment secret and is shown to administrators only.</p>
            )}
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Safari" actions={<Badge>Not supervised</Badge>} />
          <CardBody className="flex flex-col gap-4">
            <p className="text-sm text-ink-muted">For a device that should keep its data. The user installs a profile and can remove it later; supervised-only features are not available.</p>
            <Steps
              items={[
                'Scan the QR code with the camera, or open the link in Safari.',
                <>Tap <strong className="text-ink">Download Enrollment Profile</strong>, then <strong className="text-ink">Allow</strong>.</>,
                <>In Settings, tap <strong className="text-ink">Profile Downloaded</strong> → <strong className="text-ink">Install</strong>, enter the passcode and trust remote management.</>,
              ]}
            />
            {shown ? (
              <div className="flex flex-col items-center gap-3">
                <img src={`/api/v1/qr${buildQuery({ text: shown })}`} alt="QR code for the enrollment link" className="size-44 rounded-lg border border-line bg-white p-2" />
                <div className="flex w-full items-center gap-2">
                  <Input readOnly mono value={shown} className="flex-1" aria-label="Enrollment link" />
                  <CopyButton text={shown} />
                </div>
                {!s.open_enrollment && <p className="text-xs text-ink-muted">One-time link, valid for 7 days.</p>}
              </div>
            ) : isAdmin ? (
              <div>
                <p className="mb-3 text-xs text-ink-muted">Open enrollment is off, so each device needs an invitation link.</p>
                <Button icon={<QrCode />} loading={creating} onClick={createLink}>
                  Create a one-time link
                </Button>
              </div>
            ) : (
              <p className="text-xs text-ink-muted">Open enrollment is off; ask an administrator for an enrollment link.</p>
            )}
          </CardBody>
        </Card>
      </div>
    </>
  )
}
