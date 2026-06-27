import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { db } from '@/mocks/db'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ProjectFormModal } from './ProjectFormModal'

describe('ProjectFormModal — dynamic custom_fields', () => {
  it('generates one form field per ACTIVE project_field dict item (and hides retired ones)', async () => {
    renderWithProviders(<ProjectFormModal open editing={null} onClose={() => {}} />)
    const dialog = await screen.findByRole('dialog')

    // 自定义字段子表单在 project_field 字典解析后渲染（一字段一项 ACTIVE 字典项）。
    await within(dialog).findByText('自定义字段', undefined, { timeout: 3000 })
    // ACTIVE project_field items: region (string) + budget (number)
    expect(within(dialog).getByLabelText('所属区域')).toBeInTheDocument()
    expect(within(dialog).getByLabelText('预算（万元）')).toBeInTheDocument()
    // retired item (legacy_owner) must NOT render
    expect(within(dialog).queryByLabelText('历史负责人')).not.toBeInTheDocument()
  })

  it('submits custom_fields collected from the dynamic sub-form', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ProjectFormModal open editing={null} onClose={() => {}} />)
    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByLabelText('所属区域')

    await user.type(within(dialog).getByLabelText('项目编码'), 'PRJ-FORM-001')
    await user.type(within(dialog).getByLabelText('项目名称'), '表单测试项目')
    await user.type(within(dialog).getByLabelText('所属区域'), '余杭区')
    await user.type(within(dialog).getByLabelText('预算（万元）'), '256')

    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    await waitFor(() => {
      const created = db.projects.find((p) => p.code === 'PRJ-FORM-001')
      expect(created).toBeDefined()
      expect(created?.custom_fields).toMatchObject({ region: '余杭区', budget: 256 })
    })
  })

  it('prefills base + custom fields when editing an existing project', async () => {
    const existing = db.projects.find((p) => p.id === 1)!
    renderWithProviders(<ProjectFormModal open editing={existing} onClose={() => {}} />)
    const dialog = await screen.findByRole('dialog')

    expect(await within(dialog).findByDisplayValue('PRJ-2026-001')).toBeInTheDocument()
    // custom_fields 在 project_field 字典解析后回填到动态字段。
    expect(
      await within(dialog).findByDisplayValue('滨江区', undefined, { timeout: 3000 }),
    ).toBeInTheDocument()
  })
})
