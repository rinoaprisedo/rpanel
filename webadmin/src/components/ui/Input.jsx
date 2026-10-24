import { forwardRef, useState } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { cn } from '../../utils/cn'

const Input = forwardRef(function Input({ label, error, className, type, ...props }, ref) {
  const [showPassword, setShowPassword] = useState(false)
  const isPassword = type === 'password'

  return (
    <div className="flex flex-col gap-1">
      {label && (
        <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">{label}</label>
      )}
      <div className="relative">
        <input
          ref={ref}
          type={isPassword && showPassword ? 'text' : type}
          className={cn(
            'h-[42px] w-full rounded-md border-[1.5px] border-surface-border bg-field px-3 text-sm text-text-primary outline-none transition-colors duration-150 focus:border-primary',
            isPassword && 'pr-10',
            error && 'border-danger focus:border-danger',
            className,
          )}
          {...props}
        />
        {isPassword && (
          <button
            type="button"
            tabIndex={-1}
            onClick={() => setShowPassword((v) => !v)}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text-primary"
            aria-label={showPassword ? 'Hide password' : 'Show password'}
          >
            {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
          </button>
        )}
      </div>
      {error && <span className="text-xs text-danger">{error}</span>}
    </div>
  )
})

export default Input
