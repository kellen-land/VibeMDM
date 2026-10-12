import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { cn } from '../../lib/cn'
import { Spinner } from './Spinner'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'danger-outline'
export type ButtonSize = 'xs' | 'sm' | 'md' | 'lg'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  loading?: boolean
  icon?: ReactNode
  iconRight?: ReactNode
}

const variants: Record<ButtonVariant, string> = {
  primary:
    'glow-under-cyan bg-accent-500 text-[#03252d] font-semibold hover:bg-accent-400 active:bg-accent-600 border border-transparent disabled:bg-accent-500/50 disabled:shadow-none',
  secondary: 'bg-surface-raised text-ink border border-line-strong/70 hover:bg-surface-sunken hover:border-line-strong active:bg-line/50',
  ghost: 'bg-transparent text-ink-muted hover:text-ink hover:bg-line/40 border border-transparent',
  danger: 'bg-critical text-white font-semibold hover:brightness-110 active:brightness-95 border border-transparent',
  'danger-outline': 'bg-transparent text-critical border border-critical/45 hover:bg-critical/10',
}

const sizes: Record<ButtonSize, string> = {
  xs: 'h-7 px-2 text-xs gap-1 rounded-md',
  sm: 'h-8 px-2.5 text-[13px] gap-1.5 rounded-md',
  md: 'h-[34px] px-3.5 text-[13px] gap-2 rounded-md',
  lg: 'h-10 px-5 text-sm gap-2 rounded-lg',
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', loading, icon, iconRight, className, children, disabled, type = 'button', ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      disabled={disabled || loading}
      className={cn(
        'inline-flex items-center justify-center font-medium whitespace-nowrap select-none transition-colors focus-ring disabled:opacity-55 disabled:cursor-not-allowed',
        variants[variant],
        sizes[size],
        className,
      )}
      {...rest}
    >
      {loading ? <Spinner className="size-4" /> : icon ? <span className="shrink-0 [&>svg]:size-4">{icon}</span> : null}
      {children}
      {iconRight ? <span className="shrink-0 [&>svg]:size-4">{iconRight}</span> : null}
    </button>
  )
})

export function IconButton({ label, className, size = 'sm', children, ...rest }: ButtonProps & { label: string }) {
  return (
    <Button
      aria-label={label}
      title={label}
      size={size}
      className={cn('px-0', size === 'xs' ? 'w-7' : size === 'sm' ? 'w-8' : 'w-[34px]', className)}
      {...rest}
    >
      <span className="[&>svg]:size-4">{children}</span>
    </Button>
  )
}
