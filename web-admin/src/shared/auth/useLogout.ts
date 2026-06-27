import { useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import { useAuthStore } from './authStore'

/** 登出：POST /auth/logout 撤销 refresh + 清 store + 清查询缓存。后端失败也要清本地会话。 */
export function useLogout() {
  const reset = useAuthStore((s) => s.reset)
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      try {
        await http.post<null>('/auth/logout')
      } catch {
        // 即使后端登出失败，本地会话也必须清理
      }
    },
    onSettled: () => {
      reset()
      queryClient.clear()
    },
  })
}
