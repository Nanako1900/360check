/**
 * ProblemMarkerLayer（§P4）：`ProblemFeatureCollection` 的问题点 → `TMap.MultiMarker` + `MarkerCluster`。
 *
 * - 每个点经 `wgsToTMapLatLng` 统一转换（WGS84→GCJ-02→LatLng(lat,lng)）。
 * - MarkerCluster 聚合大量点位控性能；点击单点回调 `onSelect`（父级据此打开 InfoWindow）。
 * - 卸载 / features 变化时清理（setMap(null)/destroy）防泄漏。
 */
import { useEffect, useRef } from 'react'
import type { ProblemFeatureCollection, ProblemFeatureProperties } from '@/shared/api/types'
import { asLngLat, wgsToTMapLatLng } from './toLatLng'
import type { InfoWindowTarget } from './MapInfoWindow'
import type {
  MarkerCluster,
  MarkerEvent,
  MarkerGeometry,
  MultiMarker,
  TMapMap,
  TMapNamespace,
} from './tmap'

interface ProblemMarkerLayerProps {
  map: TMapMap
  TMap: TMapNamespace
  data: ProblemFeatureCollection | undefined
  /** 点击单个问题点时回调（携带坐标 + 业务属性），父级用于打开 InfoWindow。 */
  onSelect: (target: InfoWindowTarget) => void
}

const MARKER_STYLE_ID = 'problem'

interface ParsedFeature {
  geometry: MarkerGeometry
  position: [number, number]
  properties: ProblemFeatureProperties
}

/** 解析 FeatureCollection → 可绘制 marker 几何（丢弃畸形坐标）。 */
function parseFeatures(
  data: ProblemFeatureCollection | undefined,
  TMap: TMapNamespace,
): ParsedFeature[] {
  const features = data?.features ?? []
  const result: ParsedFeature[] = []
  for (const feature of features) {
    const position = asLngLat(feature.geometry?.coordinates ?? [])
    if (!position) continue
    const props = feature.properties
    result.push({
      position,
      properties: props,
      geometry: {
        id: String(props.id),
        styleId: MARKER_STYLE_ID,
        position: wgsToTMapLatLng(position, TMap),
        properties: { problemId: props.id },
      },
    })
  }
  return result
}

/** 副作用组件：不渲染 DOM，仅管理 marker/cluster 生命周期。 */
export function ProblemMarkerLayer({
  map,
  TMap,
  data,
  onSelect,
}: ProblemMarkerLayerProps): null {
  const markerRef = useRef<MultiMarker | null>(null)
  const clusterRef = useRef<MarkerCluster | null>(null)
  const onSelectRef = useRef(onSelect)
  onSelectRef.current = onSelect

  useEffect(() => {
    // 清理旧图层。
    if (markerRef.current) {
      markerRef.current.setMap(null)
      markerRef.current.destroy?.()
      markerRef.current = null
    }
    if (clusterRef.current) {
      clusterRef.current.setMap?.(null)
      clusterRef.current.destroy?.()
      clusterRef.current = null
    }

    const parsed = parseFeatures(data, TMap)
    if (parsed.length === 0) return

    const geometries = parsed.map((p) => p.geometry)
    const byId = new Map(parsed.map((p) => [p.geometry.id, p]))

    // MultiMarker 负责单点展示与点击；MarkerCluster 负责聚合视觉。
    const marker = new TMap.MultiMarker({
      map,
      styles: { [MARKER_STYLE_ID]: { width: 24, height: 32, anchor: { x: 12, y: 32 } } },
      geometries,
    })
    marker.on('click', (event: MarkerEvent) => {
      const id = event.geometry?.id
      if (!id) return
      const hit = byId.get(id)
      if (!hit) return
      onSelectRef.current({ position: hit.position, properties: hit.properties })
    })
    markerRef.current = marker

    clusterRef.current = new TMap.MarkerCluster({
      map,
      geometries,
      enableDefaultStyle: true,
      minimumClusterSize: 2,
      zoomOnClick: true,
      gridSize: 60,
    })

    return () => {
      if (markerRef.current) {
        markerRef.current.setMap(null)
        markerRef.current.destroy?.()
        markerRef.current = null
      }
      if (clusterRef.current) {
        clusterRef.current.setMap?.(null)
        clusterRef.current.destroy?.()
        clusterRef.current = null
      }
    }
  }, [map, TMap, data])

  return null
}
