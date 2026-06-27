import { describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import { createQueryClient } from './queryClient'
import { ApiError } from './apiError'

type RetryFn = (failureCount: number, error: unknown) => boolean

describe('createQueryClient', () => {
  it('does not retry non-retryable ApiError codes', () => {
    const qc = createQueryClient()
    const retry = qc.getDefaultOptions().queries?.retry as RetryFn
    expect(retry(0, new ApiError('VALIDATION_FAILED', ''))).toBe(false)
    expect(retry(0, new ApiError('FORBIDDEN', ''))).toBe(false)
    expect(retry(0, new ApiError('NOT_FOUND', ''))).toBe(false)
  })

  it('retries transient errors up to 2 attempts', () => {
    const qc = createQueryClient()
    const retry = qc.getDefaultOptions().queries?.retry as RetryFn
    expect(retry(0, new ApiError('INTERNAL', ''))).toBe(true)
    expect(retry(1, new ApiError('INTERNAL', ''))).toBe(true)
    expect(retry(2, new ApiError('INTERNAL', ''))).toBe(false)
  })

  it('toasts non-auth errors and stays silent for auth failures', () => {
    const spy = vi.spyOn(message, 'error').mockImplementation(() => ({}) as never)
    const qc = createQueryClient()
    const onError = qc.getQueryCache().config.onError
    onError?.(new ApiError('INTERNAL', 'boom'), {} as never)
    expect(spy).toHaveBeenCalledTimes(1)
    onError?.(new ApiError('UNAUTHENTICATED', 'x'), {} as never)
    expect(spy).toHaveBeenCalledTimes(1) // 鉴权类交给会话总线，不再额外 toast
    spy.mockRestore()
  })
})
