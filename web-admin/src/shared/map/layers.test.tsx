import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import { wgs84ToGcj02Tuple } from '@/shared/geo/coordTransform'
import type { GeoJSONLineString, ProblemFeatureCollection } from '@/shared/api/types'
import { installFakeTMap, uninstallFakeTMap } from '@/test/fakeTMap'
import type { FakeTMapHandles } from '@/test/fakeTMap'
import { TrajectoryLayer } from './TrajectoryLayer'
import { ProblemMarkerLayer } from './ProblemMarkerLayer'
import type { TMapMap } from './tmap'

afterEach(() => {
  uninstallFakeTMap()
})

function fakeMap(): TMapMap {
  return {
    setCenter: vi.fn(),
    setZoom: vi.fn(),
    fitBounds: vi.fn(),
    destroy: vi.fn(),
    on: vi.fn(),
    off: vi.fn(),
  }
}

const ROUTE: GeoJSONLineString = {
  type: 'LineString',
  coordinates: [
    [116.404, 39.915],
    [116.41, 39.92],
  ],
}

const PROBLEMS: ProblemFeatureCollection = {
  type: 'FeatureCollection',
  features: [
    {
      type: 'Feature',
      geometry: { type: 'Point', coordinates: [116.404, 39.915] },
      properties: { id: 1, type_label: '裂缝', status_label: '待处理', status_color: '#8c8c8c' },
    },
    {
      type: 'Feature',
      geometry: { type: 'Point', coordinates: [116.41, 39.92] },
      properties: { id: 2, type_label: '裂缝', status_label: '处理中', status_color: '#1677ff' },
    },
  ],
}

describe('TrajectoryLayer', () => {
  it('constructs MultiPolyline with GCJ-02-converted, lat-first vertices', () => {
    const handles: FakeTMapHandles = installFakeTMap()
    const map = fakeMap()
    render(<TrajectoryLayer map={map} TMap={handles.TMap} route={ROUTE} showEndpoints={false} />)

    expect(handles.MultiPolyline).toHaveBeenCalledTimes(1)
    const options = handles.MultiPolyline.mock.calls[0][0] as {
      geometries: Array<{ paths: Array<{ lat: number; lng: number }> }>
    }
    const paths = options.geometries[0].paths
    expect(paths).toHaveLength(2)
    // 每个顶点 = wgs84ToGcj02 后的值，且 lat/lng 来自 (lat, lng) 构造顺序。
    ROUTE.coordinates.forEach((coord, i) => {
      const [gLng, gLat] = wgs84ToGcj02Tuple(coord as [number, number])
      expect(paths[i].lat).toBeCloseTo(gLat, 9)
      expect(paths[i].lng).toBeCloseTo(gLng, 9)
    })
  })

  it('skips drawing when route has fewer than 2 vertices', () => {
    const handles = installFakeTMap()
    const map = fakeMap()
    const shortRoute: GeoJSONLineString = { type: 'LineString', coordinates: [[116.4, 39.9]] }
    render(<TrajectoryLayer map={map} TMap={handles.TMap} route={shortRoute} />)
    expect(handles.MultiPolyline).not.toHaveBeenCalled()
  })

  it('cleans up the polyline on unmount (setMap(null))', () => {
    const handles = installFakeTMap()
    const map = fakeMap()
    const { unmount } = render(
      <TrajectoryLayer map={map} TMap={handles.TMap} route={ROUTE} showEndpoints={false} />,
    )
    const instance = handles.MultiPolyline.mock.results[0].value as { setMap: ReturnType<typeof vi.fn> }
    unmount()
    expect(instance.setMap).toHaveBeenCalledWith(null)
  })
})

describe('ProblemMarkerLayer', () => {
  it('constructs MultiMarker + MarkerCluster with GCJ-02-converted positions', () => {
    const handles = installFakeTMap()
    const map = fakeMap()
    render(
      <ProblemMarkerLayer map={map} TMap={handles.TMap} data={PROBLEMS} onSelect={vi.fn()} />,
    )

    expect(handles.MultiMarker).toHaveBeenCalledTimes(1)
    expect(handles.MarkerCluster).toHaveBeenCalledTimes(1)
    const options = handles.MultiMarker.mock.calls[0][0] as {
      geometries: Array<{ position: { lat: number; lng: number }; id: string }>
    }
    expect(options.geometries).toHaveLength(2)
    PROBLEMS.features.forEach((feature, i) => {
      const [gLng, gLat] = wgs84ToGcj02Tuple(feature.geometry.coordinates as [number, number])
      expect(options.geometries[i].position.lat).toBeCloseTo(gLat, 9)
      expect(options.geometries[i].position.lng).toBeCloseTo(gLng, 9)
    })
  })

  it('invokes onSelect with the clicked feature (position + properties)', () => {
    const handles = installFakeTMap()
    const map = fakeMap()
    const onSelect = vi.fn()
    render(<ProblemMarkerLayer map={map} TMap={handles.TMap} data={PROBLEMS} onSelect={onSelect} />)

    handles.markerInstance?.fireClick('2')
    expect(onSelect).toHaveBeenCalledTimes(1)
    const arg = onSelect.mock.calls[0][0] as {
      position: [number, number]
      properties: { id: number }
    }
    expect(arg.properties.id).toBe(2)
    expect(arg.position).toEqual([116.41, 39.92])
  })

  it('does not draw layers for an empty FeatureCollection', () => {
    const handles = installFakeTMap()
    const map = fakeMap()
    const empty: ProblemFeatureCollection = { type: 'FeatureCollection', features: [] }
    render(<ProblemMarkerLayer map={map} TMap={handles.TMap} data={empty} onSelect={vi.fn()} />)
    expect(handles.MultiMarker).not.toHaveBeenCalled()
    expect(handles.MarkerCluster).not.toHaveBeenCalled()
  })

  it('cleans up marker + cluster on unmount', () => {
    const handles = installFakeTMap()
    const map = fakeMap()
    const { unmount } = render(
      <ProblemMarkerLayer map={map} TMap={handles.TMap} data={PROBLEMS} onSelect={vi.fn()} />,
    )
    const marker = handles.MultiMarker.mock.results[0].value as { setMap: ReturnType<typeof vi.fn> }
    const cluster = handles.MarkerCluster.mock.results[0].value as {
      setMap: ReturnType<typeof vi.fn>
    }
    unmount()
    expect(marker.setMap).toHaveBeenCalledWith(null)
    expect(cluster.setMap).toHaveBeenCalledWith(null)
  })
})
