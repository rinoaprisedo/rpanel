import { describe, it, expect } from 'vitest'
import { loginSchema, resetPasswordSchema } from './validation'

describe('loginSchema', () => {
  it('accepts a valid email/password pair', () => {
    const result = loginSchema.safeParse({ email: 'user@example.com', password: 'secret' })
    expect(result.success).toBe(true)
  })

  it('rejects an invalid email', () => {
    const result = loginSchema.safeParse({ email: 'not-an-email', password: 'secret' })
    expect(result.success).toBe(false)
  })

  it('rejects an empty password', () => {
    const result = loginSchema.safeParse({ email: 'user@example.com', password: '' })
    expect(result.success).toBe(false)
  })
})

describe('resetPasswordSchema', () => {
  const strongPassword = 'Passw0rd!'

  it('accepts matching strong passwords', () => {
    const result = resetPasswordSchema.safeParse({ password: strongPassword, confirmPassword: strongPassword })
    expect(result.success).toBe(true)
  })

  it('rejects mismatched passwords', () => {
    const result = resetPasswordSchema.safeParse({ password: strongPassword, confirmPassword: 'Different1!' })
    expect(result.success).toBe(false)
  })

  it('rejects a password missing a symbol', () => {
    const result = resetPasswordSchema.safeParse({ password: 'Password0', confirmPassword: 'Password0' })
    expect(result.success).toBe(false)
  })
})
