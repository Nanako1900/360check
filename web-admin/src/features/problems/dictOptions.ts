import { useMemo } from 'react'
import { useDict } from '@/shared/dict/useDict'

export interface DictOption {
  value: number
  label: string
}

/**
 * 由字典缓存派生「仅 active 项」的 Select 选项（§P6 强约束）：
 * **筛选 / 编辑下拉只列 active 项**；列表 / 详情渲染则用 `DictTag`（含退役项历史容忍）。
 */
export function useActiveDictOptions(code: string): DictOption[] {
  const { data } = useDict(code)
  return useMemo(
    () =>
      (data?.items ?? [])
        .filter((i) => i.is_active !== false)
        .sort((a, b) => a.sort_order - b.sort_order)
        .map((i) => ({ value: i.id, label: i.label })),
    [data],
  )
}
