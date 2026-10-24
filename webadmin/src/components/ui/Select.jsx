import { forwardRef } from 'react'
import { ChevronDown } from 'lucide-react'
import { cn } from '../../utils/cn'

const Select = forwardRef(function Select({ label, error, className, children, ...props }, ref) {
  return (
    <div className="flex flex-col gap-1">
      {label && (
        <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">{label}</label>
      )}
      <div className="relative">
        <select
          ref={ref}
          className={cn(
            'h-[42px] w-full appearance-none rounded-md border-[1.5px] border-surface-border bg-white px-3 pr-9 text-sm text-text-primary outline-none transition-colors duration-150 focus:border-primary',
            error && 'border-danger',
            className,
          )}
          {...props}
        >
          {children}
        </select>
        <ChevronDown
          size={16}
          strokeWidth={1.7}
          className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-text-tertiary"
        />
      </div>
      {error && <span className="text-xs text-danger">{error}</span>}
    </div>
  )
})

export default Select
