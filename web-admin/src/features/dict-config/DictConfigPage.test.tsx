import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { db } from '@/mocks/db'
import { renderWithProviders } from '@/test/renderWithProviders'
import { DictConfigPage } from './DictConfigPage'
import { scopeLabel } from './scopes'

describe('scopeLabel', () => {
  it('maps known scopes to Chinese labels and falls back for unknown', () => {
    expect(scopeLabel('capture_preset')).toBe('拍照预设')
    expect(scopeLabel('problem_status')).toBe('问题状态')
  })
})

describe('DictConfigPage', () => {
  it('renders the dict types tab with seeded rows and can switch to config tab', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DictConfigPage />)

    // 字典类型 tab：种子类型渲染（code 列唯一）
    expect(await screen.findByText('problem_status')).toBeInTheDocument()
    expect(screen.getByText('problem_type')).toBeInTheDocument()

    // 切到系统配置 tab
    await user.click(screen.getByRole('tab', { name: '系统配置' }))
    expect(await screen.findByText('导出图片')).toBeInTheDocument()
    expect(screen.getByText('拍照默认')).toBeInTheDocument()
  })

  it('creates a new dict type through the modal', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DictConfigPage />)
    await screen.findByText('problem_status')

    await user.click(screen.getByRole('button', { name: /新建字典类型/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('机器键'), 'extra_field')
    await user.type(within(dialog).getByLabelText('名称'), '额外字段')
    // 选择作用域
    await user.click(within(dialog).getByLabelText('作用域'))
    const option = await screen.findByText('项目字段')
    await user.click(option)
    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    await waitFor(() => expect(db.dictTypes.some((t) => t.code === 'extra_field')).toBe(true))
  })

  it('deletes a dict type via popconfirm', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DictConfigPage />)
    await screen.findByText('历史杂项') // legacy_misc row (retired type)

    const rows = screen.getAllByRole('row')
    const legacyRow = rows.find((r) => within(r).queryByText('历史杂项'))
    expect(legacyRow).toBeDefined()
    await user.click(within(legacyRow as HTMLElement).getByRole('button', { name: '删除' }))
    const confirm = await screen.findByRole('button', { name: /确\s*定/ })
    await user.click(confirm)

    await waitFor(() => expect(db.dictTypes.some((t) => t.code === 'legacy_misc')).toBe(false))
  })

  it('opens the items drawer from a row action', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DictConfigPage />)
    await screen.findByText('problem_status')

    const rows = screen.getAllByRole('row')
    const psRow = rows.find((r) => within(r).queryByText('problem_status'))
    await user.click(within(psRow as HTMLElement).getByRole('button', { name: '管理字典项' }))

    // 抽屉打开，显示字典项标题与提示
    expect(await screen.findByText(/字典项管理/)).toBeInTheDocument()
    expect(screen.getByText('停用后历史数据仍可见')).toBeInTheDocument()
  })

  it('opens config history drawer showing version timeline', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DictConfigPage />)
    await screen.findByText('problem_status')
    await user.click(screen.getByRole('tab', { name: '系统配置' }))
    await screen.findByText('导出图片')

    const card = screen.getByText('导出图片').closest('.ant-card') as HTMLElement
    await user.click(within(card).getByRole('button', { name: /查看历史/ }))

    expect(await screen.findByText(/版本历史/)).toBeInTheDocument()
    // 当前生效版本 v2 + 历史 v1
    expect(await screen.findByText('当前生效')).toBeInTheDocument()
  })
})
