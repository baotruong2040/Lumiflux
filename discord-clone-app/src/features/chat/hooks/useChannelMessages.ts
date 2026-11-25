import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import { messageApi } from '../../../core/api'
import type { User } from '../../../core/api'
import type { ChatMessage } from '../types'
import {
  flattenPagesChronologically,
  formatChatError,
  generateNonce,
  markOptimisticFailed,
  removeMessageFromPages,
  resolveOptimisticMessage,
  updateMessageInPages,
  upsertOptimisticMessage,
} from '../utils/messageHelpers'
import type { MessagesInfiniteData } from '../utils/messageHelpers'

const PAGE_LIMIT = 50

export function useChannelMessages(channelId: string | null, currentUser: User | null) {
  const queryClient = useQueryClient()
  const queryKey = useMemo(() => ['messages', channelId] as const, [channelId])

  const messagesQuery = useInfiniteQuery({
    queryKey,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      if (!channelId) return []
      return messageApi.listMessages(channelId, {
        before: pageParam,
        limit: PAGE_LIMIT,
      })
    },
    getNextPageParam: (lastPage) => {
      if (lastPage.length < PAGE_LIMIT) {
        return undefined
      }
      return lastPage[lastPage.length - 1]?.id
    },
    enabled: Boolean(channelId && currentUser),
  })

  const messages = useMemo(
    () => flattenPagesChronologically(messagesQuery.data?.pages),
    [messagesQuery.data?.pages],
  )

  async function sendWithNonce(trimmedContent: string, existingNonce?: string): Promise<void> {
    if (!channelId || !currentUser) return
    const nonce = existingNonce ?? generateNonce()

    const optimisticMsg: ChatMessage = {
      id: `optimistic-${nonce}`,
      channelId,
      author: {
        id: currentUser.id,
        username: currentUser.username,
        displayName: currentUser.displayName,
        avatarUrl: currentUser.avatarUrl,
      },
      content: trimmedContent,
      nonce,
      editedAt: null,
      createdAt: new Date().toISOString(),
      optimisticStatus: 'sending',
    }

    queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
      upsertOptimisticMessage(old, optimisticMsg),
    )

    try {
      const created = await messageApi.sendMessage(channelId, {
        content: trimmedContent,
        nonce,
      })
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        resolveOptimisticMessage(old, nonce, created),
      )
    } catch (err) {
      const errorMessage = formatChatError(err)
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        markOptimisticFailed(old, nonce, errorMessage),
      )
      throw err
    }
  }

  const editMutation = useMutation({
    mutationFn: ({ messageId, content }: { messageId: string; content: string }) => {
      if (!channelId) {
        return Promise.reject(new Error('Missing active channel'))
      }
      return messageApi.updateMessage(channelId, messageId, { content })
    },
    onSuccess: (updated) => {
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        updateMessageInPages(old, updated),
      )
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (messageId: string) => {
      if (!channelId) {
        return Promise.reject(new Error('Missing active channel'))
      }
      return messageApi.deleteMessage(channelId, messageId)
    },
    onSuccess: (_, messageId) => {
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        removeMessageFromPages(old, messageId),
      )
    },
  })

  function dismissFailedMessage(idOrNonce: string) {
    queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
      removeMessageFromPages(old, idOrNonce),
    )
  }

  return {
    messages,
    isLoading: messagesQuery.isLoading,
    isError: messagesQuery.isError,
    error: messagesQuery.error,
    hasNextPage: Boolean(messagesQuery.hasNextPage),
    isFetchingNextPage: messagesQuery.isFetchingNextPage,
    fetchNextPage: messagesQuery.fetchNextPage,
    refetch: messagesQuery.refetch,
    sendMessage: (content: string) => sendWithNonce(content),
    retryMessage: (msg: ChatMessage) => sendWithNonce(msg.content, msg.nonce ?? undefined),
    dismissFailedMessage,
    editMessage: (messageId: string, content: string) =>
      editMutation.mutateAsync({ messageId, content }),
    isEditing: editMutation.isPending,
    deleteMessage: (messageId: string) => deleteMutation.mutateAsync(messageId),
    isDeleting: deleteMutation.isPending,
  }
}
