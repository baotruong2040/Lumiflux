import { z } from 'zod'
import { ApiError } from '../../../core/api'

const USERNAME_REGEX = /^[a-zA-Z0-9._-]{3,32}$/

function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

export const loginSchema = z.object({
  login: z.string().trim().min(1, 'Vui lòng nhập username hoặc email'),
  password: z.string().min(1, 'Vui lòng nhập mật khẩu'),
})

export const registerSchema = z.object({
  username: z
    .string()
    .trim()
    .regex(
      USERNAME_REGEX,
      'Username dài 3–32 ký tự, chỉ gồm chữ cái, số, ".", "_" hoặc "-"',
    ),
  email: z
    .string()
    .trim()
    .max(254, 'Email không được vượt quá 254 ký tự')
    .email('Địa chỉ email không hợp lệ'),
  displayName: z
    .string()
    .trim()
    .min(1, 'Vui lòng nhập tên hiển thị')
    .max(64, 'Tên hiển thị tối đa 64 ký tự'),
  password: z
    .string()
    .min(8, 'Mật khẩu phải có ít nhất 8 ký tự')
    .refine(
      (val) => utf8ByteLength(val) <= 72,
      'Mật khẩu không được vượt quá 72 byte (giới hạn bcrypt)',
    ),
})

export type LoginFormValues = z.infer<typeof loginSchema>
export type RegisterFormValues = z.infer<typeof registerSchema>

export function formatAuthErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'INVALID_CREDENTIALS':
        return 'Sai tên đăng nhập/email hoặc mật khẩu.'
      case 'RATE_LIMITED':
        return err.retryAfter
          ? `Bạn đã thử quá nhiều lần. Vui lòng đợi ${err.retryAfter} giây rồi thử lại.`
          : 'Bạn đã gửi quá nhiều yêu cầu. Vui lòng thử lại sau ít phút.'
      case 'USERNAME_TAKEN':
        return 'Username này đã được sử dụng.'
      case 'EMAIL_TAKEN':
        return 'Email này đã được đăng ký.'
      case 'VALIDATION_ERROR':
        return err.message
      default:
        return err.message || 'Đã có lỗi xảy ra, vui lòng thử lại.'
    }
  }

  if (err instanceof Error) {
    return err.message
  }

  return 'Không thể kết nối tới máy chủ. Vui lòng kiểm tra lại.'
}
