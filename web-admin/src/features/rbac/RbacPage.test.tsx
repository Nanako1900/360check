import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { db } from '@/mocks/db'
import { renderWithProviders } from '@/test/renderWithProviders'
import { RbacPage } from './RbacPage'

async function switchToRoles(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(screen.getByRole('tab', { name: '角色' }))
  // 角色页特有锚点：自定义角色 code「auditor」（用户名中不存在，消除 admin 歧义）
  await screen.findByText('auditor')
}

describe('RbacPage — users tab', () => {
  it('renders seeded users in the list', async () => {
    renderWithProviders(<RbacPage />)
    expect(await screen.findByText('admin')).toBeInTheDocument()
    expect(screen.getByText('inspector_li')).toBeInTheDocument()
    // last_login_at 渲染（李巡查有登录时间），王巡查显示「从未登录」
    expect(screen.getByText('从未登录')).toBeInTheDocument()
  })

  it('creates a new user through the modal', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('admin')

    await user.click(screen.getByRole('button', { name: /新建用户/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('用户名'), 'qa_user')
    await user.type(within(dialog).getByLabelText('初始密码'), 'secret123')
    await user.type(within(dialog).getByLabelText('姓名'), '测试用户')
    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    await waitFor(() => expect(db.users.some((u) => u.username === 'qa_user')).toBe(true))
  })

  it('soft-deletes a user via popconfirm (row removed)', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('inspector_li')

    const rows = screen.getAllByRole('row')
    const targetRow = rows.find((r) => within(r).queryByText('inspector_li'))
    expect(targetRow).toBeDefined()
    await user.click(within(targetRow as HTMLElement).getByRole('button', { name: '删除' }))
    const confirm = await screen.findByRole('button', { name: /确\s*定/ })
    await user.click(confirm)

    await waitFor(() => expect(db.users.some((u) => u.username === 'inspector_li')).toBe(false))
  })

  it('resets another user password through the reset modal', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('inspector_li')

    const rows = screen.getAllByRole('row')
    const targetRow = rows.find((r) => within(r).queryByText('inspector_li'))
    await user.click(within(targetRow as HTMLElement).getByRole('button', { name: '重置密码' }))

    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('新密码'), 'brandnew12')
    await user.type(within(dialog).getByLabelText('确认新密码'), 'brandnew12')
    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    await waitFor(() => expect(db.passwords['inspector_li']).toBe('brandnew12'))
  })

  it('assigns roles through the roles drawer', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('inspector_li')

    const rows = screen.getAllByRole('row')
    const targetRow = rows.find((r) => within(r).queryByText('inspector_li'))
    await user.click(within(targetRow as HTMLElement).getByRole('button', { name: '分配角色' }))

    // 抽屉打开，勾选「审计员」角色（id=3）后保存
    await screen.findByText('分配角色 · 李巡查')
    const auditorCheckbox = await screen.findByRole('checkbox', { name: /审计员/ })
    await user.click(auditorCheckbox)
    await user.click(screen.getByRole('button', { name: /保\s*存/ }))

    // userRoles[2] 现包含 role 3（审计员）
    await waitFor(() => expect(db.userRoles[2]).toContain(3))
  })

  it('rejects a weak password on create with field error', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('admin')

    await user.click(screen.getByRole('button', { name: /新建用户/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('用户名'), 'weak_user')
    await user.type(within(dialog).getByLabelText('初始密码'), 'short')
    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    expect(await within(dialog).findByText('密码至少 8 位，需包含字母与数字')).toBeInTheDocument()
    expect(db.users.some((u) => u.username === 'weak_user')).toBe(false)
  })

  it('backfills the username field error on duplicate (VALIDATION_FAILED)', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('admin')

    await user.click(screen.getByRole('button', { name: /新建用户/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('用户名'), 'inspector_li')
    await user.type(within(dialog).getByLabelText('初始密码'), 'secret123')
    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    // 后端 422 field error 回填到 username 字段
    expect(await within(dialog).findByText('用户名已存在')).toBeInTheDocument()
  })

  it('edits an existing user without a password field', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('inspector_li')

    const rows = screen.getAllByRole('row')
    const targetRow = rows.find((r) => within(r).queryByText('inspector_li'))
    await user.click(within(targetRow as HTMLElement).getByRole('button', { name: '编辑' }))

    const dialog = await screen.findByRole('dialog')
    // 编辑模式无初始密码字段
    expect(within(dialog).queryByLabelText('初始密码')).toBeNull()
    const displayName = within(dialog).getByLabelText('姓名')
    await user.clear(displayName)
    await user.type(displayName, '李工程师')
    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    await waitFor(() =>
      expect(db.users.find((u) => u.username === 'inspector_li')?.display_name).toBe('李工程师'),
    )
  })
})

describe('RbacPage — roles tab', () => {
  it('renders seeded roles and disables delete on system roles', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('admin')
    await switchToRoles(user)

    const rows = screen.getAllByRole('row')
    // 角色名「管理员」唯一标识 admin 角色行（用户 display_name 为「系统管理员」，不冲突）
    const adminRow = rows.find((r) => within(r).queryByText('管理员'))
    expect(adminRow).toBeDefined()
    // 系统角色：删除按钮禁用
    const deleteBtn = within(adminRow as HTMLElement).getByRole('button', { name: '删除' })
    expect(deleteBtn).toBeDisabled()

    // 自定义角色 auditor：删除按钮可用
    const auditorRow = rows.find((r) => within(r).queryByText('auditor'))
    const auditorDelete = within(auditorRow as HTMLElement).getByRole('button', { name: '删除' })
    expect(auditorDelete).toBeEnabled()
  })

  it('creates a new role through the modal', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('admin')
    await switchToRoles(user)

    await user.click(screen.getByRole('button', { name: /新建角色/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('角色编码'), 'reviewer')
    await user.type(within(dialog).getByLabelText('角色名称'), '复核员')
    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    await waitFor(() => expect(db.roles.some((r) => r.code === 'reviewer')).toBe(true))
  })

  it('configures role permissions via the checkable tree and saves', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RbacPage />)
    await screen.findByText('admin')
    await switchToRoles(user)

    const rows = screen.getAllByRole('row')
    const auditorRow = rows.find((r) => within(r).queryByText('auditor'))
    await user.click(within(auditorRow as HTMLElement).getByRole('button', { name: '配置权限' }))

    // 抽屉 + 权限树渲染（分组标题可见）
    await screen.findByText('配置权限 · 审计员')
    const leaf = await screen.findByText(/新建用户（user:create）/)
    await user.click(leaf)
    await user.click(screen.getByRole('button', { name: /保\s*存/ }))

    // auditor (id=3) 的权限集合发生变化（含 user:create 对应 id）
    await waitFor(() => {
      const createId = db.permissionCatalog.find((p) => p.code === 'user:create')?.id
      expect(createId).toBeDefined()
      expect(db.rolePermissions[3]).toContain(createId)
    })
  })
})
