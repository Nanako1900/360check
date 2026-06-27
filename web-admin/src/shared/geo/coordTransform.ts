/**
 * ★ 唯一 WGS84 ↔ GCJ-02 转换工具（§4.3 强约束）。
 *
 * - 后端永远 WGS84/SRID 4326；GCJ-02 绝不持久化、绝不回传后端。
 * - 仅在「腾讯地图渲染边界」使用：读后端 WGS84 → `wgs84ToGcj02` 绘制；地图取点 → `gcj02ToWgs84` 提交。
 * - 输入/输出统一 `[lng, lat]`（与 GeoJSON 一致）；腾讯地图 `LatLng(lat, lng)` 的顺序适配只在 map 封装层做。
 * - 纯函数、无副作用。任何组件都不得内联坐标转换公式。
 *
 * 算法：标准 GCJ-02（火星坐标）偏移。境外点位不偏移（与后端约定一致）。
 */

// Krasovsky 1940 椭球参数
const GCJ_A = 6378245.0
const GCJ_EE = 0.006_693_421_622_965_943

/** 经纬度元组（GeoJSON 顺序：[经度, 纬度]）。 */
export type LngLat = readonly [number, number]

/** 是否在中国境外（粗略外包框）。境外不做 GCJ-02 偏移。 */
export function outOfChina(lng: number, lat: number): boolean {
  if (lng < 72.004 || lng > 137.8347) return true
  if (lat < 0.8293 || lat > 55.8271) return true
  return false
}

function transformLat(x: number, y: number): number {
  let ret = -100.0 + 2.0 * x + 3.0 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * Math.sqrt(Math.abs(x))
  ret += ((20.0 * Math.sin(6.0 * x * Math.PI) + 20.0 * Math.sin(2.0 * x * Math.PI)) * 2.0) / 3.0
  ret += ((20.0 * Math.sin(y * Math.PI) + 40.0 * Math.sin((y / 3.0) * Math.PI)) * 2.0) / 3.0
  ret +=
    ((160.0 * Math.sin((y / 12.0) * Math.PI) + 320 * Math.sin((y * Math.PI) / 30.0)) * 2.0) / 3.0
  return ret
}

function transformLng(x: number, y: number): number {
  let ret = 300.0 + x + 2.0 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * Math.sqrt(Math.abs(x))
  ret += ((20.0 * Math.sin(6.0 * x * Math.PI) + 20.0 * Math.sin(2.0 * x * Math.PI)) * 2.0) / 3.0
  ret += ((20.0 * Math.sin(x * Math.PI) + 40.0 * Math.sin((x / 3.0) * Math.PI)) * 2.0) / 3.0
  ret +=
    ((150.0 * Math.sin((x / 12.0) * Math.PI) + 300.0 * Math.sin((x / 30.0) * Math.PI)) * 2.0) / 3.0
  return ret
}

/** 计算某点的 GCJ-02 相对 WGS84 偏移量 [dLng, dLat]（度）。 */
function offset(lng: number, lat: number): [number, number] {
  let dLat = transformLat(lng - 105.0, lat - 35.0)
  let dLng = transformLng(lng - 105.0, lat - 35.0)
  const radLat = (lat / 180.0) * Math.PI
  let magic = Math.sin(radLat)
  magic = 1 - GCJ_EE * magic * magic
  const sqrtMagic = Math.sqrt(magic)
  dLat = (dLat * 180.0) / (((GCJ_A * (1 - GCJ_EE)) / (magic * sqrtMagic)) * Math.PI)
  dLng = (dLng * 180.0) / ((GCJ_A / sqrtMagic) * Math.cos(radLat) * Math.PI)
  return [dLng, dLat]
}

/** WGS84 → GCJ-02（读后端坐标 → 腾讯地图绘制）。境外原样返回。 */
export function wgs84ToGcj02(lng: number, lat: number): LngLat {
  if (outOfChina(lng, lat)) return [lng, lat]
  const [dLng, dLat] = offset(lng, lat)
  return [lng + dLng, lat + dLat]
}

/**
 * GCJ-02 → WGS84（地图取点 → 提交后端）。境外原样返回。
 * 迭代逼近：往返误差 < 1e-9 度，满足 §4.3 的 < 1e-6 要求。
 */
export function gcj02ToWgs84(lng: number, lat: number): LngLat {
  if (outOfChina(lng, lat)) return [lng, lat]
  let wgsLng = lng
  let wgsLat = lat
  for (let i = 0; i < 6; i += 1) {
    const [gLng, gLat] = wgs84ToGcj02(wgsLng, wgsLat)
    const dLng = lng - gLng
    const dLat = lat - gLat
    wgsLng += dLng
    wgsLat += dLat
    if (Math.abs(dLng) < 1e-12 && Math.abs(dLat) < 1e-12) break
  }
  return [wgsLng, wgsLat]
}

/** 元组重载：方便对 `[lng, lat]` 直接转换。 */
export function wgs84ToGcj02Tuple(point: LngLat): LngLat {
  return wgs84ToGcj02(point[0], point[1])
}
export function gcj02ToWgs84Tuple(point: LngLat): LngLat {
  return gcj02ToWgs84(point[0], point[1])
}
