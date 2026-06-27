import { useMutation } from '@tanstack/react-query'
import { http, SKIP_AUTH_HEADER } from '@/shared/api/http'
import { useAuthStore } from './authStore'
import type { AuthTokens, LoginRequest } from '@/shared/api/types'

/** 登录：POST /auth/login（security: []，无需 Bearer），成功落库 token + user。 */
export function useLogin() {
  const setTokens = useAuthStore((s) => s.setTokens)
  return useMutation({
    mutationFn: (body: LoginRequest) =>
      http.post<AuthTokens>('/auth/login', body, { headers: { [SKIP_AUTH_HEADER]: '1' } }),
    onSuccess: (tokens) => setTokens(tokens),
  })
}
