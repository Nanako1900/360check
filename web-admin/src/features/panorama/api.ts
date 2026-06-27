import { useQuery } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import type { MediaAsset } from '@/shared/api/types'

/** react-query key（按 media id）。 */
export const mediaKey = (id: number) => ['media', id] as const

/**
 * `GET /media/{id}`（§P5）：返回携带「该 tier 签名 CDN URL」的 MediaAsset。
 *
 * 签名 URL 有时效（§P5 坑）：PanoramaViewer 加载失败时调 onExpired → 上层调用本 hook 暴露的
 * `refetch()` 重新拉取，获得新的 `signed_url`。`staleTime: 0` 保证 refetch 一定打后端取新签名。
 */
export function useMedia(id: number, enabled = true) {
  return useQuery<MediaAsset>({
    queryKey: mediaKey(id),
    queryFn: () => http.get<MediaAsset>(`/media/${id}`),
    enabled: enabled && id > 0,
    staleTime: 0,
    gcTime: 0,
  })
}
