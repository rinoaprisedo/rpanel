import { describe, it, expect } from 'vitest'
import { zodResolver } from '@hookform/resolvers/zod'
import { loginSchema } from './validation'

// Guards the zod 4 + @hookform/resolvers 5 upgrade: field errors must still reach the form.
describe('zodResolver with zod 4', () => {
  it('reports field errors and passes valid input', async () => {
    const r = zodResolver(loginSchema)
    const bad = await r({ email: 'nope', password: '' }, undefined, { fields: {}, shouldUseNativeValidation: false })
    expect(bad.errors.email.message).toBe('Enter a valid email')
    expect(bad.errors.password.message).toBe('Password is required')
    const ok = await r({ email: 'a@b.co', password: 'x' }, undefined, { fields: {}, shouldUseNativeValidation: false })
    expect(ok.errors).toEqual({})
  })
})
