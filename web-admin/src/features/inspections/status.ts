import type { InspectionStatus } from '@/shared/api/types'

/** 巡查状态：冻结字面量（IN_PROGRESS/FINISHED/ABANDONED）→ 中文标签 + antd Tag 颜色（§4.5）。 */
export interface InspectionStatusMeta {
  label: string
  color: string
}

export const INSPECTION_STATUS_META: Record<InspectionStatus, InspectionStatusMeta> = {
  IN_PROGRESS: { label: '进行中', color: 'processing' },
  FINISHED: { label: '已结束', color: 'success' },
  ABANDONED: { label: '已放弃', color: 'default' },
}

export const INSPECTION_STATUS_ORDER: readonly InspectionStatus[] = [
  'IN_PROGRESS',
  'FINISHED',
  'ABANDONED',
]
