import { useRef, useState } from 'react'
import { Download, ExternalLink, Upload } from 'lucide-react'
import { qk, useAPNs, useInvalidate } from '../../api/queries'
import { api, downloadFile, errorMessage, saveText } from '../../api/client'
import { useAuth } from '../../auth/AuthContext'
import type { APNsCert, APNsRequestResponse } from '../../api/types'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Button } from '../../components/ui/Button'
import { Field, FormGrid } from '../../components/ui/Field'
import { Input, Textarea } from '../../components/ui/Input'
import { Checkbox } from '../../components/ui/Checkbox'
import { Alert } from '../../components/ui/Alert'
import { Badge } from '../../components/ui/Badge'
import { Table } from '../../components/ui/Table'
import { LoadingBlock } from '../../components/ui/Spinner'
import { CodeBlock } from '../../components/ui/CopyButton'
import { RelativeTime } from '../../components/ui/RelativeTime'
import { useToast } from '../../components/ui/Toast'
import { absoluteTime } from '../../lib/format'
import { cn } from '../../lib/cn'

function Step({ n, title, description, children, done }: { n: number; title: string; description: string; children: React.ReactNode; done?: boolean }) {
  return (
    <div className="flex gap-4">
      <div className={cn('flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold', done ? 'bg-good text-white' : 'bg-accent-600 text-white')}>{n}</div>
      <div className="min-w-0 flex-1">
        <h3 className="text-sm font-semibold text-ink">{title}</h3>
        <p className="mt-0.5 text-xs text-ink-muted">{description}</p>
        <div className="mt-3">{children}</div>
      </div>
    </div>
  )
}

