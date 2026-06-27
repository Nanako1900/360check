/**
 * usePanoMarkers（§P5，可选）：在全景球内放置标记（如问题位置、视角注记）。
 *
 * 极简封装 `@photo-sphere-viewer/markers-plugin`：把 { id, yaw, pitch, tooltip } 列表同步到插件。
 * 与 PanoramaViewer 同样懒加载，且仅在传入 plugin 实例后才操作；本 hook 不持有 Viewer 生命周期。
 *
 * 注意：插件实例由调用方通过 `viewer.getPlugin('markers')` 取得后传入（Viewer 需在创建时注册插件），
 * 故此 hook 保持纯同步副作用，便于在测试中以 fake plugin 注入。
 */
import { useEffect } from 'react'

/** 全景内单个标记（球面角度，单位由 PSV 解析，可 '30deg' 字符串或弧度数值）。 */
export interface PanoMarker {
  id: string
  yaw: number | string
  pitch: number | string
  /** 悬浮提示文案（已转义的纯文本）。 */
  tooltip?: string
  /** HTML 内容（调用方负责转义）。缺省用一个小圆点。 */
  html?: string
}

/** PSV MarkersPlugin 的最小化结构子集（仅本 hook 使用的方法）。 */
export interface PanoMarkersPluginLike {
  clearMarkers(render?: boolean): void
  addMarker(config: {
    id: string
    position: { yaw: number | string; pitch: number | string }
    tooltip?: string
    html?: string
    size?: { width: number; height: number }
  }): void
}

const DEFAULT_DOT =
  '<div style="width:14px;height:14px;border-radius:50%;background:#1677ff;border:2px solid #fff"></div>'

/** 把 markers 列表同步到插件（清空后重建）。plugin 为 null 时空操作。 */
export function usePanoMarkers(
  plugin: PanoMarkersPluginLike | null | undefined,
  markers: readonly PanoMarker[],
): void {
  useEffect(() => {
    if (!plugin) return
    plugin.clearMarkers(false)
    for (const m of markers) {
      plugin.addMarker({
        id: m.id,
        position: { yaw: m.yaw, pitch: m.pitch },
        tooltip: m.tooltip,
        html: m.html ?? DEFAULT_DOT,
        size: { width: 18, height: 18 },
      })
    }
  }, [plugin, markers])
}
