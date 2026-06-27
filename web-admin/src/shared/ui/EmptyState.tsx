import { Empty } from 'antd'
import type { ReactNode } from 'react'

interface EmptyStateProps {
  description?: ReactNode
  extra?: ReactNode
}

/** 统一空态（§10.4）：中文友好文案 + 可选下一步操作。 */
export function EmptyState({ description = '暂无数据', extra }: EmptyStateProps) {
  return (
    <div style={{ padding: 'var(--space-8)', display: 'grid', placeItems: 'center' }}>
      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={description}>
        {extra}
      </Empty>
    </div>
  )
}
