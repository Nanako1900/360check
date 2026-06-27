import { describe, expect, it } from 'vitest'
import { filterMenu, isMenuItemVisible, MENU } from './menu'
import type { AppMenuItem } from './menu'

describe('menu filtering', () => {
  it('keeps permission-free items and hides unauthorized ones', () => {
    const can = (c: string) => c === 'project:read'
    const paths = filterMenu(MENU, can).map((i) => i.path)
    expect(paths).toContain('/') // 无需权限
    expect(paths).toContain('/projects') // project:read 已授予
    expect(paths).not.toContain('/stats')
  })

  it('shows all items when every permission is granted', () => {
    expect(filterMenu(MENU, () => true).length).toBe(MENU.length)
  })

  it('shows only permission-free items when nothing is granted', () => {
    const none = filterMenu(MENU, () => false)
    expect(none.every((i) => !i.permission)).toBe(true)
    expect(none.map((i) => i.path)).toEqual(['/'])
  })

  it('isMenuItemVisible treats array permission as any-of', () => {
    const item: AppMenuItem = {
      path: '/dict',
      name: 'menu.dict',
      permission: ['dict:read', 'config:read'],
    }
    expect(isMenuItemVisible(item, (c) => c === 'config:read')).toBe(true)
    expect(isMenuItemVisible(item, () => false)).toBe(false)
  })
})
