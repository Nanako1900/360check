import { describe, expect, it } from 'vitest'
import {
  gcj02ToWgs84,
  gcj02ToWgs84Tuple,
  outOfChina,
  wgs84ToGcj02,
  wgs84ToGcj02Tuple,
} from './coordTransform'

// 天安门附近 WGS84
const BEIJING: [number, number] = [116.404, 39.915]
// 香港尖沙咀附近 WGS84
const HONGKONG: [number, number] = [114.1772, 22.302]
// 东京（境外）
const TOKYO: [number, number] = [139.7671, 35.6812]

describe('coordTransform WGS84 <-> GCJ-02', () => {
  it('round-trips within < 1e-6 degrees (§4.3)', () => {
    for (const [lng, lat] of [BEIJING, HONGKONG, [121.4737, 31.2304] as [number, number]]) {
      const [gLng, gLat] = wgs84ToGcj02(lng, lat)
      const [wLng, wLat] = gcj02ToWgs84(gLng, gLat)
      expect(Math.abs(wLng - lng)).toBeLessThan(1e-6)
      expect(Math.abs(wLat - lat)).toBeLessThan(1e-6)
    }
  })

  it('applies a positive offset in Beijing (direction + plausible magnitude)', () => {
    const [gLng, gLat] = wgs84ToGcj02(BEIJING[0], BEIJING[1])
    const dLng = gLng - BEIJING[0]
    const dLat = gLat - BEIJING[1]
    // 北京方向：经纬度均正向偏移
    expect(dLng).toBeGreaterThan(0)
    expect(dLat).toBeGreaterThan(0)
    // 偏移量量级 ~ 数百米（度级 ~0.001..0.01）
    expect(dLng).toBeGreaterThan(0.001)
    expect(dLng).toBeLessThan(0.01)
    expect(dLat).toBeGreaterThan(0.001)
    expect(dLat).toBeLessThan(0.01)
  })

  it('matches a known reference vector within 1e-4 (catches gross algorithm errors)', () => {
    const [gLng, gLat] = wgs84ToGcj02(BEIJING[0], BEIJING[1])
    expect(gLng).toBeCloseTo(116.41024, 4)
    expect(gLat).toBeCloseTo(39.91641, 4)
  })

  it('does not offset out-of-China points', () => {
    expect(outOfChina(TOKYO[0], TOKYO[1])).toBe(true)
    expect(wgs84ToGcj02(TOKYO[0], TOKYO[1])).toEqual(TOKYO)
    expect(gcj02ToWgs84(TOKYO[0], TOKYO[1])).toEqual(TOKYO)
    expect(outOfChina(BEIJING[0], BEIJING[1])).toBe(false)
  })

  it('preserves [lng, lat] order (lng first, lat second)', () => {
    const [gLng, gLat] = wgs84ToGcj02Tuple(BEIJING)
    // 经度 ~116（>100），纬度 ~39（<100）——确认没有把顺序写反
    expect(gLng).toBeGreaterThan(100)
    expect(gLat).toBeLessThan(100)
    const back = gcj02ToWgs84Tuple([gLng, gLat])
    expect(back[0]).toBeCloseTo(BEIJING[0], 6)
    expect(back[1]).toBeCloseTo(BEIJING[1], 6)
  })
})
