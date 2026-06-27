import type { Permission } from '@/shared/api/types'

/**
 * 权限树节点（驱动 antd `Tree` checkable，§P2）。
 * - 分组父节点：key = `group:{group_name}`（字符串，避免与叶子 id 冲突），不可勾选独立语义，仅聚合。
 * - 权限叶子：key = permission.id（数字），勾选项即为后端 permission id。
 */
export interface PermissionTreeNode {
  /** antd Tree 节点 key。父节点为 `group:` 前缀字符串；叶子为 permission id（数字）。 */
  key: string | number
  /** 节点显示标题。 */
  title: string
  /** 叶子携带的权限码（如 `project:read`）；父节点为 undefined。 */
  code?: string
  /** 子节点（叶子无此字段）。 */
  children?: PermissionTreeNode[]
}

/** 未分组权限的兜底组名。 */
const UNGROUPED = '其他'

/** 分组父节点 key 前缀。 */
const GROUP_PREFIX = 'group:'

/**
 * 把扁平权限目录按 `group_name` 分组成两层树（组 → 权限叶子）。
 * - 组按首次出现顺序保留；组内权限按 `sort_order`（缺省回退原顺序）升序。
 * - 叶子 key = permission.id，title = `name（code）`。纯函数，无副作用。
 */
export function buildPermissionTree(permissions: Permission[]): PermissionTreeNode[] {
  const order: string[] = []
  const groups = new Map<string, Permission[]>()

  for (const perm of permissions) {
    const group = perm.group_name && perm.group_name.length > 0 ? perm.group_name : UNGROUPED
    if (!groups.has(group)) {
      groups.set(group, [])
      order.push(group)
    }
    groups.get(group)?.push(perm)
  }

  return order.map((group) => {
    const items = [...(groups.get(group) ?? [])].sort(sortByOrder)
    return {
      key: `${GROUP_PREFIX}${group}`,
      title: group,
      children: items.map((perm) => ({
        key: perm.id,
        title: `${perm.name}（${perm.code}）`,
        code: perm.code,
      })),
    }
  })
}

/** 稳定排序：sort_order 升序，缺省视为大值以排后。 */
function sortByOrder(a: Permission, b: Permission): number {
  const ao = a.sort_order ?? Number.MAX_SAFE_INTEGER
  const bo = b.sort_order ?? Number.MAX_SAFE_INTEGER
  return ao - bo
}

/** 角色当前权限对象数组 → 勾选的叶子 id 数组（antd Tree `checkedKeys`）。 */
export function toCheckedIds(rolePermissions: Permission[]): number[] {
  return rolePermissions.map((p) => p.id)
}

/**
 * antd Tree 勾选的 key 集合 → SetRolePermissionsRequest 的 permission_ids。
 * 过滤掉分组父节点 key（`group:` 前缀字符串），仅保留数字叶子 id。
 * 入参兼容 React.Key（string | number | bigint），无需引入 React 依赖。
 */
export function toPermissionIds(checkedKeys: ReadonlyArray<string | number | bigint>): number[] {
  return checkedKeys.filter((k): k is number => typeof k === 'number')
}

/** 角色集合 → 勾选的角色 id 数组（用户角色抽屉复用）。 */
export function toCheckedRoleIds(roles: { id: number }[]): number[] {
  return roles.map((r) => r.id)
}
