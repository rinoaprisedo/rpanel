import { cn } from '../../utils/cn'

const variants = {
  primary: 'bg-primary text-white hover:opacity-[.88]',
  secondary: 'bg-surface-card text-text-secondary border border-surface-border hover:bg-surface-hover hover:border-surface-border-hover',
  danger: 'bg-danger text-white hover:opacity-[.88]',
  ghost: 'bg-transparent text-text-primary hover:bg-surface-hover',
}

function Button({ variant = 'primary', className, children, disabled, ...props }) {
  return (
    <button
      disabled={disabled}
      className={cn(
        'inline-flex h-[38px] items-center justify-center gap-2 rounded-md px-4 text-[13px] font-medium transition-colors duration-150 active:scale-[.98] disabled:opacity-50 disabled:cursor-not-allowed',
        variants[variant],
        className,
      )}
      {...props}
    >
      {children}
    </button>
  )
}

export default Button
