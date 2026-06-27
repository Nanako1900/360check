import type { DictScope } from '@/shared/api/types'

/** 6 个 DictScope 字面量 + 中文标签（单源，供筛选 Select 与表单复用）。 */
export interface DictScopeOption {
  value: DictScope
  label: string
}

export const DICT_SCOPES: readonly DictScopeOption[] = [
  { value: 'problem_type', label: '问题类型' },
  { value: 'problem_status', label: '问题状态' },
  { value: 'problem_category', label: '问题分类' },
  { value: 'project_field', label: '项目字段' },
  { value: 'capture_preset', label: '拍照预设' },
  { value: 'misc', label: '其他' },
]

const SCOPE_LABELS: Record<DictScope, string> = DICT_SCOPES.reduce(
  (acc, s) => ({ ...acc, [s.value]: s.label }),
  {} as Record<DictScope, string>,
)

/** 作用域 → 中文标签（未知值回退原值，避免「未知」）。 */
export function scopeLabel(scope: DictScope): string {
  return SCOPE_LABELS[scope] ?? scope
}
