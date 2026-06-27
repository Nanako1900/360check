/**
 * TrajectoryLayer（§P4）：把巡查 `route`（GeoJSON LineString，WGS84）画成 `TMap.MultiPolyline`。
 *
 * - 用 MultiPolyline（而非单 Polyline）便于多段扩展（断点续巡）。
 * - 每个顶点经 `wgsPathToLatLng` 统一转换（WGS84→GCJ-02→LatLng(lat,lng)），不在本组件内联任何坐标逻辑。
 * - 可叠加起/终点 marker（MultiMarker）。
 * - 卸载 / route 变化时清理图层（setMap(null)/destroy）防泄漏。
 */
import { useEffect, useRef } from 'react'
import { asLngLat, wgsPathToLatLng, wgsToTMapLatLng } from './toLatLng'
import type { GeoJSONLineString } from '@/shared/api/types'
import type {
  MarkerGeometry,
  MultiMarker,
  MultiPolyline,
  TMapMap,
  TMapNamespace,
} from './tmap'

interface TrajectoryLayerProps {
  map: TMapMap
  TMap: TMapNamespace
  /** 巡查路线（WGS84 LineString）；缺省 / 空则不绘制。 */
  route: GeoJSONLineString | undefined
  /** 是否叠加起终点 marker（默认 true）。 */
  showEndpoints?: boolean
  /** 折线样式（颜色/宽度），透传给 TMap。 */
  color?: string
  width?: number
}

const POLYLINE_STYLE_ID = 'route'
const ENDPOINT_STYLE_ID = 'endpoint'

/** 副作用组件：不渲染 DOM，仅管理地图图层生命周期。 */
export function TrajectoryLayer({
  map,
  TMap,
  route,
  showEndpoints = true,
  color = '#1677ff',
  width = 5,
}: TrajectoryLayerProps): null {
  const polylineRef = useRef<MultiPolyline | null>(null)
  const markerRef = useRef<MultiMarker | null>(null)

  useEffect(() => {
    // 先清理旧图层（route 变更或重渲染）。
    if (polylineRef.current) {
      polylineRef.current.setMap(null)
      polylineRef.current.destroy?.()
      polylineRef.current = null
    }
    if (markerRef.current) {
      markerRef.current.setMap(null)
      markerRef.current.destroy?.()
      markerRef.current = null
    }

    const coords = (route?.coordinates ?? [])
      .map(asLngLat)
      .filter((c): c is [number, number] => c !== null)
    if (coords.length < 2) return

    const path = wgsPathToLatLng(coords, TMap)
    polylineRef.current = new TMap.MultiPolyline({
      map,
      styles: {
        [POLYLINE_STYLE_ID]: { color, width, borderWidth: 0, lineCap: 'round' },
      },
      geometries: [{ id: 'route-0', styleId: POLYLINE_STYLE_ID, paths: path }],
    })

    if (showEndpoints) {
      const start = coords[0]
      const end = coords[coords.length - 1]
      const endpointGeometries: MarkerGeometry[] = [
        { id: 'start', styleId: ENDPOINT_STYLE_ID, position: wgsToTMapLatLng(start, TMap) },
        { id: 'end', styleId: ENDPOINT_STYLE_ID, position: wgsToTMapLatLng(end, TMap) },
      ]
      markerRef.current = new TMap.MultiMarker({
        map,
        styles: { [ENDPOINT_STYLE_ID]: { width: 20, height: 28, anchor: { x: 10, y: 28 } } },
        geometries: endpointGeometries,
      })
    }

    return () => {
      if (polylineRef.current) {
        polylineRef.current.setMap(null)
        polylineRef.current.destroy?.()
        polylineRef.current = null
      }
      if (markerRef.current) {
        markerRef.current.setMap(null)
        markerRef.current.destroy?.()
        markerRef.current = null
      }
    }
  }, [map, TMap, route, showEndpoints, color, width])

  return null
}
