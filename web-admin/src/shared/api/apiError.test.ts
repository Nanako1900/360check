import { describe, expect, it } from 'vitest'
import { ApiError, isAuthFailure, toUserMessage } from './apiError'

describe('ApiError', () => {
  it('carries code / message / details / httpStatus and is an Error', () => {
    const e = new ApiError(
      'VALIDATION_FAILED',
      'bad',
      [{ field: 'x', code: 'R', message: 'm' }],
      422,
    )
    expect(e).toBeInstanceOf(Error)
    expect(e.code).toBe('VALIDATION_FAILED')
    expect(e.httpStatus).toBe(422)
    expect(e.details).toHaveLength(1)
    expect(ApiError.is(e)).toBe(true)
    expect(ApiError.is(new Error('x'))).toBe(false)
  })

  it('fieldErrors keeps the first message per field', () => {
    const e = new ApiError('VALIDATION_FAILED', 'x', [
      { field: 'a', code: '', message: 'm1' },
      { field: 'a', code: '', message: 'm2' },
      { field: 'b', code: '', message: 'mb' },
    ])
    expect(e.fieldErrors()).toEqual({ a: 'm1', b: 'mb' })
  })

  it('toUserMessage prefers backend message then falls back by code', () => {
    expect(toUserMessage(new ApiError('FORBIDDEN', ''))).toBe('没有权限执行此操作')
    expect(toUserMessage(new ApiError('FORBIDDEN', '自定义'))).toBe('自定义')
    expect(toUserMessage(new Error('boom'))).toBe('boom')
    expect(toUserMessage('weird')).toBe('服务异常，请稍后重试')
  })

  it('toUserMessage never surfaces a raw INTERNAL/500 backend message (§7)', () => {
    expect(toUserMessage(new ApiError('INTERNAL', 'pq: relation "x" does not exist'))).toBe(
      '服务异常，请稍后重试',
    )
  })

  it('isAuthFailure is true only for auth codes', () => {
    expect(isAuthFailure(new ApiError('UNAUTHENTICATED', ''))).toBe(true)
    expect(isAuthFailure(new ApiError('TOKEN_EXPIRED', ''))).toBe(true)
    expect(isAuthFailure(new ApiError('INTERNAL', ''))).toBe(false)
    expect(isAuthFailure(new Error('x'))).toBe(false)
  })
})
