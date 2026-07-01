/**
 * ProjectAreaLayer（§P1 地图总览）：把一批 `Project` 的 `area_geom` 画到地图上。
 *
 * - 外环轮廓 → `TMap.MultiPolyline`（描出项目地理范围，视觉，不接收点击）。
 * - 项目质心 → `TMap.MultiMarker`（可点击；点击回调 `onSelect(projectId)`，父级据此弹选中卡）。
 * - 首次绘制自动 `fitBounds` 到全部项目（仅一次，避免覆盖用户后续缩放）。
 * - 每点经 `toLatLng` 唯一转换（WGS84→GCJ-02→LatLng(lat,lng)）；卸载 / 数据变化时清理防泄漏。
 */
import { useEffect, useRef } from 'react'
import type { Project } from '@/shared/api/types'
import { centroid, closeRing, outerRings, projectPoints } from './projectGeo'
import type { LngLat } from './projectGeo'
import { fitBoundsToPoints } from './fitBounds'
import { wgsPathToLatLng, wgsToTMapLatLng } from './toLatLng'
import type {
  MarkerEvent,
  MarkerGeometry,
  MultiMarker,
  MultiPolyline,
  PolylineGeometry,
  TMapMap,
  TMapNamespace,
} from './tmap'

interface ProjectAreaLayerProps {
  map: TMapMap
  TMap: TMapNamespace
  projects: Project[]
  /** 点击项目质心标记时回调，参数为项目 id。 */
  onSelect: (projectId: number) => void
  /** 首次是否自动 fitBounds 到全部项目（默认 true）。 */
  autoFit?: boolean
}

const AREA_STYLE_ID = 'project-area'
const MARKER_STYLE_ID = 'project'

/** 副作用组件：不渲染 DOM，仅管理项目图层生命周期。 */
export function ProjectAreaLayer({
  map,
  TMap,
  projects,
  onSelect,
  autoFit = true,
}: ProjectAreaLayerProps): null {
  const lineRef = useRef<MultiPolyline | null>(null)
  const markerRef = useRef<MultiMarker | null>(null)
  const onSelectRef = useRef(onSelect)
  onSelectRef.current = onSelect
  const fittedRef = useRef(false)

  useEffect(() => {
    const cleanup = (): void => {
      if (lineRef.current) {
        lineRef.current.setMap(null)
        lineRef.current.destroy?.()
        lineRef.current = null
      }
      if (markerRef.current) {
        markerRef.current.setMap(null)
        markerRef.current.destroy?.()
        markerRef.current = null
      }
    }
    cleanup()

    const lineGeoms: PolylineGeometry[] = []
    const markerGeoms: MarkerGeometry[] = []
    const allPoints: LngLat[] = []

    for (const project of projects) {
      outerRings(project).forEach((ring, i) => {
        lineGeoms.push({
          id: `p${project.id}-r${i}`,
          styleId: AREA_STYLE_ID,
          paths: wgsPathToLatLng(closeRing(ring), TMap),
        })
        allPoints.push(...ring)
      })
      const c = centroid(projectPoints(project))
      if (c) {
        markerGeoms.push({
          id: String(project.id),
          styleId: MARKER_STYLE_ID,
          position: wgsToTMapLatLng(c, TMap),
          properties: { projectId: project.id },
        })
      }
    }

    if (lineGeoms.length > 0) {
      lineRef.current = new TMap.MultiPolyline({
        map,
        styles: { [AREA_STYLE_ID]: { color: 'rgba(22, 119, 255, 0.9)', width: 3, borderWidth: 0 } },
        geometries: lineGeoms,
      })
    }

    if (markerGeoms.length > 0) {
      const marker = new TMap.MultiMarker({
        map,
        styles: { [MARKER_STYLE_ID]: { width: 24, height: 32, anchor: { x: 12, y: 32 } } },
        geometries: markerGeoms,
      })
      marker.on('click', (event: MarkerEvent) => {
        const id = event.geometry?.id
        if (!id) return
        const projectId = Number(id)
        if (Number.isFinite(projectId)) onSelectRef.current(projectId)
      })
      markerRef.current = marker
    }

    if (autoFit && !fittedRef.current && allPoints.length > 0) {
      if (fitBoundsToPoints(map, TMap, allPoints)) fittedRef.current = true
    }

    return cleanup
  }, [map, TMap, projects, autoFit])

  return null
}
