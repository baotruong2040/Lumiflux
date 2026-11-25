import { z } from 'zod'
import { ApiError } from '../../../core/api'

export const guildNameSchema = z
  .string()
  .trim()
  .min(1, 'Tên server không được để trống')
  .max(100, 'Tên server tối đa 100 ký tự')

export const optionalUrlSchema = z
  .string()
  .trim()
  .refine(
    (val) => {
      if (!val) return true
      if (val.length > 2048) return false
      try {
        const parsed = new URL(val)
        return parsed.protocol === 'http:' || parsed.protocol === 'https:'
      } catch {
        return false
      }
    },
    { message: 'URL phải bắt đầu bằng http:// hoặc https://' },
  )

export const createGuildSchema = z.object({
  name: guildNameSchema,
  iconUrl: optionalUrlSchema,
})

export const editGuildSchema = z.object({
  name: guildNameSchema,
  iconUrl: optionalUrlSchema,
})

export const joinGuildSchema = z.object({
  code: z
    .string()
    .trim()
    .min(1, 'Vui lòng nhập mã mời'),
})

export const channelFormSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Tên kênh không được để trống')
    .max(100, 'Tên kênh tối đa 100 ký tự'),
  topic: z
    .string()
    .trim()
    .max(255, 'Mô tả kênh tối đa 255 ký tự'),
})

export function getInitials(name: string): string {
  const words = name
    .trim()
    .split(/\s+/)
    .filter(Boolean)
  if (words.length === 0) return '?'
  if (words.length === 1) {
    return words[0].slice(0, 2).toUpperCase()
  }
  return `${words[0][0] ?? ''}${words[1][0] ?? ''}`.toUpperCase()
}

export function extractInviteCode(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  const parts = trimmed.split('/').filter(Boolean)
  return parts.length > 0 ? parts[parts.length - 1] : trimmed
}

export function formatWorkspaceError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'CHANNEL_NAME_TAKEN':
        return 'Tên kênh đã tồn tại trong server này.'
      case 'FORBIDDEN':
        return 'Bạn không có quyền thực hiện thao tác này.'
      case 'INVITE_EXPIRED':
        return 'Mã mời đã hết hạn sử dụng.'
      case 'INVITE_EXHAUSTED':
        return 'Mã mời đã đạt giới hạn lượt tham gia.'
      case 'INVITE_NOT_FOUND':
        return 'Mã mời không tồn tại hoặc đã bị thu hồi.'
      case 'ALREADY_MEMBER':
        return 'Bạn đã là thành viên của server này rồi.'
      case 'GUILD_NOT_FOUND':
        return 'Server không tồn tại hoặc đã bị xóa.'
      case 'CHANNEL_NOT_FOUND':
        return 'Kênh không tồn tại hoặc đã bị xóa.'
      case 'VALIDATION_ERROR':
        return err.message || 'Dữ liệu nhập vào chưa hợp lệ.'
      default:
        return err.message || 'Đã xảy ra lỗi không xác định.'
    }
  }
  if (err instanceof Error && err.message) {
    return err.message
  }
  return 'Không thể kết nối tới máy chủ API.'
}
