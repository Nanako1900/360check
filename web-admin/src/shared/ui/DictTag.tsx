import { Tag } from 'antd'
import { useTranslation } from 'react-i18next'
import { useDict } from '@/shared/dict/useDict'

interface DictTagProps {
  /** dict_type.code，如 'problem_status' / 'problem_type' / 'problem_category' */
  code: string
  /** problems.{type|status|category}_item_id（软 FK），可能指向已退役项 */
  itemId: number | null | undefined
}

/**
 * 字典标签（§P1）：按 item_id 渲染 label + color。
 * **历史容忍**：缓存含退役（is_active=false）项 → 仍正常渲染并附「（已停用）」灰色后缀，绝不显示「未知」。
 * 不以颜色作为唯一信息载体（§10.2）：始终带文字 label。
 */
export function DictTag({ code, itemId }: DictTagProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useDict(code)

  if (itemId === null || itemId === undefined) return <Tag>—</Tag>

  const item = data?.items.find((i) => i.id === itemId)
  if (!item) {
    // 缓存未就绪或项缺失：中性占位，不报错（历史容忍）
    return <Tag bordered={false}>{isLoading ? '…' : `#${itemId}`}</Tag>
  }

  const retired = item.is_active === false
  return (
    <Tag color={item.color ?? undefined} bordered={!item.color}>
      {item.label}
      {retired ? (
        <span style={{ color: '#94a3b8', marginInlineStart: 4 }}>{t('dict.retiredSuffix')}</span>
      ) : null}
    </Tag>
  )
}
