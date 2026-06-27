import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { installFakeTMap, uninstallFakeTMap } from '@/test/fakeTMap'
import type { FakeTMapHandles } from '@/test/fakeTMap'
import type { GeoJSONPoint } from '@/shared/api/types'
import { ProblemPointMap } from './ProblemPointMap'

const POINT: GeoJSONPoint = { type: 'Point', coordinates: [120.2115, 30.2509] }

describe('ProblemPointMap', () => {
  let handles: FakeTMapHandles

  beforeEach(() => {
    handles = installFakeTMap()
  })
  afterEach(() => {
    uninstallFakeTMap()
  })

  it('places a single marker for the WGS84 point via the shared toLatLng conversion', async () => {
    renderWithProviders(<ProblemPointMap geom={POINT} />)
    await waitFor(() => expect(handles.MultiMarker).toHaveBeenCalledTimes(1))
    // 坐标经 wgsToTMapLatLng（WGS84→GCJ-02→LatLng(lat,lng)）转换，绝不内联换算。
    expect(handles.LatLng).toHaveBeenCalled()
  })

  it('renders the no-point hint when geom is missing', () => {
    const { getByText } = renderWithProviders(<ProblemPointMap geom={undefined} />)
    expect(getByText('该问题暂无点位坐标。')).toBeInTheDocument()
  })
})
