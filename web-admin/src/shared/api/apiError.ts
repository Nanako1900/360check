/**
 * 错误码契约（§4.6 / 契约 00 错误码目录）—— 冻结闭集，必须与生成的 ErrorCode 逐字一致。
 * 注意：是 `UNAUTHENTICATED` 而非 `UNAUTHORIZED`。
 * 与 generated ErrorCode 的一致性由 `types.ts` 的编译期断言守护。
 */
export type ApiErrorCode =
  | 'VALIDATION_FAILED'
  | 'UNAUTHENTICATED'
  | 'TOKEN_EXPIRED'
  | 'FORBIDDEN'
  | 'NOT_FOUND'
  | 'CONFLICT'
  | 'MEDIA_VERIFY_FAILED'
  | 'DICT_VERSION_RETIRED'
  | 'RATE_LIMITED'
  | 'INTERNAL'

export interface ApiErrorDetail {
  field: string
  code: string
  message: string
}

/** 业务异常：承载 code / message / details[] / httpStatus（§4.2 拦截器职责 2）。 */
export class ApiError extends Error {
  readonly code: ApiErrorCode
  readonly details: ApiErrorDetail[]
  readonly httpStatus: number | undefined

  constructor(
    code: ApiErrorCode,
    message: string,
    details: ApiErrorDetail[] = [],
    httpStatus?: number,
  ) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.details = details
    this.httpStatus = httpStatus
    // 维持 instanceof 在编译到 ES5/继承内置类时仍可用
    Object.setPrototypeOf(this, ApiError.prototype)
  }

  static is(error: unknown): error is ApiError {
    return error instanceof ApiError
  }

  /** 字段级错误映射（VALIDATION_FAILED），便于表单按 field 回填（§4.6）。 */
  fieldErrors(): Record<string, string> {
    const out: Record<string, string> = {}
    for (const d of this.details) {
      if (d.field && !(d.field in out)) out[d.field] = d.message
    }
    return out
  }
}

/** 面向用户的友好中文兜底文案（不泄露后端细节，§7）。 */
const FALLBACK_MESSAGE: Record<ApiErrorCode, string> = {
  VALIDATION_FAILED: '提交的数据未通过校验，请检查后重试',
  UNAUTHENTICATED: '登录已失效，请重新登录',
  TOKEN_EXPIRED: '登录状态已过期，正在尝试恢复',
  FORBIDDEN: '没有权限执行此操作',
  NOT_FOUND: '请求的资源不存在或已被删除',
  CONFLICT: '操作与当前数据状态冲突，请刷新后重试',
  MEDIA_VERIFY_FAILED: '媒体校验未通过，请重试上传',
  DICT_VERSION_RETIRED: '字典版本已退役',
  RATE_LIMITED: '操作过于频繁，请稍后再试',
  INTERNAL: '服务异常，请稍后重试',
}

/** 把任意 unknown 错误规整为用户可读消息（优先后端 message，否则兜底）。 */
export function toUserMessage(error: unknown): string {
  if (error instanceof ApiError) {
    // INTERNAL/500 的后端 message 可能含内部细节（栈/SQL/panic）→ 始终用兜底文案（§7 不泄露后端内部）。
    if (error.code === 'INTERNAL') return FALLBACK_MESSAGE.INTERNAL
    return error.message || FALLBACK_MESSAGE[error.code]
  }
  if (error instanceof Error && error.message) return error.message
  return FALLBACK_MESSAGE.INTERNAL
}

/** 是否为「需登出/重定向」类错误（由会话总线统一处理，不再额外弹 toast）。 */
export function isAuthFailure(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    (error.code === 'UNAUTHENTICATED' || error.code === 'TOKEN_EXPIRED')
  )
}
