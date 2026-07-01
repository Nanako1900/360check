/**
 * 测试用 fake 腾讯地图 GL（注入 window.TMap）。
 * jsdom 无法加载真实 SDK，loadTencentSdk 见 window.TMap 即 resolve，故注入此 fake 即可驱动 hook/图层。
 * 所有构造器用 vi.fn 记录调用，断言图层用「转换后坐标」构造。
 */
import { vi } from 'vitest'
import type { TMapNamespace } from '@/shared/map/tmap'
import { __resetSdkSingleton } from '@/shared/map/tmap'

export interface FakeTMapHandles {
  TMap: TMapNamespace
  LatLng: ReturnType<typeof vi.fn>
  Map: ReturnType<typeof vi.fn>
  MultiPolyline: ReturnType<typeof vi.fn>
  MultiMarker: ReturnType<typeof vi.fn>
  MarkerCluster: ReturnType<typeof vi.fn>
  InfoWindow: ReturnType<typeof vi.fn>
  /** 最近一次创建的 Map 实例的方法 spy。 */
  mapInstance: {
    setCenter: ReturnType<typeof vi.fn>
    setZoom: ReturnType<typeof vi.fn>
    fitBounds: ReturnType<typeof vi.fn>
    destroy: ReturnType<typeof vi.fn>
  } | null
  /** 最近一次创建的 MultiMarker 实例（用于触发 click）。 */
  markerInstance: FakeMarker | null
  /** 最近一次创建的 InfoWindow 实例。 */
  infoInstance: FakeInfoWindow | null
}

interface FakeMarker {
  setMap: ReturnType<typeof vi.fn>
  destroy: ReturnType<typeof vi.fn>
  setGeometries: ReturnType<typeof vi.fn>
  on: ReturnType<typeof vi.fn>
  off: ReturnType<typeof vi.fn>
  geometries: unknown[]
  fireClick: (id: string) => void
}

interface FakeInfoWindow {
  open: ReturnType<typeof vi.fn>
  close: ReturnType<typeof vi.fn>
  setContent: ReturnType<typeof vi.fn>
  setPosition: ReturnType<typeof vi.fn>
  destroy: ReturnType<typeof vi.fn>
  on: ReturnType<typeof vi.fn>
  lastContent: string
}

export function installFakeTMap(): FakeTMapHandles {
  const handles: FakeTMapHandles = {
    TMap: {} as TMapNamespace,
    LatLng: vi.fn(),
    Map: vi.fn(),
    MultiPolyline: vi.fn(),
    MultiMarker: vi.fn(),
    MarkerCluster: vi.fn(),
    InfoWindow: vi.fn(),
    mapInstance: null,
    markerInstance: null,
    infoInstance: null,
  }

  const LatLng = vi.fn(function (this: Record<string, unknown>, lat: number, lng: number) {
    this.lat = lat
    this.lng = lng
    this.getLat = () => lat
    this.getLng = () => lng
  })

  const Map = vi.fn(function (this: Record<string, unknown>) {
    const instance = {
      setCenter: vi.fn(),
      setZoom: vi.fn(),
      fitBounds: vi.fn(),
      destroy: vi.fn(),
      on: vi.fn(),
      off: vi.fn(),
    }
    handles.mapInstance = instance
    Object.assign(this, instance)
  })

  const MultiPolyline = vi.fn(function (this: Record<string, unknown>, options: { geometries: unknown[] }) {
    this.geometries = options.geometries
    this.setMap = vi.fn()
    this.destroy = vi.fn()
    this.setGeometries = vi.fn()
  })

  const MultiMarker = vi.fn(function (
    this: Record<string, unknown>,
    options: { geometries: unknown[] },
  ) {
    const listeners: Record<string, (event: { geometry?: { id: string } }) => void> = {}
    const marker: FakeMarker = {
      setMap: vi.fn(),
      destroy: vi.fn(),
      setGeometries: vi.fn(),
      on: vi.fn((name: string, handler: (event: { geometry?: { id: string } }) => void) => {
        listeners[name] = handler
      }),
      off: vi.fn(),
      geometries: options.geometries,
      fireClick: (id: string) => listeners.click?.({ geometry: { id } }),
    }
    handles.markerInstance = marker
    Object.assign(this, marker)
  })

  const MarkerCluster = vi.fn(function (this: Record<string, unknown>) {
    this.setGeometries = vi.fn()
    this.setMap = vi.fn()
    this.destroy = vi.fn()
    this.on = vi.fn()
  })

  const InfoWindow = vi.fn(function (this: Record<string, unknown>) {
    const info: FakeInfoWindow = {
      open: vi.fn(),
      close: vi.fn(),
      setContent: vi.fn(function (content: string) {
        info.lastContent = content
      }),
      setPosition: vi.fn(),
      destroy: vi.fn(),
      on: vi.fn(),
      lastContent: '',
    }
    handles.infoInstance = info
    Object.assign(this, info)
  })

  const LatLngBounds = vi.fn(function (this: Record<string, unknown>) {
    const pts: unknown[] = []
    this.extend = vi.fn((ll: unknown) => {
      pts.push(ll)
      return this
    })
    this.isEmpty = () => pts.length === 0
  })

  handles.LatLng = LatLng
  handles.Map = Map
  handles.MultiPolyline = MultiPolyline
  handles.MultiMarker = MultiMarker
  handles.MarkerCluster = MarkerCluster
  handles.InfoWindow = InfoWindow

  const TMap = {
    LatLng,
    LatLngBounds,
    Map,
    MultiPolyline,
    MultiMarker,
    MarkerCluster,
    InfoWindow,
  } as unknown as TMapNamespace

  handles.TMap = TMap
  window.TMap = TMap
  __resetSdkSingleton()
  return handles
}

export function uninstallFakeTMap(): void {
  delete window.TMap
  __resetSdkSingleton()
}
