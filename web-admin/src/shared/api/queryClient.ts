import { QueryCache, MutationCache, QueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { ApiError, isAuthFailure, toUserMessage } from './apiError'

/** 不重试的错误码（鉴权/校验/冲突类重试无意义）。 */
const NON_RETRYABLE: ReadonlySet<string> = new Set([
  'UNAUTHENTICATED',
  'TOKEN_EXPIRED',
  'FORBIDDEN',
  'VALIDATION_FAILED',
  'NOT_FOUND',
  'CONFLICT',
])

/** 全局错误兜底：弹友好提示；鉴权类交给会话总线处理（重定向），不再额外 toast。 */
function notifyError(error: unknown): void {
  if (isAuthFailure(error)) return
  message.error(toUserMessage(error))
}

export function createQueryClient(): QueryClient {
  return new QueryClient({
    queryCache: new QueryCache({ onError: notifyError }),
    mutationCache: new MutationCache({ onError: notifyError }),
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        gcTime: 5 * 60_000,
        refetchOnWindowFocus: false,
        retry: (failureCount, error) => {
          if (error instanceof ApiError && NON_RETRYABLE.has(error.code)) return false
          return failureCount < 2
        },
      },
      mutations: {
        retry: false,
      },
    },
  })
}
