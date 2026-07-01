import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { installFakeTMap, uninstallFakeTMap } from '@/test/fakeTMap'
import { useAuthStore } from '@/shared/auth/authStore'
import { USERS } from '@/mocks/db'
import { MapHome } from './MapHome'

beforeEach(() => {
  useAuthStore.getState().setMe({
    user: USERS[0],
    roles: [],
    permissions: ['project:read', 'problem:read'],
  })
})

afterEach(() => {
  uninstallFakeTMap()
})

describe('MapHome', () => {
  it('renders the full-screen map shell with floating chrome (topbar + rail + controls)', async () => {
    installFakeTMap()
    renderWithProviders(<MapHome />)

    // 悬浮顶栏标题 + 左栏图层区 + 右下缩放控件 —— 与地图加载状态无关，始终在场。
    expect(await screen.findByText('地图总览')).toBeInTheDocument()
    expect(screen.getByText('图层')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '放大' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '回到全部范围' })).toBeInTheDocument()
    // 快捷入口按权限过滤：project:read → 显示「项目管理」。
    expect(screen.getByText('项目管理')).toBeInTheDocument()
  })

  it('shows a full-screen hint when the map SDK/key is unavailable', async () => {
    // 不注入 fake，且测试环境 VITE_MAP_KEY 为空 → loadTencentSdk 拒绝 → status=error 降级。
    uninstallFakeTMap()
    renderWithProviders(<MapHome />)
    expect(
      await screen.findByText(
        '在 EdgeOne 配置 VITE_MAP_KEY 并将前端域名加入腾讯地图 key 白名单后即可显示地图。',
      ),
    ).toBeInTheDocument()
  })
})
