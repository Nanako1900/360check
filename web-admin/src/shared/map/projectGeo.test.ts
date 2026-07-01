import { describe, expect, it } from 'vitest'
import type { Project } from '@/shared/api/types'
import { boundsOf, centroid, closeRing, hasArea, outerRings, projectPoints } from './projectGeo'

/** 构造带 area_geom 的最小 Project（测试只关心几何，其余字段断言无关，故 cast）。 */
function mkProject(coordinates: number[][][][] | undefined): Project {
  return {
    id: 1,
    code: 'P-1',
    name: '测试项目',
    status: 'ACTIVE',
    area_geom: coordinates ? { type: 'MultiPolygon', coordinates } : undefined,
  } as unknown as Project
}

const SQUARE: number[][][][] = [
  [
    [
      [120, 30],
      [121, 30],
      [121, 31],
      [120, 31],
    ],
  ],
]

describe('projectGeo', () => {
  it('outerRings extracts the outer ring of each polygon and drops malformed points', () => {
    const rings = outerRings(mkProject(SQUARE))
    expect(rings).toHaveLength(1)
    expect(rings[0]).toEqual([
      [120, 30],
      [121, 30],
      [121, 31],
      [120, 31],
    ])
  })

  it('ignores rings with fewer than 2 valid points and drops NaN coords', () => {
    const bad: number[][][][] = [[[[Number.NaN, 1]]], [[[10, 10], [11, 11]]]]
    const rings = outerRings(mkProject(bad))
    expect(rings).toHaveLength(1)
    expect(rings[0]).toEqual([
      [10, 10],
      [11, 11],
    ])
  })

  it('hasArea reflects whether any drawable ring exists', () => {
    expect(hasArea(mkProject(SQUARE))).toBe(true)
    expect(hasArea(mkProject(undefined))).toBe(false)
    expect(hasArea(mkProject([]))).toBe(false)
  })

  it('projectPoints flattens all outer-ring points', () => {
    expect(projectPoints(mkProject(SQUARE))).toHaveLength(4)
  })

  it('centroid averages the points', () => {
    expect(centroid(projectPoints(mkProject(SQUARE)))).toEqual([120.5, 30.5])
    expect(centroid([])).toBeNull()
  })

  it('boundsOf computes the bounding box; null on empty', () => {
    expect(boundsOf(projectPoints(mkProject(SQUARE)))).toEqual({
      minLng: 120,
      minLat: 30,
      maxLng: 121,
      maxLat: 31,
    })
    expect(boundsOf([])).toBeNull()
  })

  it('closeRing appends the first point only when the ring is open', () => {
    const open: [number, number][] = [
      [0, 0],
      [1, 0],
      [1, 1],
    ]
    expect(closeRing(open)).toEqual([
      [0, 0],
      [1, 0],
      [1, 1],
      [0, 0],
    ])
    const closed: [number, number][] = [
      [0, 0],
      [1, 0],
      [0, 0],
    ]
    expect(closeRing(closed)).toEqual(closed)
  })
})
