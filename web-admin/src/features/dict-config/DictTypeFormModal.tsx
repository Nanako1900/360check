import { App as AntdApp, Form, Input, Modal, Select, Switch } from 'antd'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import type { DictScope, DictType } from '@/shared/api/types'
import { DICT_SCOPES } from './scopes'
import { useCreateDictType, useUpdateDictType } from './api'

interface DictTypeFormModalProps {
  open: boolean
  /** 传入则为编辑模式；否则为新建模式。 */
  editing?: DictType | null
  onClose: () => void
  onSuccess?: () => void
}

interface FormValues {
  code: string
  name: string
  scope: DictScope
  description?: string
  is_active: boolean
}

/** 机器键：小写字母 / 数字 / 下划线（边界校验，§7）。 */
const CODE_PATTERN = /^[a-z][a-z0-9_]*$/

export function DictTypeFormModal({ open, editing, onClose, onSuccess }: DictTypeFormModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const { message } = AntdApp.useApp()
  const createType = useCreateDictType()
  const updateType = useUpdateDictType()
  const isEdit = Boolean(editing)

  useEffect(() => {
    if (!open) return
    if (editing) {
      form.setFieldsValue({
        code: editing.code,
        name: editing.name,
        scope: editing.scope,
        description: editing.description,
        is_active: editing.is_active,
      })
    } else {
      form.resetFields()
      form.setFieldsValue({ is_active: true })
    }
  }, [open, editing, form])

  const close = (): void => {
    form.resetFields()
    onClose()
  }

  const handleOk = async (): Promise<void> => {
    const values = await form.validateFields()
    try {
      if (editing) {
        await updateType.mutateAsync({
          id: editing.id,
          body: {
            name: values.name,
            description: values.description,
            is_active: values.is_active,
          },
        })
      } else {
        await createType.mutateAsync({
          code: values.code,
          name: values.name,
          scope: values.scope,
          description: values.description,
        })
      }
      message.success(t('dict.saved'))
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
      title={isEdit ? t('dict.editType') : t('dict.newType')}
      onOk={handleOk}
      onCancel={close}
      confirmLoading={createType.isPending || updateType.isPending}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" requiredMark>
        <Form.Item
          name="code"
          label={t('dict.colCode')}
          rules={[
            { required: true, message: t('dict.codeRequired') },
            { pattern: CODE_PATTERN, message: t('dict.codePattern') },
          ]}
        >
          <Input disabled={isEdit} placeholder="problem_type" autoComplete="off" />
        </Form.Item>
        <Form.Item
          name="name"
          label={t('dict.colName')}
          rules={[{ required: true, message: t('dict.nameRequired') }]}
        >
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item
          name="scope"
          label={t('dict.colScope')}
          rules={[{ required: true, message: t('dict.scopeRequired') }]}
        >
          <Select
            disabled={isEdit}
            options={DICT_SCOPES.map((s) => ({ value: s.value, label: s.label }))}
            placeholder={t('dict.scopeRequired')}
          />
        </Form.Item>
        <Form.Item name="description" label={t('dict.colDescription')}>
          <Input.TextArea rows={2} maxLength={200} showCount />
        </Form.Item>
        {isEdit ? (
          <Form.Item name="is_active" label={t('dict.colActive')} valuePropName="checked">
            <Switch checkedChildren={t('dict.active')} unCheckedChildren={t('dict.inactive')} />
          </Form.Item>
        ) : null}
      </Form>
    </Modal>
  )
}
