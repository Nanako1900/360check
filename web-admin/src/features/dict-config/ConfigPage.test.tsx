import { describe, expect, it } from 'vitest'
import { http as msw } from 'msw'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { server } from '@/mocks/server'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ConfigCard } from './ConfigCard'
import { exportImageSchema } from '@/shared/util/jsonSchema'

const BASE = '*/api/v1'

describe('ConfigCard', () => {
  it('renders version/hash and loads the seeded value', async () => {
    renderWithProviders(
      <ConfigCard
        configKey="export.image"
        title="导出图片"
        schema={exportImageSchema}
        onViewHistory={() => {}}
      />,
    )
    expect(await screen.findByText('v2')).toBeInTheDocument()
    expect(screen.getByText('cfg-exp-2')).toBeInTheDocument()
    await waitFor(() => {
      const textarea = screen.getByLabelText('配置值（JSON）') as HTMLTextAreaElement
      expect(textarea.value).toContain('"width"')
    })
  })

  it('rejects an invalid config value via zod (quality > 100)', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <ConfigCard
        configKey="export.image"
        title="导出图片"
        schema={exportImageSchema}
        onViewHistory={() => {}}
      />,
    )
    const textarea = (await screen.findByLabelText('配置值（JSON）')) as HTMLTextAreaElement
    await user.clear(textarea)
    await user.type(textarea, '{{"width":4096,"quality":999}')
    await user.click(screen.getByRole('button', { name: '保存配置' }))
    expect(await screen.findByText(/quality：/)).toBeInTheDocument()
  })

  it('rejects malformed JSON syntax', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <ConfigCard
        configKey="export.image"
        title="导出图片"
        schema={exportImageSchema}
        onViewHistory={() => {}}
      />,
    )
    const textarea = (await screen.findByLabelText('配置值（JSON）')) as HTMLTextAreaElement
    await user.clear(textarea)
    await user.type(textarea, 'not json')
    await user.click(screen.getByRole('button', { name: '保存配置' }))
    expect(await screen.findByText('JSON 语法错误，请检查格式')).toBeInTheDocument()
  })

  it('saves a valid config value via PUT', async () => {
    let capturedBody: unknown = null
    server.use(
      msw.put(`${BASE}/config/:key`, async ({ request }) => {
        capturedBody = await request.json()
        return Response.json({
          success: true,
          data: {
            id: 1,
            config_key: 'export.image',
            value: { width: 1024, quality: 50 },
            version: 3,
            content_hash: 'cfg-exp-3',
            is_active: true,
          },
          error: null,
          meta: null,
        })
      }),
    )
    const user = userEvent.setup()
    renderWithProviders(
      <ConfigCard
        configKey="export.image"
        title="导出图片"
        schema={exportImageSchema}
        onViewHistory={() => {}}
      />,
    )
    const textarea = (await screen.findByLabelText('配置值（JSON）')) as HTMLTextAreaElement
    await user.clear(textarea)
    await user.type(textarea, '{{"width":1024,"quality":50}')
    await user.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() => expect(capturedBody).toEqual({ value: { width: 1024, quality: 50 } }))
  })

  it('opens history via the view-history button', async () => {
    const user = userEvent.setup()
    let opened = ''
    renderWithProviders(
      <ConfigCard
        configKey="export.image"
        title="导出图片"
        schema={exportImageSchema}
        onViewHistory={(k) => {
          opened = k
        }}
      />,
    )
    await screen.findByText('v2')
    const card = screen.getByText('导出图片').closest('.ant-card') as HTMLElement
    await user.click(within(card).getByRole('button', { name: /查看历史/ }))
    expect(opened).toBe('export.image')
  })
})
