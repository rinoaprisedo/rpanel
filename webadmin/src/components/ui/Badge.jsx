import { cn } from '../../utils/cn'

const variants = {
  success: 'bg-success-bg text-success',
  danger: 'bg-danger-bg text-danger',
  warning: 'bg-warning-bg text-warning',
  primary: 'bg-info-bg text-info',
  neutral: 'bg-surface-hover text-text-secondary',
}

function Badge({ variant = 'neutral', children, className }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium capitalize',
        variants[variant],
        className,
      )}
    >
      {children}
    </span>
  )
}

export default Badge
