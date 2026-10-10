import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { Button, type ButtonProps } from './Button'

export function CopyButton({ text, label = 'Copy', size = 'sm', variant = 'secondary', ...rest }: Omit<ButtonProps, 'onClick'> & { text: string; label?: string }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      const ta = document.createElement('textarea')
      ta.value = text
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      ta.remove()
    }
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <Button size={size} variant={variant} onClick={copy} icon={copied ? <Check /> : <Copy />} {...rest}>
      {copied ? 'Copied' : label}
    </Button>
  )
}

export function CodeBlock({ text, filename, className, maxHeight = '16rem' }: { text: string; filename?: string; className?: string; maxHeight?: string }) {
  return (
    <div className={`relative rounded-lg border border-line bg-surface-sunken ${className ?? ''}`}>
      <div className="flex items-center justify-between gap-2 border-b border-line px-3 py-1.5">
        <span className="text-xs text-ink-muted font-mono truncate">{filename ?? ''}</span>
        <CopyButton text={text} size="xs" variant="ghost" />
      </div>
      <pre className="overflow-auto p-3 text-xs font-mono leading-relaxed text-ink whitespace-pre-wrap break-all" style={{ maxHeight }}>
        {text}
      </pre>
    </div>
  )
}
