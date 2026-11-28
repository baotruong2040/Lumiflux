import { z } from 'zod'

const publicUserSchema = z.object({
  id: z.string(),
  username: z.string(),
  displayName: z.string(),
  avatarUrl: z.string().nullish().transform((v) => v ?? null),
})

const channelSchema = z.object({
  id: z.string(),
  guildId: z.string(),
  name: z.string(),
  type: z.number(),
  topic: z.string().nullish().transform((v) => v ?? null),
  position: z.number(),
  createdAt: z.string(),
})

export const readyFrameSchema = z.object({
  t: z.literal('READY'),
  d: z.object({
    userId: z.string(),
    guildIds: z.array(z.string()),
    heartbeatIntervalMs: z.number().int().positive(),
  }),
})

export const heartbeatAckFrameSchema = z.object({
  t: z.literal('HEARTBEAT_ACK'),
  d: z.record(z.string(), z.unknown()).optional(),
})

export const errorFrameSchema = z.object({
  t: z.literal('ERROR'),
  d: z.object({
    code: z.string(),
    message: z.string(),
  }),
})

export const messageCreateFrameSchema = z.object({
  t: z.literal('MESSAGE_CREATE'),
  d: z.object({
    id: z.string(),
    channelId: z.string(),
    guildId: z.string(),
    author: publicUserSchema,
    content: z.string(),
    nonce: z.string().nullish().transform((v) => v ?? null),
    editedAt: z.string().nullish().transform((v) => v ?? null),
    createdAt: z.string(),
  }),
})

export const messageUpdateFrameSchema = z.object({
  t: z.literal('MESSAGE_UPDATE'),
  d: z.object({
    id: z.string(),
    channelId: z.string(),
    guildId: z.string(),
    content: z.string(),
    editedAt: z.string().nullish().transform((v) => v ?? null),
  }),
})

export const messageDeleteFrameSchema = z.object({
  t: z.literal('MESSAGE_DELETE'),
  d: z.object({
    id: z.string(),
    channelId: z.string(),
    guildId: z.string(),
  }),
})

export const channelCreateFrameSchema = z.object({
  t: z.literal('CHANNEL_CREATE'),
  d: z.object({
    guildId: z.string(),
    channel: channelSchema,
  }),
})

export const channelUpdateFrameSchema = z.object({
  t: z.literal('CHANNEL_UPDATE'),
  d: z.object({
    guildId: z.string(),
    channel: channelSchema,
  }),
})

export const channelDeleteFrameSchema = z.object({
  t: z.literal('CHANNEL_DELETE'),
  d: z.object({
    guildId: z.string(),
    channel: channelSchema,
  }),
})

export const guildMemberAddFrameSchema = z.object({
  t: z.literal('GUILD_MEMBER_ADD'),
  d: z.object({
    guildId: z.string(),
    user: publicUserSchema,
    role: z.enum(['owner', 'admin', 'member']).optional().default('member'),
    nickname: z.string().nullish().transform((v) => v ?? null),
    joinedAt: z.string().optional(),
  }),
})

export const guildMemberRemoveFrameSchema = z.object({
  t: z.literal('GUILD_MEMBER_REMOVE'),
  d: z.object({
    guildId: z.string(),
    user: publicUserSchema,
  }),
})

export const guildDeleteFrameSchema = z.object({
  t: z.literal('GUILD_DELETE'),
  d: z.object({
    guildId: z.string(),
  }),
})

export const presenceUpdateFrameSchema = z.object({
  t: z.literal('PRESENCE_UPDATE'),
  d: z.object({
    userId: z.string(),
    status: z.enum(['online', 'offline']),
  }),
})

export const userTypingFrameSchema = z.object({
  t: z.literal('USER_TYPING'),
  d: z.object({
    channelId: z.string(),
    guildId: z.string(),
    userId: z.string(),
    username: z.string(),
    timestamp: z.number(),
  }),
})

export const gatewayEventSchema = z.discriminatedUnion('t', [
  readyFrameSchema,
  heartbeatAckFrameSchema,
  errorFrameSchema,
  messageCreateFrameSchema,
  messageUpdateFrameSchema,
  messageDeleteFrameSchema,
  channelCreateFrameSchema,
  channelUpdateFrameSchema,
  channelDeleteFrameSchema,
  guildMemberAddFrameSchema,
  guildMemberRemoveFrameSchema,
  guildDeleteFrameSchema,
  presenceUpdateFrameSchema,
  userTypingFrameSchema,
])

export type GatewayEvent = z.infer<typeof gatewayEventSchema>

export type GatewayOutboundFrame =
  | { t: 'HEARTBEAT' }
  | { t: 'TYPING_START'; d: { channelId: string } }

export function parseGatewayFrame(raw: string): GatewayEvent | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  const result = gatewayEventSchema.safeParse(parsed)
  if (!result.success) {
    return null
  }
  return result.data
}
