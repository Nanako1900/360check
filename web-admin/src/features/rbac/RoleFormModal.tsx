import { App as AntdApp, Form, Input, InputNumber, Modal } from 'antd'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import type { Role } from '@/shared/api/types'
import { useCreateRole, useUpdateRole } from './api'

interface RoleFormModalProps {
  open: boolean
  /** 传入则为编辑模式；否则为新建模式。 */
  editing?: Role | null
  onClose: () => void
  onSuccess?: () => void
}

interface FormValues {
  code: string
  name: string
  description?: string
  sort_order?: number
}

/** 角色编码：小写字母 / 数字 / 下划线（边界校验，§7）。 */
const CODE_PATTERN = /^[a-z][a-z0-9_]*$/

export function RoleFormModal({ open, editing, onClose, onSuccess }: RoleFormModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const { message } = AntdApp.useApp()
  const createRole = useCreateRole()
  const updateRole = useUpdateRole()
  const isEdit = Boolean(editing)

  useEffect(() => {
    if (!open) return
    if (editing) {
      form.setFieldsValue({
        code: editing.code,
        name: editing.name,
        description: editing.description,
        sort_order: editing.sort_order,
      })
    } else {
      form.resetFields()
      form.setFieldsValue({ sort_order: 0 })
    }
  }, [open, editing, form])

  const close = (): void => {
    form.resetFields()
    onClose()
  }

  const handleOk = async (): Promise<void> => {
    // 校验失败时安静返回（错误已挂到字段上），避免 onOk 抛出未处理的 promise rejection。
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      if (editing) {
        await updateRole.mutateAsync({
          id: editing.id,
          body: {
            name: values.name,
            description: values.description,
            sort_order: values.sort_order,
          },
        })
      } else {
        await createRole.mutateAsync({
          code: values.code,
          name: values.name,
          description: values.description,
          sort_order: values.sort_order,
        })
      }
      message.success(t('rbac.roleSaved'))
      close()
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        const entries = Object.entries(err.fieldErrors())
        if (entries.length > 0) {
          form.setFields(
            entries.map(([name, msg]) => ({ name: name as keyof FormValues, errors: [msg] })),
          )
        } else {
          message.error(err.message)
        }
      }
    }
  }

  return (
    <Modal
      open={open}
      title={isEdit ? t('rbac.editRole') : t('rbac.newRole')}
      onOk={handleOk}
      onCancel={close}
      confirmLoading={createRole.isPending || updateRole.isPending}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" requiredMark>
        <Form.Item
          name="code"
          label={t('rbac.roleCode')}
          rules={[
            { required: true, message: t('rbac.roleCodeRequired') },
            { pattern: CODE_PATTERN, message: t('rbac.roleCodePattern') },
          ]}
        >
          <Input disabled={isEdit} placeholder="auditor" autoComplete="off" />
        </Form.Item>
        <Form.Item
          name="name"
          label={t('rbac.roleName')}
          rules={[{ required: true, message: t('rbac.roleNameRequired') }]}
        >
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="description" label={t('rbac.roleDescription')}>
          <Input.TextArea rows={2} maxLength={200} showCount />
        </Form.Item>
        <Form.Item name="sort_order" label={t('rbac.roleSortOrder')}>
          <InputNumber min={0} max={999} style={{ width: '100%' }} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
