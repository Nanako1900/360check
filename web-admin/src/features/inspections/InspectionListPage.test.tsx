import { describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { renderWithProviders } from '@/test/renderWithProviders'
import { InspectionListPage } from './InspectionListPage'

const BASE = '*/api/v1'

describe('InspectionListPage', () => {
  it('renders seeded inspections with formatted duration and mileage in km', async () => {
    renderWithProviders(<InspectionListPage />)
    // FINISHED 巡查：duration 3925s = 1h 5m 25s；mileage 12567.8m = 12.57 km
    expect(await screen.findByText('1h 5m 25s')).toBeInTheDocument()
    expect(screen.getByText('12.57 km')).toBeInTheDocument()
  })

  it('renders in-progress placeholder instead of duration/mileage for IN_PROGRESS rows', async () => {
    renderWithProviders(<InspectionListPage />)
    await screen.findByText('1h 5m 25s')
    // 进行中行：时长列显示「进行中」，不显示 0s
    expect(screen.getAllByText('进行中').length).toBeGreaterThan(0)
  })

  it('seeds the project_id filter from the URL and sends it in the query string', async () => {
    let captured = ''
    server.use(
      http.get(`${BASE}/inspections`, ({ request }) => {
        captured = new URL(request.url).search
        return HttpResponse.json({
          success: true,
          data: [],
          error: null,
          meta: { total: 0, page: 1, page_size: 20 },
        })
      }),
    )
    renderWithProviders(<InspectionListPage />, { route: '/inspections?project_id=2' })
    await waitFor(() => {
      expect(new URLSearchParams(captured).get('project_id')).toBe('2')
    })
  })
})
