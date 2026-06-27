import type { ProjectStatus, TaskStatus } from '@/shared/api/types'

/** 项目状态：冻结字面量（ACTIVE/PAUSED/ARCHIVED）→ 中文标签 + antd Tag 颜色（§4.5）。 */
export interface StatusMeta {
  label: string
  color: string
}

export const PROJECT_STATUS_META: Record<ProjectStatus, StatusMeta> = {
  ACTIVE: { label: '进行中', color: 'green' },
  PAUSED: { label: '已暂停', color: 'orange' },
  ARCHIVED: { label: '已归档', color: 'default' },
}

export const PROJECT_STATUS_ORDER: readonly ProjectStatus[] = ['ACTIVE', 'PAUSED', 'ARCHIVED']

/** 任务状态：冻结字面量（PENDING/IN_PROGRESS/COMPLETED/ARCHIVED，绝不用 OPEN/DONE）。 */
export const TASK_STATUS_META: Record<TaskStatus, StatusMeta> = {
  PENDING: { label: '待开始', color: 'default' },
  IN_PROGRESS: { label: '进行中', color: 'processing' },
  COMPLETED: { label: '已完成', color: 'success' },
  ARCHIVED: { label: '已归档', color: 'default' },
}

export const TASK_STATUS_ORDER: readonly TaskStatus[] = [
  'PENDING',
  'IN_PROGRESS',
  'COMPLETED',
  'ARCHIVED',
]
