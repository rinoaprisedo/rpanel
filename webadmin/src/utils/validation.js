import { z } from 'zod'

export const loginSchema = z.object({
  email: z.string().email('Enter a valid email'),
  password: z.string().min(1, 'Password is required'),
})

export const forgotPasswordSchema = z.object({
  email: z.string().email('Enter a valid email'),
})

export const resetPasswordSchema = z
  .object({
    password: z
      .string()
      .min(8, 'Min 8 characters')
      .regex(/[A-Z]/, 'Must contain an uppercase letter')
      .regex(/[0-9]/, 'Must contain a number')
      .regex(/[^A-Za-z0-9]/, 'Must contain a symbol'),
    confirmPassword: z.string(),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  })

export const userSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  email: z.string().email('Enter a valid email'),
  password: z.string().optional().or(z.literal('')),
  status: z.enum(['active', 'inactive', 'suspended']),
  role_uuid: z.string().optional().or(z.literal('')),
})

export const roleSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  description: z.string().optional().or(z.literal('')),
})

export const menuSectionSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  icon: z.string().optional().or(z.literal('')),
  order: z.union([z.string(), z.number()]).optional(),
})

export const menuSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  icon: z.string().optional().or(z.literal('')),
  path: z.string().min(1, 'Path is required'),
  menu_section_uuid: z.string().min(1, 'Section is required'),
  order: z.union([z.string(), z.number()]).optional(),
  is_active: z.boolean().optional(),
})
