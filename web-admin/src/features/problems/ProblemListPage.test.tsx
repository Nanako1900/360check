import { describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ProblemListPage } from './ProblemListPage'

const BASE = '*/api/v1'

describe('ProblemListPage', () => {
  it('renders seeded problems with their type/status DictTags', async () => {
    renderWithProviders(<ProblemListPage />)
    // 列表渲染问题标题与字典标签（类型「裂缝」、状态「待处理」/「处理中」）。
    expect(await screen.findByText('步道路面横向裂缝，长约 1.2m')).toBeInTheDocument()
    expect(await screen.findByText('待处理')).toBeInTheDocument()
    expect(await screen.findByText('处理中')).toBeInTheDocument()
  })

  it('renders a retired dict item (is_active=false) in the table via DictTag (history tolerance)', async () => {
    // 问题 5003 引用退役状态 199（label「历史状态」）。用 inspection 过滤为空 → 用项目 2 过滤命中。
    renderWithProviders(<ProblemListPage />, { route: '/problems' })
    // 退役项仍以人类可读 label + 「（已停用）」后缀渲染，绝不显示「未知」。
    expect(await screen.findByText('历史状态')).toBeInTheDocument()
    expect(screen.getByText('（已停用）')).toBeInTheDocument()
  })

  it('seeds the inspection_id filter from the URL and sends it in the query string (D1)', async () => {
    let captured = ''
    server.use(
      http.get(`${BASE}/problems`, ({ request }) => {
        captured = new URL(request.url).search
        return HttpResponse.json({
          success: true,
          data: [],
          error: null,
          meta: { total: 0, page: 1, page_size: 10 },
        })
      }),
    )
    renderWithProviders(<ProblemListPage />, { route: '/problems?inspection_id=1' })
    await waitFor(() => {
      expect(new URLSearchParams(captured).get('inspection_id')).toBe('1')
    })
  })

})
