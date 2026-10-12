import { useRef, useState, type FormEvent } from 'react'
import { Download, Package as PackageIcon, Pencil, Trash2, Upload } from 'lucide-react'
import { qk, useInvalidate, usePackages } from '../api/queries'
import { api, errorMessage, uploadWithProgress } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Package } from '../api/types'
import { Button } from '../components/ui/Button'
import { Badge } from '../components/ui/Badge'
import { RelativeTime } from '../components/ui/RelativeTime'
import { Modal } from '../components/ui/Modal'
import { Field, FormGrid } from '../components/ui/Field'
import { Input, Textarea } from '../components/ui/Input'
import { ProgressBar } from '../components/ui/Progress'
import { useToast } from '../components/ui/Toast'
import { IndexPage } from '../components/templates/IndexPage'
import { formatBytes, shortHash } from '../lib/format'

export function PackagesPage() {
  const { isAdmin } = useAuth()
  const packages = usePackages()
  const invalidate = useInvalidate()
  const [uploadOpen, setUploadOpen] = useState(false)
  const [edit, setEdit] = useState<Package | null>(null)
  const rows = packages.data?.items
  const categories = [...new Set((rows ?? []).map((p) => p.category).filter(Boolean))].sort()
  const nPackages = (n: number) => `${n} ${n === 1 ? 'package' : 'packages'}`

  const download = (p: Package) => {
    const a = document.createElement('a')
    a.href = `/api/v1/packages/${p.id}/download`
    a.download = p.filename
    document.body.appendChild(a)
    a.click()
    a.remove()
  }

  return (
    <>
      <IndexPage<Package>
        title="Packages"
        description="Installers (.pkg, .mpkg, .dmg, .zip) served to computers by policies."
        newLabel="Upload package"
        onNew={() => setUploadOpen(true)}
        onRowClick={isAdmin ? (p) => setEdit(p) : undefined}
        rows={rows}
        loading={packages.isPending}
        error={packages.error}
        rowKey={(p) => p.id}
        search={{ placeholder: 'Search packages…', text: (p) => `${p.name} ${p.filename} ${p.category}` }}
        filters={categories.length ? [{ key: 'category', label: 'Any category', options: categories.map((c) => ({ value: c, label: c })), match: (p, v) => p.category === v }] : undefined}
        defaultSort={{ key: 'name', dir: 'asc' }}
        onChanged={() => invalidate(qk.packages, qk.dashboard)}
        bulkActions={[
          {
            label: 'Delete',
            icon: <Trash2 />,
            destructive: true,
            confirm: { title: (n) => `Delete ${nPackages(n)}?`, description: 'The files are removed from the server. Policies that install them will fail until they are re-uploaded.' },
            run: (p) => api.del(`/api/v1/packages/${p.id}`),
            done: (n) => `${nPackages(n)} deleted`,
          },
        ]}
        empty={{ title: 'No packages yet', description: 'Upload an installer (.pkg, .mpkg, .dmg or .zip) to deploy it with a policy.' }}
        columns={[
          {
            key: 'name',
            header: 'Name',
            sortValue: (p) => p.name,
            render: (p) => (
              <span className="flex items-center gap-2.5">
                <PackageIcon className="size-4 text-ink-faint" />
                <span className="min-w-0">
                  <span className="block font-medium text-ink">{p.name}</span>
                  {p.info && <span className="block text-xs text-ink-muted truncate max-w-md">{p.info}</span>}
                </span>
              </span>
            ),
          },
          { key: 'filename', header: 'Filename', sortValue: (p) => p.filename, render: (p) => <span className="font-mono text-xs">{p.filename}</span> },
          { key: 'category', header: 'Category', sortValue: (p) => p.category, render: (p) => (p.category ? <Badge>{p.category}</Badge> : <span className="text-ink-faint">—</span>) },
          { key: 'size', header: 'Size', align: 'right', sortValue: (p) => p.size_bytes, render: (p) => <span className="tabular-nums">{formatBytes(p.size_bytes)}</span> },
          { key: 'sha', header: 'SHA-256', render: (p) => <span className="font-mono text-xs text-ink-muted" title={p.sha256}>{shortHash(p.sha256)}</span> },
          { key: 'priority', header: 'Priority', align: 'right', sortValue: (p) => p.priority, render: (p) => <span className="tabular-nums text-ink-muted">{p.priority}</span> },
          { key: 'updated', header: 'Updated', sortValue: (p) => p.updated_at, render: (p) => <RelativeTime value={p.updated_at} className="text-ink-muted" /> },
          {
            key: 'actions',
            header: '',
            align: 'right',
            render: (p) => (
              <span className="inline-flex gap-1" onClick={(e) => e.stopPropagation()}>
                <Button size="xs" variant="ghost" icon={<Download />} aria-label="Download" onClick={() => download(p)} />
                {isAdmin && <Button size="xs" variant="ghost" icon={<Pencil />} aria-label="Edit" onClick={() => setEdit(p)} />}
              </span>
            ),
          },
        ]}
      />
      <UploadDialog open={uploadOpen} onClose={() => setUploadOpen(false)} />
      {edit && <EditDialog pkg={edit} onClose={() => setEdit(null)} />}
    </>
  )
}

function UploadDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [name, setName] = useState('')
  const [category, setCategory] = useState('')
  const [info, setInfo] = useState('')
  const [notes, setNotes] = useState('')
  const [priority, setPriority] = useState(10)
  const [progress, setProgress] = useState<number | null>(null)
  const [abort, setAbort] = useState<AbortController | null>(null)

  const reset = () => {
    setFile(null)
    setName('')
    setCategory('')
    setInfo('')
    setNotes('')
    setPriority(10)
    setProgress(null)
    setAbort(null)
  }
  const close = () => {
    abort?.abort()
    reset()
    onClose()
  }
  const pick = (f: File | null) => {
    setFile(f)
    if (f && !name) setName(f.name.replace(/\.(pkg|mpkg|dmg|zip)$/i, ''))
  }
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!file) return
    const form = new FormData()
    form.append('name', name)
    form.append('category', category)
    form.append('info', info)
    form.append('notes', notes)
    form.append('priority', String(priority))
    form.append('file', file, file.name)
    const ac = new AbortController()
    setAbort(ac)
    setProgress(0)
    try {
      await uploadWithProgress<Package>('/api/v1/packages', form, setProgress, ac.signal)
      toast.success('Package uploaded', `${file.name} (${formatBytes(file.size)})`)
      await invalidate(qk.packages, qk.dashboard)
      reset()
      onClose()
    } catch (err) {
      toast.error('Upload failed', errorMessage(err))
      setProgress(null)
      setAbort(null)
    }
  }
  const uploading = progress !== null
  return (
    <Modal open={open} onClose={close} title="Upload package" description="Supported: .pkg, .mpkg, .dmg, .zip. Large files are streamed; keep this dialog open until the upload completes." size="md">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <div
          onDragOver={(e) => e.preventDefault()}
          onDrop={(e) => {
            e.preventDefault()
            pick(e.dataTransfer.files?.[0] ?? null)
          }}
          onClick={() => !uploading && fileRef.current?.click()}
          className="flex cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed border-line-strong/70 px-4 py-8 text-center hover:border-accent-500 hover:bg-accent-50/40 dark:hover:bg-accent-950/30"
        >
          <Upload className="size-6 text-ink-faint" />
          {file ? (
            <>
              <p className="text-sm font-medium text-ink">{file.name}</p>
              <p className="text-xs text-ink-muted">{formatBytes(file.size)}</p>
            </>
          ) : (
            <>
              <p className="text-sm text-ink">Drop an installer here or click to browse</p>
              <p className="text-xs text-ink-muted">.pkg, .mpkg, .dmg or .zip</p>
            </>
          )}
          <input ref={fileRef} type="file" accept=".pkg,.mpkg,.dmg,.zip" className="hidden" onChange={(e) => pick(e.target.files?.[0] ?? null)} disabled={uploading} />
        </div>
        <FormGrid cols={2}>
          <Field label="Display name">
            <Input value={name} onChange={(e) => setName(e.target.value)} disabled={uploading} placeholder={file?.name} />
          </Field>
          <Field label="Category">
            <Input value={category} onChange={(e) => setCategory(e.target.value)} disabled={uploading} />
          </Field>
          <Field label="Priority" hint="Lower installs first within a policy.">
            <Input type="number" value={priority} onChange={(e) => setPriority(Number(e.target.value) || 0)} disabled={uploading} />
          </Field>
          <Field label="Information">
            <Input value={info} onChange={(e) => setInfo(e.target.value)} disabled={uploading} />
          </Field>
          <Field label="Notes" className="md:col-span-2">
            <Textarea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} disabled={uploading} />
          </Field>
        </FormGrid>
        {uploading && (
          <div className="flex flex-col gap-1.5">
            <ProgressBar value={progress} />
            <p className="text-xs text-ink-muted tabular-nums">{progress < 100 ? `Uploading… ${progress}%` : 'Processing on server…'}</p>
          </div>
        )}
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={close} data-shortcut="cancel">
            {uploading ? 'Cancel upload' : 'Cancel'}
          </Button>
          <Button type="submit" variant="primary" disabled={!file} loading={uploading} data-shortcut="save">
            Upload
          </Button>
        </div>
      </form>
    </Modal>
  )
}

function EditDialog({ pkg, onClose }: { pkg: Package; onClose: () => void }) {
  const toast = useToast()
  const invalidate = useInvalidate()
  const [form, setForm] = useState({ name: pkg.name, category: pkg.category, info: pkg.info, notes: pkg.notes, priority: pkg.priority })
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api.put(`/api/v1/packages/${pkg.id}`, { ...pkg, ...form })
      toast.success('Package updated')
      await invalidate(qk.packages)
      onClose()
    } catch (err) {
      toast.error('Update failed', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open onClose={onClose} title="Edit package" description={`${pkg.filename} · ${formatBytes(pkg.size_bytes)}`} size="md">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <FormGrid cols={2}>
          <Field label="Display name" required>
            <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </Field>
          <Field label="Category">
            <Input value={form.category} onChange={(e) => setForm({ ...form, category: e.target.value })} />
          </Field>
          <Field label="Priority">
            <Input type="number" value={form.priority} onChange={(e) => setForm({ ...form, priority: Number(e.target.value) || 0 })} />
          </Field>
          <Field label="Information">
            <Input value={form.info} onChange={(e) => setForm({ ...form, info: e.target.value })} />
          </Field>
          <Field label="Notes" className="md:col-span-2">
            <Textarea rows={3} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
          </Field>
          <Field label="SHA-256" className="md:col-span-2">
            <Input readOnly mono value={pkg.sha256} />
          </Field>
        </FormGrid>
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={onClose} data-shortcut="cancel">
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={busy} data-shortcut="save">
            Save
          </Button>
        </div>
      </form>
    </Modal>
  )
}
