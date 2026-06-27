import { useEffect, useState } from 'react'
import { App as AntdApp, Form, Input, Modal, Select } from 'antd'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import type { ClientLogAction, ProblemLogCreate, User } from '@/shared/api/types'
import { useUsers } from '@/features/rbac/api'
import { useAppendProblemLog } from './api'

interface AddProcessingLogModalProps {
  open: boolean
  problemId: number
  onClose: () => void
}

interface FormValues {
  action: ClientLogAction
  note?: string
  operator_id?: number
}

/** 前端可选动作仅 COMMENT / REASSIGN（D3：STATUS_CHANGE 后端独写，此处类型层面已排除）。 */
const CLIENT_ACTIONS: ClientLogAction[] = ['COMMENT', 'REASSIGN']

/**
 * AddProcessingLogModal（§P6 / D3 强约束）：`POST /problems/{id}/logs`，**仅** COMMENT（纯备注）
 * 或 REASSIGN（改派，operator_id = 选中的新负责人）。
 *
 * **前端绝不构造 STATUS_CHANGE payload**：`action` 受 `ClientLogAction`（=`'COMMENT'|'REASSIGN'`）
 * 约束；状态变更必须走 ProblemEditForm 的单次 PUT。处理日志追加式 → 无编辑/删除。
 */
export function AddProcessingLogModal({ open, problemId, onClose }: AddProcessingLogModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const { message } = AntdApp.useApp()
  const appendLog = useAppendProblemLog(problemId)
  const [action, setAction] = useState<ClientLogAction>('COMMENT')

  const { data: usersData } = useUsers({ page: 1, page_size: 200 })
  const userOptions = (usersData?.items ?? []).map((u: User) => ({
    value: u.id,
    label: u.display_name || u.username,
  }))

  useEffect(() => {
    if (open) {
      form.resetFields()
      setAction('COMMENT')
    }
  }, [open, form])

  const actionLabel = (a: ClientLogAction): string =>
    a === 'REASSIGN' ? t('problem.actionReassign') : t('problem.actionComment')

  const handleOk = async (): Promise<void> => {
    const values = await form.validateFields()
    // payload 的 action 永远是 COMMENT/REASSIGN —— 由 ClientLogAction 类型保证，绝不 STATUS_CHANGE。
    const body: ProblemLogCreate = {
      action: values.action,
      ...(values.note ? { note: values.note } : {}),
      ...(values.action === 'REASSIGN' ? { operator_id: values.operator_id ?? null } : {}),
    }
    try {
      await appendLog.mutateAsync(body)
      message.success(t('problem.logAdded'))
      onClose()
    } catch (err) {
      if (err instanceof ApiError) {
        const entries = Object.entries(err.fieldErrors())
        if (entries.length > 0) {
          form.setFields(
            entries.map(([name, msg]) => ({ name: name as keyof FormValues, errors: [msg] })),
          )
        } else if (err.code === 'FORBIDDEN') {
          message.error(t('problem.forbidden'))
        } else {
          message.error(err.message)
        }
      }
    }
  }

  return (
    <Modal
      open={open}
      title={t('problem.addLog')}
      onOk={handleOk}
      onCancel={onClose}
      confirmLoading={appendLog.isPending}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" initialValues={{ action: 'COMMENT' }}>
        <Form.Item
          name="action"
          label={t('problem.logAction')}
          rules={[{ required: true, message: t('problem.logActionRequired') }]}
        >
          <Select
            options={CLIENT_ACTIONS.map((a) => ({ value: a, label: actionLabel(a) }))}
            onChange={(value: ClientLogAction) => setAction(value)}
          />
        </Form.Item>
        {action === 'REASSIGN' ? (
          <Form.Item
            name="operator_id"
            label={t('problem.reassignTo')}
            rules={[{ required: true, message: t('problem.reassignRequired') }]}
          >
            <Select
              showSearch
              optionFilterProp="label"
              placeholder={t('problem.reassignPlaceholder')}
              options={userOptions}
            />
          </Form.Item>
        ) : null}
        <Form.Item
          name="note"
          label={t('problem.logNote')}
          rules={
            action === 'COMMENT'
              ? [{ required: true, message: t('problem.logNoteRequired') }]
              : undefined
          }
        >
          <Input.TextArea rows={3} maxLength={2000} placeholder={t('problem.logNotePlaceholder')} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
