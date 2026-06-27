import { useEffect } from 'react'
import { App as AntdApp, Button, Form, Input, Select, Space } from 'antd'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import type { Problem, ProblemUpdate } from '@/shared/api/types'
import { useUpdateProblem } from './api'
import { useActiveDictOptions } from './dictOptions'

interface ProblemEditFormProps {
  problem: Problem
  /** UI 门控：无 problem:update 权限时只读（仍由后端 casbin 最终鉴权）。 */
  canUpdate: boolean
}

interface FormValues {
  type_item_id?: number | null
  status_item_id?: number | null
  category_item_id?: number | null
  title?: string
  description?: string
  note?: string
}

/**
 * ProblemEditForm（§P6 / D3 强约束）：`PUT /problems/{id}` 改分类/状态/备注等。
 *
 * **状态变更 = 单次 PUT**：改 `status_item_id` 走这一个 PUT，后端在同一事务内自动追加
 * `STATUS_CHANGE` 日志。前端**绝不**额外 POST `STATUS_CHANGE`（仅 useUpdateProblem，无 logs POST）。
 *
 * 下拉只列 active 字典项；若问题原值引用了退役项，仍以受控值展示（option 缺失时 antd 显示原 id，
 * 详情区另用 DictTag 渲染人类可读标签）。字段级错误按 VALIDATION_FAILED 回填。
 */
export function ProblemEditForm({ problem, canUpdate }: ProblemEditFormProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const { message } = AntdApp.useApp()
  const updateProblem = useUpdateProblem(problem.id)

  const typeOptions = useActiveDictOptions('problem_type')
  const statusOptions = useActiveDictOptions('problem_status')
  const categoryOptions = useActiveDictOptions('problem_category')

  useEffect(() => {
    form.setFieldsValue({
      type_item_id: problem.type_item_id ?? undefined,
      status_item_id: problem.status_item_id ?? undefined,
      category_item_id: problem.category_item_id ?? undefined,
      title: problem.title ?? undefined,
      description: problem.description ?? undefined,
      note: problem.note ?? undefined,
    })
  }, [problem, form])

  const handleSubmit = async (): Promise<void> => {
    const values = await form.validateFields()
    // 单次 PUT（D3）：状态变更也只走这里，后端原子追加 STATUS_CHANGE，前端绝不 POST log。
    const body: ProblemUpdate = {
      type_item_id: values.type_item_id ?? null,
      status_item_id: values.status_item_id ?? null,
      category_item_id: values.category_item_id ?? null,
      title: values.title ?? '',
      description: values.description ?? '',
      note: values.note ?? '',
    }
    try {
      await updateProblem.mutateAsync(body)
      message.success(t('problem.saved'))
    } catch (err) {
      if (err instanceof ApiError) {
        const entries = Object.entries(err.fieldErrors())
        if (entries.length > 0) {
          form.setFields(
            entries.map(([name, msg]) => ({ name: name as keyof FormValues, errors: [msg] })),
          )
        } else if (err.code === 'FORBIDDEN') {
          message.error(t('problem.forbidden'))
        } else if (err.code === 'CONFLICT') {
          message.error(t('problem.conflict'))
        } else {
          message.error(err.message)
        }
      }
    }
  }

  return (
    <Form form={form} layout="vertical" disabled={!canUpdate}>
      <Form.Item name="status_item_id" label={t('problem.status')}>
        <Select allowClear placeholder={t('problem.statusPlaceholder')} options={statusOptions} />
      </Form.Item>
      <Form.Item name="type_item_id" label={t('problem.type')}>
        <Select allowClear placeholder={t('problem.typePlaceholder')} options={typeOptions} />
      </Form.Item>
      <Form.Item name="category_item_id" label={t('problem.category')}>
        <Select allowClear placeholder={t('problem.categoryPlaceholder')} options={categoryOptions} />
      </Form.Item>
      <Form.Item name="title" label={t('problem.titleCol')}>
        <Input maxLength={200} autoComplete="off" />
      </Form.Item>
      <Form.Item name="description" label={t('problem.description')}>
        <Input.TextArea rows={3} maxLength={2000} />
      </Form.Item>
      <Form.Item name="note" label={t('problem.note')}>
        <Input.TextArea rows={2} maxLength={2000} />
      </Form.Item>
      <Space>
        <Button
          type="primary"
          onClick={handleSubmit}
          loading={updateProblem.isPending}
          disabled={!canUpdate}
        >
          {t('common.save')}
        </Button>
      </Space>
    </Form>
  )
}
