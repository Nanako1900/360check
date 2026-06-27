import { useMemo } from 'react'
import { Empty, Space, Spin, Tag, Timeline, Typography } from 'antd'
import { ArrowRightOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { DictTag } from '@/shared/ui/DictTag'
import { fmt } from '@/shared/time/dayjs'
import type { ProblemProcessingLog as ProblemLog, ProcessingAction, User } from '@/shared/api/types'
import { useUsers } from '@/features/rbac/api'
import { useProblemLogs } from './api'

interface ProblemProcessingLogProps {
  problemId: number
  enabled?: boolean
}

const ACTION_COLOR: Record<ProcessingAction, string> = {
  STATUS_CHANGE: 'blue',
  COMMENT: 'default',
  REASSIGN: 'gold',
}

/**
 * ProblemProcessingLog（§P6）：`GET /problems/{id}/logs` 渲染为追加式审计时间线。
 *
 * **追加式（强约束）**：`problem_processing_log` 是 insert-only、无 updated_at、无软删 →
 * UI 只读，绝不提供编辑 / 删除日志的控件。
 * `STATUS_CHANGE` 行由后端在状态变更事务内写入（D3），前端从不构造此动作。
 * 用户派生文本（note）经 antd Text 节点渲染（React 默认转义），不用 dangerouslySetInnerHTML。
 */
export function ProblemProcessingLog({ problemId, enabled = true }: ProblemProcessingLogProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useProblemLogs(problemId, enabled)
  const { data: usersData } = useUsers({ page: 1, page_size: 200 })

  const userById = useMemo(() => {
    const map = new Map<number, User>()
    for (const u of usersData?.items ?? []) map.set(u.id, u)
    return map
  }, [usersData])

  const logs = data?.items ?? []

  if (isLoading) {
    return (
      <div style={{ display: 'grid', placeItems: 'center', minHeight: 120 }}>
        <Spin />
      </div>
    )
  }

  if (logs.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('problem.logEmpty')} />
  }

  const actionLabel = (action: ProcessingAction): string => {
    if (action === 'STATUS_CHANGE') return t('problem.actionStatusChange')
    if (action === 'REASSIGN') return t('problem.actionReassign')
    return t('problem.actionComment')
  }

  const operatorName = (id: number | null | undefined): string => {
    if (id == null) return t('problem.systemOperator')
    const u = userById.get(id)
    return u ? u.display_name || u.username : `#${id}`
  }

  const items = logs.map((log: ProblemLog) => ({
    key: log.id,
    color: ACTION_COLOR[log.action],
    children: (
      <Space direction="vertical" size={4} style={{ width: '100%' }}>
        <Space size={8} wrap align="center">
          <Tag color={ACTION_COLOR[log.action]}>{actionLabel(log.action)}</Tag>
          {log.action === 'STATUS_CHANGE' ? (
            <Space size={4} align="center">
              <DictTag code="problem_status" itemId={log.from_status_item_id} />
              <ArrowRightOutlined style={{ color: 'var(--color-ink-muted)' }} />
              <DictTag code="problem_status" itemId={log.to_status_item_id} />
            </Space>
          ) : null}
        </Space>
        {log.note ? <Typography.Text>{log.note}</Typography.Text> : null}
        <Space size={12} wrap style={{ color: 'var(--color-ink-muted)', fontSize: 12 }}>
          <span>{operatorName(log.operator_id)}</span>
          <span className="tabular">{fmt(log.created_at)}</span>
        </Space>
      </Space>
    ),
  }))

  return <Timeline items={items} />
}
