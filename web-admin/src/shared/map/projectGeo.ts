/**
 * 项目地理工具（纯函数，§P1 地图总览）：从 `Project.area_geom`（GeoJSONMultiPolygon, WGS84）
 * 提取外环、质心、包围盒，供 ProjectAreaLayer 描范围 / 放标记 / fitBounds 使用。
 *
 * GeoJSON 坐标一律 `[lng, lat]`；WGS84→GCJ-02 的转换只在 `toLatLng` 做，本文件不碰坐标系。
 */
import type { Project } from '@/shared/api/types'
import { asLngLat } from './toLatLng'

export type LngLat = [number, number]

/**
 * 取 MultiPolygon 每个多边形的外环（`rings[0]`），返回外环点集数组。
 * 丢弃畸形点；点数 <2 的环忽略（画不出线）。
 */
export function outerRings(project: Project): LngLat[][] {
  const coords = project.area_geom?.coordinates
  if (!coords) return []
  const rings: LngLat[][] = []
  for (const polygon of coords) {
    const outer = polygon?.[0]
    if (!outer) continue
    const ring: LngLat[] = []
    for (const pt of outer) {
      const ll = asLngLat(pt)
      if (ll) ring.push(ll)
    }
    if (ring.length >= 2) rings.push(ring)
  }
  return rings
}

/** 项目所有外环点（扁平），用于质心 / 包围盒。 */
export function projectPoints(project: Project): LngLat[] {
  return outerRings(project).flat()
}

/** 项目是否带可绘制的地理范围。 */
export function hasArea(project: Project): boolean {
  return outerRings(project).length > 0
}

/**
 * 简单质心：外环点算术平均（够用于放标记 / 飞行定位；非严格多边形质心）。
 * 空集返回 null。
 */
export function centroid(points: readonly LngLat[]): LngLat | null {
  if (points.length === 0) return null
  let sx = 0
  let sy = 0
  for (const [lng, lat] of points) {
    sx += lng
    sy += lat
  }
  return [sx / points.length, sy / points.length]
}

export interface LngLatBounds {
  minLng: number
  minLat: number
  maxLng: number
  maxLat: number
}

/** 计算一组点的包围盒；空集返回 null。 */
export function boundsOf(points: readonly LngLat[]): LngLatBounds | null {
  if (points.length === 0) return null
  let minLng = Number.POSITIVE_INFINITY
  let minLat = Number.POSITIVE_INFINITY
  let maxLng = Number.NEGATIVE_INFINITY
  let maxLat = Number.NEGATIVE_INFINITY
  for (const [lng, lat] of points) {
    if (lng < minLng) minLng = lng
    if (lng > maxLng) maxLng = lng
    if (lat < minLat) minLat = lat
    if (lat > maxLat) maxLat = lat
  }
  return { minLng, minLat, maxLng, maxLat }
}

/** 闭合外环（首点 != 末点时把首点补到末尾），用于用折线描出封闭轮廓。 */
export function closeRing(ring: readonly LngLat[]): LngLat[] {
  if (ring.length < 2) return [...ring]
  const first = ring[0]
  const last = ring[ring.length - 1]
  if (first[0] === last[0] && first[1] === last[1]) return [...ring]
  return [...ring, first]
}
