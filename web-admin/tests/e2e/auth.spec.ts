import { test, expect } from '@playwright/test'

// 关键流程（§6.3 #1）：登录 → 改密 → 登出。dev server 启用 MSW 按 openapi.yaml mock。
test.describe('认证外壳', () => {
  test('登录后进入工作台并可登出', async ({ page }) => {
    await page.goto('/login')
    await expect(page.getByPlaceholder('请输入用户名')).toBeVisible()

    await page.getByPlaceholder('请输入用户名').fill('admin')
    await page.getByPlaceholder('请输入密码').fill('admin12345')
    await page.getByRole('button', { name: /登/ }).click()

    // 进入受保护区（动态菜单出现“工作台”）
    await expect(page.getByText('工作台').first()).toBeVisible()

    // 打开用户菜单 → 退出登录
    await page.getByText('系统管理员', { exact: true }).click()
    await page.getByText('退出登录').click()

    await expect(page).toHaveURL(/\/login$/)
  })

  test('修改密码后回到登录页', async ({ page }) => {
    await page.goto('/login')
    await page.getByPlaceholder('请输入用户名').fill('admin')
    await page.getByPlaceholder('请输入密码').fill('admin12345')
    await page.getByRole('button', { name: /登/ }).click()
    await expect(page.getByText('工作台').first()).toBeVisible()

    await page.getByText('系统管理员', { exact: true }).click()
    await page.getByText('修改密码').click()

    await page.getByLabel('当前密码').fill('admin12345')
    await page.getByLabel('新密码', { exact: true }).fill('admin67890')
    await page.getByLabel('确认新密码').fill('admin67890')
    await page.getByRole('button', { name: /确\s*定/ }).click()

    // 改密成功后强制登出回到登录页
    await expect(page).toHaveURL(/\/login$/)
  })
})
