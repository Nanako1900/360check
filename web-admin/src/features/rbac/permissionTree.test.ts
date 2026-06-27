import { describe, expect, it } from 'vitest'
import type { Permission } from '@/shared/api/types'
import {
  buildPermissionTree,
  toCheckedIds,
  toCheckedRoleIds,
  toPermissionIds,
} from './permissionTree'

function perm(id: number, code: string, group_name?: string, sort_order?: number): Permission {
  const [object, action] = code.split(':')
  return { id, code, name: code, object, action, group_name, sort_order }
}

const catalog: Permission[] = [
  perm(1, 'user:read', '用户管理', 0),
  perm(2, 'user:create', '用户管理', 1),
  perm(3, 'role:read', '角色权限', 0),
  perm(4, 'project:read', '项目任务', 0),
]

describe('buildPermissionTree', () => {
  it('groups permissions by group_name into two-level tree with id leaf keys', () => {
    const tree = buildPermissionTree(catalog)
    expect(tree).toHaveLength(3)
    expect(tree.map((n) => n.title)).toEqual(['用户管理', '角色权限', '项目任务'])

    const userGroup = tree[0]
    expect(userGroup.key).toBe('group:用户管理')
    expect(userGroup.children).toHaveLength(2)
    // 叶子 key = permission id（数字）
    expect(userGroup.children?.map((c) => c.key)).toEqual([1, 2])
    expect(userGroup.children?.[0].code).toBe('user:read')
    expect(userGroup.children?.[0].title).toContain('user:read')
  })

  it('sorts leaves within a group by sort_order ascending', () => {
    const unsorted: Permission[] = [
      perm(10, 'a:x', 'G', 5),
      perm(11, 'a:y', 'G', 1),
      perm(12, 'a:z', 'G', 3),
    ]
    const tree = buildPermissionTree(unsorted)
    expect(tree[0].children?.map((c) => c.key)).toEqual([11, 12, 10])
  })

  it('falls back to an ungrouped bucket when group_name is missing', () => {
    const tree = buildPermissionTree([perm(20, 'misc:read')])
    expect(tree).toHaveLength(1)
    expect(tree[0].title).toBe('其他')
    expect(tree[0].children?.[0].key).toBe(20)
  })

  it('returns an empty array for an empty catalog', () => {
    expect(buildPermissionTree([])).toEqual([])
  })
})

describe('checked-id mapping helpers', () => {
  it('toCheckedIds maps a role permission list to its id array', () => {
    expect(toCheckedIds([perm(1, 'user:read'), perm(3, 'role:read')])).toEqual([1, 3])
  })

  it('toPermissionIds keeps only numeric leaf keys (drops group parents)', () => {
    expect(toPermissionIds([1, 'group:用户管理', 2, 'group:角色权限', 3])).toEqual([1, 2, 3])
  })

  it('toPermissionIds tolerates bigint keys without including them', () => {
    expect(toPermissionIds([1, 2n, 'group:x', 3])).toEqual([1, 3])
  })

  it('toCheckedRoleIds maps a role list to its id array', () => {
    expect(toCheckedRoleIds([{ id: 7 }, { id: 9 }])).toEqual([7, 9])
  })
})
