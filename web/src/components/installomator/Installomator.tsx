import { useAppLabels, useInstallomatorPreview } from '../../api/queries'
import { errorMessage } from '../../api/client'
import type { InstallomatorChoices, InstallomatorOptions, InstallomatorPreviewRequest } from '../../api/types'
import { useDebounced } from '../../lib/hooks'
import { cn } from '../../lib/cn'
import { Field } from '../ui/Field'
import { Select } from '../ui/Input'
import { Toggle } from '../ui/Checkbox'
import { CopyButton } from '../ui/CopyButton'
import { Spinner } from '../ui/Spinner'

const optionFields: { key: Exclude<keyof InstallomatorChoices, 'type'>; label: string; hint: string }[] = [
  { key: 'notify', label: 'Notify', hint: 'NOTIFY: when the user sees Installomator notifications.' },
  { key: 'blocking_process_action', label: 'Blocking process action', hint: 'BLOCKING_PROCESS_ACTION: what to do if the app is running.' },
  { key: 'reopen', label: 'Reopen', hint: 'REOPEN: relaunch the app after updating if it was running.' },
  { key: 'ignore_app_store_apps', label: 'Ignore App Store apps', hint: 'IGNORE_APP_STORE_APPS: replace a Mac App Store copy when "no".' },
]

/** Humanize an option value: prompt_user_then_kill -> "prompt user then kill". */
const human = (v: string) => v.replace(/_/g, ' ')

/** Installomator runtime options. Empty selects leave Installomator's own default in place. */
export function InstallomatorOptionsEditor({ value, onChange, disabled, compact }: { value: InstallomatorOptions; onChange: (v: InstallomatorOptions) => void; disabled?: boolean; compact?: boolean }) {
  const labels = useAppLabels()
  const choices = labels.data?.choices
  const set = (patch: Partial<InstallomatorOptions>) => onChange({ ...value, ...patch })
  return (
    <div className="flex flex-col gap-4">
      <div className={cn('grid gap-4', compact ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-4' : 'grid-cols-1 md:grid-cols-2')}>
        {optionFields.map((f) => {
          const opts = choices?.[f.key] ?? []
          const cur = value[f.key] ?? ''
          return (
            <Field key={f.key} label={f.label} hint={compact ? undefined : f.hint}>
              <Select
                value={cur}
                onChange={(e) => set({ [f.key]: e.target.value } as Partial<InstallomatorOptions>)}
                disabled={disabled}
                title={compact ? f.hint : undefined}
                options={[{ value: '', label: 'Default' }, ...opts.map((o) => ({ value: o, label: human(o) })), ...(cur && !opts.includes(cur) ? [{ value: cur, label: cur }] : [])]}
              />
            </Field>
          )
        })}
      </div>
      <Toggle
        size={compact ? 'sm' : 'md'}
        checked={value.force_install}
        onChange={(v) => set({ force_install: v })}
        disabled={disabled}
        label="Force install"
        description="INSTALL=force: reinstall even when the installed version is already current."
      />
    </div>
  )
}

/**
 * Live preview of the exact Installomator command. The body is debounced, so pass the raw form state. A validation
 * error from the server is shown inline.
 */
export function InstallomatorCommandPreview({ body, className, emptyHint = 'Fill in the form to see the command.' }: { body: InstallomatorPreviewRequest | null; className?: string; emptyHint?: string }) {
  const debounced = useDebounced(body ? JSON.stringify(body) : '', 400)
  const q = useInstallomatorPreview(debounced ? (JSON.parse(debounced) as InstallomatorPreviewRequest) : null)
  const pending = !!body && (debounced !== JSON.stringify(body) || q.isFetching)
  return (
    <div className={cn('rounded-lg border border-line bg-surface-sunken', className)}>
      <div className="flex items-center justify-between gap-2 border-b border-line px-3 py-1.5">
        <span className="inline-flex items-center gap-2 text-xs text-ink-muted">
          Command preview {pending && <Spinner className="size-3" />}
        </span>
        {q.data && !q.error && <CopyButton text={q.data.command} size="xs" variant="ghost" />}
      </div>
      <div className="px-3 py-2.5">
        {!body ? (
          <p className="text-xs text-ink-faint">{emptyHint}</p>
        ) : q.error ? (
          <p className="whitespace-pre-wrap text-xs text-critical">{errorMessage(q.error)}</p>
        ) : q.data ? (
          <pre className="whitespace-pre-wrap break-all font-mono text-xs leading-relaxed text-ink">{q.data.command}</pre>
        ) : (
          <p className="text-xs text-ink-faint">Generating…</p>
        )}
      </div>
    </div>
  )
}
