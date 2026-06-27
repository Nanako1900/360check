/**
 * MapInfoWindow（§P4）：点击单个问题点 → `TMap.InfoWindow` 弹出详情卡。
 *
 * 内容含：缩略图占位（cover_media_id，P5 接 /media/{id}）、类型（type_label）、
 * 状态（status_label + status_color）、标题摘要，以及「看全景」（P5）「看详情」（P6）按钮。
 *
 * 安全：内容构建与转义在 infoWindowContent.ts（纯函数），绝不用 dangerouslySetInnerHTML。
 * 按钮通过 data-action 委托点击：InfoWindow 内容是注入 HTML，拿不到 React 合成事件，故在 document
 * 上挂监听把动作回调出去（不在注入 HTML 里写内联 JS）。
 */
import { useEffect, useRef } from 'react'
import type { ProblemFeatureProperties } from '@/shared/api/types'
import { buildInfoWindowContent, INFO_ACTION_ATTR } from './infoWindowContent'
import type { InfoWindowLabels } from './infoWindowContent'
import { wgsToTMapLatLng } from './toLatLng'
import type { InfoWindow, TMapMap, TMapNamespace } from './tmap'

export type { InfoWindowLabels } from './infoWindowContent'

export interface InfoWindowTarget {
  /** 问题点 WGS84 坐标 [lng, lat]。 */
  position: readonly [number, number]
  properties: ProblemFeatureProperties
}

interface MapInfoWindowProps {
  map: TMapMap
  TMap: TMapNamespace
  /** 当前选中的问题点；null 则关闭弹窗。 */
  target: InfoWindowTarget | null
  /** 文案（i18n 注入，便于测试与本地化）。 */
  labels: InfoWindowLabels
  onViewPanorama: (properties: ProblemFeatureProperties) => void
  onViewDetail: (properties: ProblemFeatureProperties) => void
  onClose?: () => void
}

/** InfoWindow 初始占位坐标（实际位置随 target 在打开时设置）。 */
const ORIGIN: readonly [number, number] = [0, 0]

/** 副作用组件：管理单一 InfoWindow 实例的开/关/内容与按钮事件委托。 */
export function MapInfoWindow({
  map,
  TMap,
  target,
  labels,
  onViewPanorama,
  onViewDetail,
  onClose,
}: MapInfoWindowProps): null {
  const infoRef = useRef<InfoWindow | null>(null)
  const targetRef = useRef<InfoWindowTarget | null>(null)
  targetRef.current = target

  // 创建/销毁 InfoWindow 实例（随 map 生命周期）。
  useEffect(() => {
    const info = new TMap.InfoWindow({
      map,
      position: wgsToTMapLatLng(ORIGIN, TMap),
      content: '',
      offset: { x: 0, y: -32 },
      enableCustom: true,
    })
    info.close()
    infoRef.current = info
    return () => {
      info.destroy()
      infoRef.current = null
    }
  }, [map, TMap])

  // 按钮点击委托：在 document 上监听 data-action（InfoWindow 内容是注入 HTML，拿不到 React 合成事件）。
  useEffect(() => {
    function handleClick(event: MouseEvent): void {
      const el = event.target as HTMLElement | null
      const btn = el?.closest?.(`[${INFO_ACTION_ATTR}]`)
      if (!btn) return
      const action = btn.getAttribute(INFO_ACTION_ATTR)
      const current = targetRef.current
      if (!current) return
      if (action === 'panorama') onViewPanorama(current.properties)
      if (action === 'detail') onViewDetail(current.properties)
    }
    document.addEventListener('click', handleClick)
    return () => document.removeEventListener('click', handleClick)
  }, [onViewPanorama, onViewDetail])

  // 打开/更新/关闭随 target 变化。
  useEffect(() => {
    const info = infoRef.current
    if (!info) return
    if (!target) {
      info.close()
      onClose?.()
      return
    }
    info.setPosition(wgsToTMapLatLng(target.position, TMap))
    info.setContent(buildInfoWindowContent(target.properties, labels))
    info.open()
  }, [target, labels, TMap, onClose])

  return null
}
