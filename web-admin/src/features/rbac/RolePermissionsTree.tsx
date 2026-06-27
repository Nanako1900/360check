import { useMemo } from 'react'
import { Spin, Tree } from 'antd'
import type { Key } from 'react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/shared/ui/EmptyState'
import { buildPermissionTree, toPermissionIds } from './permissionTree'
import { usePermissions } from './api'

interface RolePermissionsTreeProps {
  /** 当前勾选的权限叶子 id（受控）。 */
  checkedIds: number[]
  /** 勾选变更回调：返回纯权限 id 数组（已过滤分组父节点）。 */
  onChange: (permissionIds: number[]) => void
  /** 初始勾选是否仍在加载（角色已有权限拉取中）。 */
  loadingChecked?: boolean
}

/** 权限树（checkable）：全量权限目录按 group_name 分组，叶子勾选项即 permission id。 */
export function RolePermissionsTree({ checkedIds, onChange, loadingChecked }: RolePermissionsTreeProps) {
  const { t } = useTranslation()
  const { data, isLoading } = usePermissions()
  const treeData = useMemo(() => buildPermissionTree(data?.items ?? []), [data])

  if (isLoading || loadingChecked) {
    return (
      <div style={{ display: 'grid', placeItems: 'center', padding: 'var(--space-8)' }}>
        <Spin />
      </div>
    )
  }

  if (treeData.length === 0) {
    return <EmptyState description={t('rbac.permissionsEmpty')} />
  }

  return (
    <Tree
      checkable
      selectable={false}
      defaultExpandAll
      treeData={treeData}
      checkedKeys={checkedIds}
      onCheck={(checked) => {
        // checkStrictly 默认 false → checked 为 Key[]（已含半选父节点的处理）
        const keys = (Array.isArray(checked) ? checked : checked.checked) as Key[]
        onChange(toPermissionIds(keys))
      }}
    />
  )
}
