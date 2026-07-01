/**
 * fitBoundsToPoints（§P1）：把地图视野收束到一组 WGS84 点的包围盒。
 *
 * 用 `TMap.LatLngBounds` 逐点 `extend`（点先经 `toLatLng` 唯一转换），再 `map.fitBounds`。
 * 对缺失实现（测试 fake / SDK 降级）容错：返回 false 表示未 fit（调用方可忽略）。
 */
import type { LngLat } from './projectGeo'
import { wgsToTMapLatLng } from './toLatLng'
import type { TMapMap, TMapNamespace } from './tmap'

export function fitBoundsToPoints(
  map: TMapMap,
  TMap: TMapNamespace,
  points: readonly LngLat[],
  padding = 48,
): boolean {
  if (points.length === 0) return false
  try {
    const bounds = new TMap.LatLngBounds()
    if (typeof bounds.extend !== 'function') return false
    for (const p of points) bounds.extend(wgsToTMapLatLng(p, TMap))
    map.fitBounds(bounds, { padding })
    return true
  } catch {
    return false
  }
}
