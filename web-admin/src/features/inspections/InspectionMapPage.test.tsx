import { afterEach, describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { Route, Routes } from 'react-router-dom'
import { server } from '@/mocks/server'
import { renderWithProviders } from '@/test/renderWithProviders'
import { installFakeTMap, uninstallFakeTMap } from '@/test/fakeTMap'
import { InspectionMapPage } from './InspectionMapPage'

const BASE = '*/api/v1'

afterEach(() => {
  uninstallFakeTMap()
})

function renderMap(id: number) {
  return renderWithProviders(
    <Routes>
      <Route path="/inspections/:id/map" element={<InspectionMapPage />} />
      <Route path="/inspections/:id" element={<div>巡查详情占位</div>} />
      <Route path="/problems" element={<div>问题列表占位</div>} />
    </Routes>,
    { route: `/inspections/${id}/map` },
  )
}

describe('InspectionMapPage', () => {
  it('renders the map title and problem count once the SDK is ready', async () => {
    const handles = installFakeTMap()
    renderMap(1)
    expect(await screen.findByText('巡查轨迹地图')).toBeInTheDocument()
    await waitFor(() => expect(handles.Map).toHaveBeenCalled())
    // 该巡查 seed 有 2 个问题点。
    expect(await screen.findByText('共 2 个问题点')).toBeInTheDocument()
  })

  it('degrades gracefully with a fallback when the SDK fails to load', async () => {
    // 不注入 window.TMap，且让脚本注入立即 error。
    uninstallFakeTMap()
    server.use(
      http.get(`${BASE}/inspections/:id/trajectory`, () =>
        HttpResponse.json({
          success: true,
          data: { inspection_id: 1, points: [] },
          error: null,
          meta: null,
        }),
      ),
    )
    renderMap(1)
    expect(await screen.findByText('巡查轨迹地图')).toBeInTheDocument()
    // 触发脚本 error → 进入降级提示。
    const script = document.getElementById('tencent-map-gl-sdk')
    script?.dispatchEvent(new Event('error'))
    expect(await screen.findByText('地图加载失败')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /打开问题列表/ })).toBeInTheDocument()
  })
})
