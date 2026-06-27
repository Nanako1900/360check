/**
 * 媒体 tier 纯函数选择逻辑（§P5）。无 React、无副作用，便于单测。
 *
 * 契约（openapi `MediaAsset`）：
 * - 仅渲染 **已确认** 媒体：`verified_at` 非空（= `capture_state === 'CONFIRMED'`）。
 * - `signed_url` 为「**当前这一 tier**」的签名 CDN URL（由 `GET /media/{id}` 注入），有时效。
 * - `tier='web'` 为 2:1 等距柱状 JPG（全景球渲染源）；`tier='thumb'` 用于导航缩略图；
 *   `original` Web 不渲染（仅下载用，见 D4）。
 *
 * tier 选择说明：后端按 `media_group` 派生 `web`/`thumb` 兄弟行（D4），每行各自携带其 tier 的
 * `signed_url`。Web 不在客户端拼 URL、不派生 tier；要拿哪一 tier 就用其对应 media id 调
 * `GET /media/{id}`，响应里的 `signed_url` 即该 tier 的可渲染地址。故下方的 `pickWebUrl` /
 * `pickThumbUrl` 仅在「传入的 media 本身就是目标 tier」时返回其 `signed_url`，否则返回 null
 * —— 不臆造其它 tier 的 URL。
 */
import type { MediaAsset } from '@/shared/api/types'

/** 媒体是否可渲染：已确认（verified_at 非空）且携带本 tier 的签名 URL。 */
export function isRenderable(media: MediaAsset | null | undefined): boolean {
  if (!media) return false
  if (media.verified_at === null || media.verified_at === undefined) return false
  return typeof media.signed_url === 'string' && media.signed_url.length > 0
}

/**
 * 取全景球渲染 URL：仅当 media 为 `web` tier 且可渲染时返回其 `signed_url`，否则 null。
 * 不跨 tier 推断地址（Web 不派生/不拼 URL）。
 */
export function pickWebUrl(media: MediaAsset | null | undefined): string | null {
  if (!media || media.tier !== 'web') return null
  return isRenderable(media) ? (media.signed_url ?? null) : null
}

/** 取缩略图导航 URL：仅当 media 为 `thumb` tier 且可渲染时返回其 `signed_url`，否则 null。 */
export function pickThumbUrl(media: MediaAsset | null | undefined): string | null {
  if (!media || media.tier !== 'thumb') return null
  return isRenderable(media) ? (media.signed_url ?? null) : null
}
