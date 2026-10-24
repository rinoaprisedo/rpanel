import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
import { useMutation } from '@tanstack/react-query'
import Input from '../../components/ui/Input'
import Button from '../../components/ui/Button'
import { login, me } from '../../api/auth'
import { loginSchema } from '../../utils/validation'
import useAuthStore from '../../store/authStore'

function Login() {
  const navigate = useNavigate()
  const setUser = useAuthStore((s) => s.setUser)
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm({ resolver: zodResolver(loginSchema) })

  const [creds, setCreds] = useState(null) // set once the password was accepted and a 2FA code is needed
  const [otp, setOtp] = useState('')

  const mutation = useMutation({
    mutationFn: login,
    onSuccess: async (res, vars) => {
      if (res?.data?.two_factor_required) {
        setCreds({ email: vars.email, password: vars.password })
        return
      }
      try {
        const res = await me()
        setUser(res.data)
      } catch {
        // ignore; guard will redirect if session invalid
      }
      toast.success('Welcome back!')
      navigate('/')
    },
    onError: (err) => {
      toast.error(err.response?.data?.message || 'Login failed')
      if (creds) setOtp('')
    },
  })

  if (creds) {
    return (
      <div>
        <h2 className="mb-1 text-center text-lg font-semibold text-text-primary">Two-factor authentication</h2>
        <p className="mb-4 text-center text-sm text-text-secondary">Enter the 6-digit code from your authenticator app, or a recovery code.</p>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            mutation.mutate({ ...creds, otp })
          }}
        >
          <Input label="Code" autoFocus autoComplete="one-time-code" inputMode="numeric" placeholder="123456" value={otp} onChange={(e) => setOtp(e.target.value)} />
          <Button type="submit" disabled={mutation.isPending || !otp.trim()} className="w-full">
            {mutation.isPending ? 'Verifying...' : 'Verify'}
          </Button>
          <button type="button" className="text-sm text-text-secondary hover:underline" onClick={() => { setCreds(null); setOtp('') }}>
            Back
          </button>
        </form>
      </div>
    )
  }

  return (
    <div>
      <h2 className="mb-4 text-center text-lg font-semibold text-text-primary">
        Sign in to your account
      </h2>
      <form className="flex flex-col gap-4" onSubmit={handleSubmit((data) => mutation.mutate(data))}>
        <Input label="Email" type="email" placeholder="you@example.com" error={errors.email?.message} {...register('email')} />
        <Input label="Password" type="password" placeholder="••••••••" error={errors.password?.message} {...register('password')} />
        <Button type="submit" disabled={mutation.isPending} className="w-full">
          {mutation.isPending ? 'Signing in...' : 'Sign in'}
        </Button>
      </form>
    </div>
  )
}

export default Login
