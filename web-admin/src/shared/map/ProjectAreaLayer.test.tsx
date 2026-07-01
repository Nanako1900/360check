import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithProviders } from '@/test/renderWithProviders'
import { installFakeTMap, uninstallFakeTMap } from '@/test/fakeTMap'
import type { FakeTMapHandles } from '@/test/fakeTMap'
import type { Project } from '@/shared/api/types'
import type { TMapMap } from './tmap'
import { ProjectAreaLayer } from './ProjectAreaLayer'

function mkProject(id: number, coordinates: number[][][][] | undefined): Project {
  return {
    id,
    code: `P-${id}`,
    name: `项目${id}`,
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

let handles: FakeTMapHandles
let map: TMapMap

beforeEach(() => {
  handles = installFakeTMap()
  map = new handles.TMap.Map(document.createElement('div'), {
    center: new handles.TMap.LatLng(0, 0),
    zoom: 12,
  })
})

afterEach(() => {
  uninstallFakeTMap()
})

describe('ProjectAreaLayer', () => {
  it('draws outline + centroid marker for projects with area and fits bounds', () => {
    renderWithProviders(
      <ProjectAreaLayer map={map} TMap={handles.TMap} projects={[mkProject(5, SQUARE)]} onSelect={vi.fn()} />,
    )

    // 一个项目一条外环轮廓 + 一个质心标记。
    expect(handles.MultiPolyline).toHaveBeenCalledTimes(1)
    expect(handles.MultiPolyline.mock.calls[0][0].geometries).toHaveLength(1)
    expect(handles.MultiMarker).toHaveBeenCalledTimes(1)
    expect(handles.MultiMarker.mock.calls[0][0].geometries).toHaveLength(1)
    // 自动 fitBounds 到全部项目。
    expect(handles.mapInstance?.fitBounds).toHaveBeenCalled()
  })

  it('fires onSelect(projectId) when a project marker is clicked', () => {
    const onSelect = vi.fn()
    renderWithProviders(
      <ProjectAreaLayer map={map} TMap={handles.TMap} projects={[mkProject(5, SQUARE)]} onSelect={onSelect} />,
    )
    handles.markerInstance?.fireClick('5')
    expect(onSelect).toHaveBeenCalledWith(5)
  })

  it('renders nothing and does not fit when no project has an area', () => {
    renderWithProviders(
      <ProjectAreaLayer map={map} TMap={handles.TMap} projects={[mkProject(6, undefined)]} onSelect={vi.fn()} />,
    )
    expect(handles.MultiPolyline).not.toHaveBeenCalled()
    expect(handles.MultiMarker).not.toHaveBeenCalled()
    expect(handles.mapInstance?.fitBounds).not.toHaveBeenCalled()
  })
})