export function APNsSettings() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const invalidate = useInvalidate()
  const apns = useAPNs()
  const [email, setEmail] = useState('')
  const [submit, setSubmit] = useState(false)
  const [csr, setCsr] = useState<APNsRequestResponse | null>(null)
  const [encrypted, setEncrypted] = useState('')
  const [appleCert, setAppleCert] = useState('')
  const [existingCert, setExistingCert] = useState('')
  const [existingKey, setExistingKey] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const encFile = useRef<HTMLInputElement>(null)
  const certFile = useRef<HTMLInputElement>(null)

  const d = apns.data
  const pendingCsr = csr?.csr_pem ?? d?.pending_request?.csr_pem

  const generate = async () => {
    setBusy('gen')
    try {
      const r = await api.post<APNsRequestResponse>('/api/v1/settings/apns/request', { email, submit })
      setCsr(r)
      await invalidate(qk.apns)
      if (submit && r.submitted) toast.success('Request generated and submitted', `Check ${email} for the signed request from the certificate service.`)
      else if (submit && r.submit_error) toast.warning('Request generated, but automatic submission failed', r.submit_error)
      else toast.success('Certificate signing request generated')
    } catch (e) {
      toast.error('Could not generate request', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }
  const decrypt = async () => {
    setBusy('dec')
    try {
      await downloadFile('/api/v1/settings/apns/decrypt', { method: 'POST', body: { encrypted }, filename: 'vibemdm_push_certificate_request.plist' })
      toast.success('Decrypted request downloaded', 'Upload the .plist at identity.apple.com to receive your push certificate.')
    } catch (e) {
      toast.error('Decrypt failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }
  const complete = async () => {
    setBusy('complete')
    try {
      const r = await api.put<APNsCert>('/api/v1/settings/apns/certificate', { cert_pem: appleCert })
      toast.success('Push certificate installed', `Topic ${r.topic}, valid until ${absoluteTime(r.not_after)}`)
      setAppleCert('')
      setCsr(null)
      await invalidate(qk.apns, qk.enrollmentStatus, qk.dashboard)
    } catch (e) {
      toast.error('Upload failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }
  const uploadExisting = async () => {
    setBusy('existing')
    try {
      const r = await api.put<APNsCert>('/api/v1/settings/apns', { cert_pem: existingCert, key_pem: existingKey })
      toast.success('Push certificate installed', `Topic ${r.topic}, valid until ${absoluteTime(r.not_after)}`)
      setExistingCert('')
      setExistingKey('')
      await invalidate(qk.apns, qk.enrollmentStatus, qk.dashboard)
    } catch (e) {
      toast.error('Upload failed', errorMessage(e))
    } finally {
      setBusy(null)
    }
  }
  const readFile = async (f: File | undefined, setter: (s: string) => void) => {
    if (!f) return
    setter(await f.text())
  }

  if (apns.isPending) return <LoadingBlock />
  if (apns.error) return <Alert tone="critical">{errorMessage(apns.error)}</Alert>

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader title="Installed push certificates" description="Apple Push Notification service certificates let the server wake devices. Renew before expiry; a new certificate must keep the same topic or devices will need to re-enroll." actions={d?.configured ? <Badge tone="good" dot>Configured</Badge> : <Badge tone="warning" dot>Not configured</Badge>} />
        <Table<APNsCert>
          rows={d?.certificates ?? []}
          rowKey={(c) => c.topic}
          dense
          emptyTitle="No push certificate installed"
          emptyDescription="Follow the steps below to obtain one from Apple."
          columns={[
            { key: 'topic', header: 'Topic', render: (c) => <span className="font-mono text-xs">{c.topic}</span> },
            { key: 'subject', header: 'Subject', render: (c) => c.subject },
            { key: 'from', header: 'Valid from', render: (c) => absoluteTime(c.not_before) },
            {
              key: 'to',
              header: 'Expires',
              render: (c) => {
                const soon = new Date(c.not_after).getTime() - Date.now() < 30 * 86400_000
                return (
                  <span className={c.expired ? 'text-critical' : soon ? 'text-[#b37a00]' : ''}>
                    {absoluteTime(c.not_after)} (<RelativeTime value={c.not_after} />)
                  </span>
                )
              },
            },
            { key: 'status', header: 'Status', render: (c) => (c.expired ? <Badge tone="critical" dot>Expired</Badge> : <Badge tone="good" dot>Valid</Badge>) },
          ]}
        />
      </Card>

      {!isAdmin ? (
        <Alert tone="info">Only administrators can request or upload push certificates.</Alert>
      ) : (
        <>
          <Card>
            <CardHeader title="Obtain a push certificate from Apple" description="Three steps. You need an Apple ID (any, ideally a shared organizational one) to sign in to the Apple Push Certificates Portal." />
            <CardBody className="flex flex-col gap-8">
              <Step n={1} title="Generate a certificate request" description="Creates a private key on the server and a signing request. The request must be countersigned by an MDM vendor before Apple accepts it." done={!!pendingCsr}>
                <FormGrid cols={2}>
                  <Field label="Contact email" hint="Included in the request and used if you choose to submit automatically.">
                    <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="it@example.com" />
                  </Field>
                  <div className="flex items-end pb-1">
                    <Checkbox checked={submit} onChange={(e) => setSubmit(e.target.checked)} label="Submit to the free mdmcert.download vendor signing service" description="The signed, encrypted request is emailed to the address above. Otherwise you must get the CSR signed yourself." />
                  </div>
                </FormGrid>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <Button variant="primary" loading={busy === 'gen'} onClick={generate} disabled={!email}>
                    {pendingCsr ? 'Regenerate request' : 'Generate request'}
                  </Button>
                  {pendingCsr && (
                    <Button icon={<Download />} onClick={() => saveText(pendingCsr, 'vibemdm_push.csr', 'application/pkcs10')}>
                      Download CSR
                    </Button>
                  )}
                  {d?.pending_request && (
                    <span className="text-xs text-ink-muted">
                      Pending request created <RelativeTime value={d.pending_request.created_at} />
                    </span>
                  )}
                </div>
                {csr?.submit_error && <Alert tone="warning" className="mt-3">{csr.submit_error}</Alert>}
                {pendingCsr && <CodeBlock text={pendingCsr} filename="vibemdm_push.csr" className="mt-3" maxHeight="10rem" />}
              </Step>

              <Step n={2} title="Decrypt the signed request" description="The vendor returns an encrypted file (usually by email). Paste its contents or upload it; the server decrypts it with the key from step 1 and gives you a .plist to upload to Apple.">
                <div className="flex flex-col gap-3">
                  <Textarea mono rows={5} value={encrypted} onChange={(e) => setEncrypted(e.target.value)} placeholder="Paste the encrypted request contents here…" />
                  <div className="flex flex-wrap items-center gap-2">
                    <input ref={encFile} type="file" className="hidden" onChange={(e) => readFile(e.target.files?.[0], setEncrypted)} />
                    <Button icon={<Upload />} onClick={() => encFile.current?.click()}>
                      Upload file
                    </Button>
                    <Button variant="primary" icon={<Download />} loading={busy === 'dec'} onClick={decrypt} disabled={!encrypted.trim() || !pendingCsr}>
                      Decrypt and download .plist
                    </Button>
                  </div>
                  <Alert tone="info">
                    Then sign in at{' '}
                    <a href="https://identity.apple.com/pushcert/" target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 font-medium underline">
                      identity.apple.com/pushcert <ExternalLink className="size-3" />
                    </a>
                    , choose “Create a Certificate”, upload the downloaded .plist, and download the resulting certificate (.pem).
                  </Alert>
                </div>
              </Step>

              <Step n={3} title="Upload Apple's certificate" description="Paste or upload the .pem you downloaded from Apple. It is paired with the private key generated in step 1.">
                <div className="flex flex-col gap-3">
                  <Textarea mono rows={5} value={appleCert} onChange={(e) => setAppleCert(e.target.value)} placeholder="-----BEGIN CERTIFICATE-----" />
                  <div className="flex flex-wrap items-center gap-2">
                    <input ref={certFile} type="file" accept=".pem,.cer,.crt" className="hidden" onChange={(e) => readFile(e.target.files?.[0], setAppleCert)} />
                    <Button icon={<Upload />} onClick={() => certFile.current?.click()}>
                      Upload .pem
                    </Button>
                    <Button variant="primary" loading={busy === 'complete'} onClick={complete} disabled={!appleCert.trim() || !pendingCsr}>
                      Install certificate
                    </Button>
                  </div>
                </div>
              </Step>
            </CardBody>
          </Card>

          <Card>
            <CardHeader title="Or upload an existing certificate and key" description="Migrating from another server? Provide the push certificate and its private key in PEM format. Keeping the same topic avoids re-enrolling devices." />
            <CardBody className="flex flex-col gap-4">
              <FormGrid cols={2}>
                <Field label="Certificate (PEM)">
                  <Textarea mono rows={6} value={existingCert} onChange={(e) => setExistingCert(e.target.value)} placeholder="-----BEGIN CERTIFICATE-----" />
                </Field>
                <Field label="Private key (PEM)">
                  <Textarea mono rows={6} value={existingKey} onChange={(e) => setExistingKey(e.target.value)} placeholder="-----BEGIN PRIVATE KEY-----" />
                </Field>
              </FormGrid>
              <div>
                <Button variant="primary" loading={busy === 'existing'} onClick={uploadExisting} disabled={!existingCert.trim() || !existingKey.trim()}>
                  Install certificate and key
                </Button>
              </div>
            </CardBody>
          </Card>
        </>
      )}
    </div>
  )
}
