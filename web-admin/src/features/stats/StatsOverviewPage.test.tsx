import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { renderWithProviders } from '@/test/renderWithProviders'
import { useAuthStore } from '@/shared/auth/authStore'
import { USERS } from '@/mocks/db'
import type { EChartOption } from '@/shared/ui/charts/options'
import { StatsOverviewPage } from './StatsOverviewPage'

const BASE = '*/api/v1'

// 不加载真实 echarts：mock EChart 渲染一个可断言的标记 + option 摘要。
vi.mock('@/shared/ui/charts/EChart', () => ({
  EChart: ({ option, ariaLabel }: { option: EChartOption; ariaLabel?: string }) => {
    const series = (option.series as Array<{ type?: string; data?: unknown[] }>) ?? []
    const count = series[0]?.data?.length ?? 0
    return (
      <div data-testid="echart" data-aria={ariaLabel} data-series-count={count}>
        chart:{ariaLabel}
      </div>
    )
  },
}))

describe('StatsOverviewPage', () => {
  beforeEach(() => {
    // PROJECT_STATS 导出按钮需 export:create 权限（前端仅 UI 门控）。
    useAuthStore.getState().setMe({ user: USERS[0], roles: [], permissions: ['export:create'] })
  })

  it('renders metric cards with formatted km and duration from /stats/overview', async () => {
    renderWithProviders(<StatsOverviewPage />)

    // total_mileage_meters 1234567.8 → 1234.57 km；avg_duration_seconds 8230 → 2h 17m 10s
    expect(await screen.findByText('1234.57 km')).toBeInTheDocument()
    expect(screen.getByText('2h 17m 10s')).toBeInTheDocument()
    // inspection_count 120 / problem_count 340
    expect(screen.getByText('120')).toBeInTheDocument()
    expect(screen.getByText('340')).toBeInTheDocument()
  })

  it('renders all four charts (type/status/inspector/project)', async () => {
    renderWithProviders(<StatsOverviewPage />)
    await waitFor(() => expect(screen.getAllByTestId('echart')).toHaveLength(4))
  })

  it('sends project_id in the query string when a project is selected and re-renders', async () => {
    const searches: string[] = []
    server.use(
      http.get(`${BASE}/stats/overview`, ({ request }) => {
        searches.push(new URL(request.url).search)
        return HttpResponse.json({
          success: true,
          data: {
            inspection_count: 7,
            problem_count: 9,
            total_mileage_meters: 0,
            total_duration_seconds: 0,
            avg_duration_seconds: 0,
            counts_by_type: [],
            counts_by_status: [],
            counts_by_inspector: [],
            counts_by_project: [],
          },
          error: null,
          meta: null,
        })
      }),
    )
    const user = userEvent.setup()
    renderWithProviders(<StatsOverviewPage />)
    await waitFor(() => expect(searches.length).toBeGreaterThanOrEqual(1))

    // 打开项目下拉（第一个 combobox）并选择第一个项目。
    const projectSelect = (await screen.findAllByRole('combobox'))[0]
    await user.click(projectSelect)
    const option = await screen.findByText('滨江绿道巡查')
    await user.click(option)

    await waitFor(() => {
      const hasProjectId = searches.some((s) => new URLSearchParams(s).get('project_id') === '1')
      expect(hasProjectId).toBe(true)
    })
  })

  it('shows the empty state when the overview has no data', async () => {
    server.use(
      http.get(`${BASE}/stats/overview`, () =>
        HttpResponse.json({
          success: true,
          data: {
            inspection_count: 0,
            problem_count: 0,
            total_mileage_meters: 0,
            total_duration_seconds: 0,
            avg_duration_seconds: 0,
            counts_by_type: [],
            counts_by_status: [],
            counts_by_inspector: [],
            counts_by_project: [],
          },
          error: null,
          meta: null,
        }),
      ),
    )
    renderWithProviders(<StatsOverviewPage />)
    expect(await screen.findByText('当前筛选条件下暂无统计数据。')).toBeInTheDocument()
  })

  it('renders the PROJECT_STATS export button (P8)', async () => {
    renderWithProviders(<StatsOverviewPage />)
    const exportBtn = await screen.findByTestId('export-button-PROJECT_STATS')
    expect(exportBtn).toBeEnabled()
  })
})
