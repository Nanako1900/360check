import { useMutation } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import type { ChangePasswordRequest } from '@/shared/api/types'

/** 修改自身密码：PUT /auth/password（旧密码 + 新密码）。 */
export function useChangePassword() {
  return useMutation({
    mutationFn: (body: ChangePasswordRequest) => http.put<null>('/auth/password', body),
  })
}
