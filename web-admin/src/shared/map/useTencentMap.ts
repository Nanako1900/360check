/**
 * useTencentMap（§P4）：动态加载腾讯地图 GL SDK → 初始化 `TMap.Map` → 卸载时 `destroy()` 防泄漏。
 *
 * - SDK 经 `loadTencentSdk` 单例注入 `<script>`，绝不进主包（§10.1）。
 * - jsdom 不能真正加载脚本：测试只需在 `window.TMap` 注入 fake，hook 即可走到 'ready'。
 * - 加载/初始化失败 → status='error'，页面降级提示（地图非唯一查看路径，§10.2）。
 */
import { useEffect, useRef, useState } from 'react'
import { wgsToTMapLatLng } from './toLatLng'
import { loadTencentSdk } from './tmap'
import type { TMapMap, TMapNamespace } from './tmap'

export type TencentMapStatus = 'loading' | 'ready' | 'error'

export interface UseTencentMapOptions {
  /** SDK key（默认取 env.VITE_MAP_KEY，可在测试注入）。 */
  apiKey: string
  /** 初始中心 WGS84 [lng, lat]（默认杭州，无数据时占位）。 */
  center?: readonly [number, number]
  zoom?: number
}

export interface UseTencentMapResult {
  containerRef: React.RefObject<HTMLDivElement | null>
  map: TMapMap | null
  TMap: TMapNamespace | null
  status: TencentMapStatus
  error: string | null
}

/** 默认中心：杭州（仅占位，真实视野由图层 fitBounds 覆盖）。 */
const DEFAULT_CENTER: readonly [number, number] = [120.21, 30.25]
const DEFAULT_ZOOM = 14

export function useTencentMap(options: UseTencentMapOptions): UseTencentMapResult {
  const { apiKey, center = DEFAULT_CENTER, zoom = DEFAULT_ZOOM } = options
  const containerRef = useRef<HTMLDivElement | null>(null)
  const mapRef = useRef<TMapMap | null>(null)
  const [map, setMap] = useState<TMapMap | null>(null)
  const [tmap, setTmap] = useState<TMapNamespace | null>(null)
  const [status, setStatus] = useState<TencentMapStatus>('loading')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function init(): Promise<void> {
      try {
        const TMap = await loadTencentSdk(apiKey)
        if (cancelled) return
        const container = containerRef.current
        if (!container) {
          setStatus('error')
          setError('地图容器未就绪')
          return
        }
        const instance = new TMap.Map(container, {
          center: wgsToTMapLatLng(center, TMap),
          zoom,
        })
        if (cancelled) {
          instance.destroy()
          return
        }
        mapRef.current = instance
        setTmap(TMap)
        setMap(instance)
        setStatus('ready')
        setError(null)
      } catch (err: unknown) {
        if (cancelled) return
        setStatus('error')
        setError(err instanceof Error ? err.message : '地图加载失败')
      }
    }

    void init()

    return () => {
      cancelled = true
      if (mapRef.current) {
        mapRef.current.destroy()
        mapRef.current = null
      }
    }
    // center/zoom 仅作初始值，变更不重建地图（避免抖动）；key 变更才重载。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiKey])

  return { containerRef, map, TMap: tmap, status, error }
}
