import { Search, X } from 'lucide-react'
import { Input, type InputProps } from './Input'
import { cn } from '../../lib/cn'

export function SearchInput({ value, onChange, className, placeholder = 'Search…', ...rest }: Omit<InputProps, 'onChange' | 'value'> & { value: string; onChange: (v: string) => void }) {
  return (
    <div className={cn('relative', className)}>
      <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-ink-faint" />
      <Input type="search" value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className="pl-8 pr-8 [&::-webkit-search-cancel-button]:hidden" {...rest} />
      {value && (
        <button type="button" onClick={() => onChange('')} className="absolute right-2 top-1/2 -translate-y-1/2 text-ink-faint hover:text-ink" aria-label="Clear search">
          <X className="size-4" />
        </button>
      )}
    </div>
  )
}
