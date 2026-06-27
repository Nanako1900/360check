/**
 * InfoWindow 安全内容构建（§P4 安全约束）：纯函数，与组件分离（便于单测 + 满足 fast-refresh）。
 *
 * InfoWindow 只接受 HTML 字符串，故所有用户文本（title/type_label/status_label）一律 `escapeHtml` 转义，
 * 绝不使用 dangerouslySetInnerHTML，绝不拼接未转义内容。
 */
import type { ProblemFeatureProperties } from '@/shared/api/types'

export interface InfoWindowLabels {
  type: string
  status: string
  viewPanorama: string
  viewDetail: string
  noThumb: string
  untitled: string
}

/** InfoWindow 内按钮的动作标记（事件委托读取 data-* 时复用）。 */
export const INFO_ACTION_ATTR = 'data-map-info-action'

/** HTML 转义：防 XSS（type_label/status_label/title 等可能含特殊字符）。 */
export function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/** 仅允许 # 开头的十六进制色值，否则回退中性色（避免把任意字符串塞进 style）。 */
export function safeColor(color: string | null | undefined): string {
  if (color && /^#[0-9a-fA-F]{3,8}$/.test(color)) return color
  return '#64748b'
}

/** 构建 InfoWindow 的安全 HTML 字符串（全部文本转义）。导出供单测断言转义生效。 */
export function buildInfoWindowContent(
  properties: ProblemFeatureProperties,
  labels: InfoWindowLabels,
): string {
  const title = properties.title ? escapeHtml(properties.title) : escapeHtml(labels.untitled)
  const typeLabel = properties.type_label ? escapeHtml(properties.type_label) : '—'
  const statusLabel = properties.status_label ? escapeHtml(properties.status_label) : '—'
  const statusColor = safeColor(properties.status_color)
  const hasCover = properties.cover_media_id != null
  const thumb = hasCover
    ? `<div class="map-info__thumb" aria-hidden="true">#${properties.cover_media_id}</div>`
    : `<div class="map-info__thumb map-info__thumb--empty">${escapeHtml(labels.noThumb)}</div>`

  return [
    '<div class="map-info">',
    thumb,
    '<div class="map-info__body">',
    `<div class="map-info__title">${title}</div>`,
    '<div class="map-info__meta">',
    `<span class="map-info__chip">${escapeHtml(labels.type)}：${typeLabel}</span>`,
    `<span class="map-info__chip" style="border-color:${statusColor};color:${statusColor}">${escapeHtml(labels.status)}：${statusLabel}</span>`,
    '</div>',
    '<div class="map-info__actions">',
    `<button type="button" ${INFO_ACTION_ATTR}="panorama" class="map-info__btn">${escapeHtml(labels.viewPanorama)}</button>`,
    `<button type="button" ${INFO_ACTION_ATTR}="detail" class="map-info__btn map-info__btn--primary">${escapeHtml(labels.viewDetail)}</button>`,
    '</div>',
    '</div>',
    '</div>',
  ].join('')
}
