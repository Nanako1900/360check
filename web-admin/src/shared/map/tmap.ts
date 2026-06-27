/**
 * 腾讯地图 GL SDK 的最小化类型门面 + 单例脚本加载器（§P4）。
 *
 * - 官方无 @types：这里只声明本项目用到的子集（Map / LatLng / MultiPolyline / MultiMarker /
 *   MarkerCluster / InfoWindow / 事件），避免 `any`。此 shim **不是** API 契约类型（API 类型一律走
 *   generated/openapi-types），故允许手写。
 * - SDK 通过 `<script>` 注入全局 `window.TMap`，**绝不进主包**（§10.1）。jsdom 无法真正加载脚本，
 *   因此 `loadTencentSdk` 设计为：若 `window.TMap` 已存在立即 resolve（测试注入 fake 即可），否则注入 script。
 */

/** GeoJSON 顺序 [lng, lat]；腾讯地图相反，统一在 toLatLng 适配（§4.3 坑）。 */
export interface LatLng {
  readonly lat: number
  readonly lng: number
  getLat(): number
  getLng(): number
}

export interface LatLngBounds {
  extend(latLng: LatLng): LatLngBounds
  isEmpty(): boolean
}

/** new TMap.LatLng(lat, lng) —— 注意 lat 在前。 */
export type LatLngConstructor = new (lat: number, lng: number) => LatLng
export type LatLngBoundsConstructor = new () => LatLngBounds

export interface TMapMap {
  setCenter(latLng: LatLng): void
  setZoom(zoom: number): void
  fitBounds(bounds: LatLngBounds, options?: { padding?: number }): void
  destroy(): void
  on(eventName: string, handler: (...args: unknown[]) => void): void
  off(eventName: string, handler: (...args: unknown[]) => void): void
}

export interface MapOptions {
  center: LatLng
  zoom?: number
  /** 限制可缩放/拖拽等，可选透传。 */
  [key: string]: unknown
}

export type MapConstructor = new (container: HTMLElement, options: MapOptions) => TMapMap

/** 几何图层基类（MultiPolyline / MultiMarker）：可 setMap(null) 卸载、可 destroy。 */
export interface MapLayer {
  setMap(map: TMapMap | null): void
  destroy?(): void
}

/** 单条折线几何项。 */
export interface PolylineGeometry {
  id?: string
  styleId?: string
  paths: LatLng[]
}

export interface MultiPolylineOptions {
  map: TMapMap
  styles?: Record<string, unknown>
  geometries: PolylineGeometry[]
}

export interface MultiPolyline extends MapLayer {
  setGeometries(geometries: PolylineGeometry[]): void
}

export type MultiPolylineConstructor = new (options: MultiPolylineOptions) => MultiPolyline

/** 单个标记几何项（携带业务 properties 以便点击回查）。 */
export interface MarkerGeometry {
  id: string
  styleId?: string
  position: LatLng
  properties?: Record<string, unknown>
}

export interface MultiMarkerOptions {
  map?: TMapMap | null
  styles?: Record<string, unknown>
  geometries: MarkerGeometry[]
}

export interface MarkerEvent {
  geometry?: MarkerGeometry
  latLng?: LatLng
}

export interface MultiMarker extends MapLayer {
  setGeometries(geometries: MarkerGeometry[]): void
  on(eventName: string, handler: (event: MarkerEvent) => void): void
  off(eventName: string, handler: (event: MarkerEvent) => void): void
}

export type MultiMarkerConstructor = new (options: MultiMarkerOptions) => MultiMarker

export interface MarkerClusterOptions {
  map: TMapMap
  geometries: MarkerGeometry[]
  enableDefaultStyle?: boolean
  minimumClusterSize?: number
  zoomOnClick?: boolean
  gridSize?: number
}

export interface MarkerCluster {
  setGeometries(geometries: MarkerGeometry[]): void
  destroy?(): void
  setMap?(map: TMapMap | null): void
  on?(eventName: string, handler: (...args: unknown[]) => void): void
}

export type MarkerClusterConstructor = new (options: MarkerClusterOptions) => MarkerCluster

export interface InfoWindowOptions {
  map: TMapMap
  position: LatLng
  content?: string
  offset?: { x: number; y: number }
  enableCustom?: boolean
}

export interface InfoWindow {
  open(): void
  close(): void
  setContent(content: string): void
  setPosition(position: LatLng): void
  destroy(): void
  on(eventName: string, handler: (...args: unknown[]) => void): void
}

export type InfoWindowConstructor = new (options: InfoWindowOptions) => InfoWindow

/** `window.TMap` 命名空间（仅本项目使用的构造器子集）。 */
export interface TMapNamespace {
  Map: MapConstructor
  LatLng: LatLngConstructor
  LatLngBounds: LatLngBoundsConstructor
  MultiPolyline: MultiPolylineConstructor
  MultiMarker: MultiMarkerConstructor
  MarkerCluster: MarkerClusterConstructor
  InfoWindow: InfoWindowConstructor
}

declare global {
  interface Window {
    TMap?: TMapNamespace
  }
}

const SDK_SCRIPT_ID = 'tencent-map-gl-sdk'
const SDK_TIMEOUT_MS = 15_000

/** 单例：同一会话只注入一次脚本；并发调用复用同一 Promise。 */
let sdkPromise: Promise<TMapNamespace> | null = null

function buildSdkUrl(key: string): string {
  return `https://map.qq.com/api/gljs?v=1.exp&key=${encodeURIComponent(key)}`
}

/**
 * 加载腾讯地图 GL SDK（带 key），返回全局 `window.TMap`。
 * - 若 `window.TMap` 已存在（测试注入 fake / 已加载）→ 立即 resolve。
 * - 否则注入 `<script>`，onload 后 resolve，onerror/超时 reject（页面降级处理）。
 */
export function loadTencentSdk(key: string): Promise<TMapNamespace> {
  if (typeof window !== 'undefined' && window.TMap) {
    return Promise.resolve(window.TMap)
  }
  if (sdkPromise) return sdkPromise

  sdkPromise = new Promise<TMapNamespace>((resolve, reject) => {
    if (typeof document === 'undefined') {
      reject(new Error('腾讯地图 SDK 仅可在浏览器环境加载'))
      return
    }
    if (!key) {
      reject(new Error('缺少腾讯地图 key（VITE_MAP_KEY）'))
      return
    }

    const existing = document.getElementById(SDK_SCRIPT_ID) as HTMLScriptElement | null
    const script = existing ?? document.createElement('script')
    let settled = false

    const timer = window.setTimeout(() => {
      if (settled) return
      settled = true
      sdkPromise = null
      reject(new Error('腾讯地图 SDK 加载超时'))
    }, SDK_TIMEOUT_MS)

    const onLoad = (): void => {
      if (settled) return
      window.clearTimeout(timer)
      if (window.TMap) {
        settled = true
        resolve(window.TMap)
      } else {
        settled = true
        sdkPromise = null
        reject(new Error('腾讯地图 SDK 已加载但未注入 window.TMap'))
      }
    }
    const onError = (): void => {
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      sdkPromise = null
      reject(new Error('腾讯地图 SDK 加载失败'))
    }

    script.addEventListener('load', onLoad)
    script.addEventListener('error', onError)

    if (!existing) {
      script.id = SDK_SCRIPT_ID
      script.async = true
      script.src = buildSdkUrl(key)
      document.head.appendChild(script)
    }
  })

  return sdkPromise
}

/** 仅供单测：复位单例缓存，避免跨用例污染。 */
export function __resetSdkSingleton(): void {
  sdkPromise = null
}
