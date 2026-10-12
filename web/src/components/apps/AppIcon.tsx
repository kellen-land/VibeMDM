import { useEffect, useState } from 'react'
import { AppWindow } from 'lucide-react'
import { cn } from '../../lib/cn'

/** A Mac App's uploaded icon, or a neutral app glyph when it has none. */
export function AppIcon({ appId, version, className }: { appId: string; version?: string; className?: string }) {
  const [broken, setBroken] = useState(false)
  useEffect(() => setBroken(false), [version])
  if (!version || broken) return <AppWindow className={cn('size-4 text-ink-faint', className)} />
  return <img src={`/api/v1/apps/${appId}/icon?v=${encodeURIComponent(version)}`} alt="" className={cn('size-6 rounded-md object-contain', className)} onError={() => setBroken(true)} />
}
