import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { renderWithProviders } from '@/test/renderWithProviders'
import { InspectionDetailPage } from './InspectionDetailPage'

function renderDetail(id: number) {
  return renderWithProviders(
    <Routes>
      <Route path="/inspections/:id" element={<InspectionDetailPage />} />
      <Route path="/inspections/:id/map" element={<div>轨迹地图占位</div>} />
      <Route path="/problems" element={<div>问题列表占位</div>} />
    </Routes>,
    { route: `/inspections/${id}` },
  )
}

describe('InspectionDetailPage', () => {
  it('renders a finished inspection with duration and mileage', async () => {
    renderDetail(1)
    expect(await screen.findByText('巡查详情')).toBeInTheDocument()
    expect(screen.getByText('1h 5m 25s')).toBeInTheDocument()
    expect(screen.getByText('12.57 km')).toBeInTheDocument()
  })

  it('hides mileage/duration and shows in-progress for an IN_PROGRESS inspection', async () => {
    renderDetail(2)
    await screen.findByText('巡查详情')
    // mileage column shows 进行中 placeholder; no km rendered
    expect(screen.queryByText(/km$/)).not.toBeInTheDocument()
    expect(screen.getAllByText('进行中').length).toBeGreaterThan(0)
    // map button disabled while in progress
    const mapBtn = screen.getByRole('button', { name: /查看轨迹地图/ })
    expect(mapBtn).toBeDisabled()
  })

  it('navigates to the problems list filtered by inspection_id', async () => {
    const user = userEvent.setup()
    renderDetail(1)
    await screen.findByText('巡查详情')
    await user.click(screen.getByRole('button', { name: /该巡查的问题/ }))
    expect(await screen.findByText('问题列表占位')).toBeInTheDocument()
  })

  it('navigates to the trajectory map placeholder for a finished inspection', async () => {
    const user = userEvent.setup()
    renderDetail(1)
    await screen.findByText('巡查详情')
    await user.click(screen.getByRole('button', { name: /查看轨迹地图/ }))
    expect(await screen.findByText('轨迹地图占位')).toBeInTheDocument()
  })

  it('shows not-found state for a missing inspection', async () => {
    renderDetail(9999)
    expect(await screen.findByText('巡查记录不存在或已被删除。')).toBeInTheDocument()
  })
})
