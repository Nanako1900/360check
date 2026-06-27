import { describe, expect, it, vi } from 'vitest'
import { wgs84ToGcj02Tuple } from '@/shared/geo/coordTransform'
import { asLngLat, wgsPathToLatLng, wgsToTMapLatLng } from './toLatLng'
import type { LatLngFactory } from './toLatLng'

// 北京（天安门附近）WGS84 [lng, lat]
const BEIJING: [number, number] = [116.404, 39.915]

/** 记录构造调用的 fake LatLng（捕获 (lat, lng) 入参顺序）。 */
function makeFakeTMap(): {
  TMap: LatLngFactory
  ctor: ReturnType<typeof vi.fn>
} {
  const ctor = vi.fn(function (this: { lat: number; lng: number }, lat: number, lng: number) {
    this.lat = lat
    this.lng = lng
  })
  const TMap = { LatLng: ctor as unknown as LatLngFactory['LatLng'] }
  return { TMap, ctor }
}

describe('toLatLng adapter (single source of WGS84→GCJ-02 + order swap)', () => {
  it('calls TMap.LatLng with (lat, lng) of the GCJ-02-converted point, proving conversion + order', () => {
    const { TMap, ctor } = makeFakeTMap()
    wgsToTMapLatLng(BEIJING, TMap)

    const [gcjLng, gcjLat] = wgs84ToGcj02Tuple(BEIJING)
    expect(ctor).toHaveBeenCalledTimes(1)
    // 入参第一位是「纬度」(≈39，<100)，第二位是「经度」(≈116，>100) —— 证明 lat-first 顺序翻转。
    const [argLat, argLng] = ctor.mock.calls[0]
    expect(argLat).toBeCloseTo(gcjLat, 9)
    expect(argLng).toBeCloseTo(gcjLng, 9)
    expect(argLat).toBeLessThan(100)
    expect(argLng).toBeGreaterThan(100)
  })

  it('differs from raw WGS84 (proves a real GCJ-02 offset was applied, not a passthrough)', () => {
    const { TMap, ctor } = makeFakeTMap()
    wgsToTMapLatLng(BEIJING, TMap)
    const [argLat, argLng] = ctor.mock.calls[0]
    // 北京有正向偏移：转换后的值必须 != 原始 WGS84（否则说明没走 coordTransform）。
    expect(argLat).not.toBe(BEIJING[1])
    expect(argLng).not.toBe(BEIJING[0])
    expect(Math.abs(argLat - BEIJING[1])).toBeGreaterThan(1e-4)
    expect(Math.abs(argLng - BEIJING[0])).toBeGreaterThan(1e-4)
  })

  it('wgsPathToLatLng converts every vertex through the same adapter, in order', () => {
    const { TMap, ctor } = makeFakeTMap()
    const path: Array<[number, number]> = [
      [116.404, 39.915],
      [120.21, 30.25],
    ]
    const result = wgsPathToLatLng(path, TMap)
    expect(result).toHaveLength(2)
    expect(ctor).toHaveBeenCalledTimes(2)
    for (let i = 0; i < path.length; i += 1) {
      const [gLng, gLat] = wgs84ToGcj02Tuple(path[i])
      const [argLat, argLng] = ctor.mock.calls[i]
      expect(argLat).toBeCloseTo(gLat, 9)
      expect(argLng).toBeCloseTo(gLng, 9)
    }
  })

  it('asLngLat narrows valid coords and rejects malformed ones', () => {
    expect(asLngLat([116.4, 39.9])).toEqual([116.4, 39.9])
    expect(asLngLat([116.4])).toBeNull()
    expect(asLngLat([])).toBeNull()
    expect(asLngLat([Number.NaN, 39.9])).toBeNull()
  })
})
