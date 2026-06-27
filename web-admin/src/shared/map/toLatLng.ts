/**
 * 坐标适配层（§4.3 强约束）：把后端 WGS84 `[lng, lat]` 转成腾讯地图 `LatLng(lat, lng)`。
 *
 * 这是**唯一**做「WGS84→GCJ-02 + [lng,lat]→(lat,lng) 顺序翻转」的地方。
 * - WGS84↔GCJ-02 一律调 `coordTransform.ts`，禁止任何内联偏移公式。
 * - 腾讯地图 `new TMap.LatLng(lat, lng)` 入参是「纬度在前」，与 GeoJSON 相反，集中在此翻转，避免散落。
 * - GCJ-02 仅存在于渲染边界，绝不回传后端、绝不持久化。
 */
import { wgs84ToGcj02Tuple } from '@/shared/geo/coordTransform'
import type { LatLng, LatLngConstructor } from './tmap'

/** TMap 命名空间中本适配器需要的最小子集（便于测试注入仅含 LatLng 的 fake）。 */
export interface LatLngFactory {
  LatLng: LatLngConstructor
}

/**
 * 单点：WGS84 `[lng, lat]` → GCJ-02 → `new TMap.LatLng(lat, lng)`。
 * 注意顺序：先转 GCJ-02，再以「纬度, 经度」构造 LatLng。
 */
export function wgsToTMapLatLng(point: readonly [number, number], TMap: LatLngFactory): LatLng {
  const [gcjLng, gcjLat] = wgs84ToGcj02Tuple(point)
  return new TMap.LatLng(gcjLat, gcjLng)
}

/** 折线/路径：WGS84 `[lng, lat][]` → `LatLng[]`，逐点经唯一适配（顺序与转换一致）。 */
export function wgsPathToLatLng(
  coords: ReadonlyArray<readonly [number, number]>,
  TMap: LatLngFactory,
): LatLng[] {
  return coords.map((point) => wgsToTMapLatLng(point, TMap))
}

/** GeoJSON 坐标数组（number[]）窄化为 `[lng, lat]` 元组（边界校验，丢弃畸形点）。 */
export function asLngLat(coord: readonly number[]): [number, number] | null {
  if (coord.length < 2) return null
  const [lng, lat] = coord
  if (typeof lng !== 'number' || typeof lat !== 'number') return null
  if (Number.isNaN(lng) || Number.isNaN(lat)) return null
  return [lng, lat]
}
