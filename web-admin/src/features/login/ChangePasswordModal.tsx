import { App as AntdApp, Form, Input, Modal } from 'antd'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { ApiError } from '@/shared/api/apiError'
import { useChangePassword } from '@/shared/auth/useChangePassword'

/** 密码强度：至少 8 位且含字母与数字（边界校验，§7）。 */
const strongPassword = z
  .string()
  .min(8)
  .regex(/[A-Za-z]/)
  .regex(/\d/)

interface ChangePasswordModalProps {
  open: boolean
  onClose: () => void
  onSuccess?: () => void
}

interface FormValues {
  old_password: string
  new_password: string
  confirm_password: string
}

export function ChangePasswordModal({ open, onClose, onSuccess }: ChangePasswordModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const changePassword = useChangePassword()
  const { message } = AntdApp.useApp()

  const close = (): void => {
    form.resetFields()
    onClose()
  }

  const handleOk = async (): Promise<void> => {
    // 校验失败时安静返回（错误已挂到字段上），避免 onOk 抛出未处理的 promise rejection。
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      await changePassword.mutateAsync({
        old_password: values.old_password,
        new_password: values.new_password,
      })
      message.success(t('password.success'))
      close()
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        const entries = Object.entries(err.fieldErrors())
        if (entries.length > 0) {
          // 字段级回填（VALIDATION_FAILED，§4.6）
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
      title={t('password.title')}
      onOk={handleOk}
      onCancel={close}
      confirmLoading={changePassword.isPending}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" requiredMark>
        <Form.Item
          name="old_password"
          label={t('password.old')}
          rules={[{ required: true, message: t('password.oldRequired') }]}
        >
          <Input.Password autoComplete="current-password" />
        </Form.Item>
        <Form.Item
          name="new_password"
          label={t('password.new')}
          rules={[
            { required: true, message: t('password.newRequired') },
            {
              validator: (_rule, value: string) =>
                !value || strongPassword.safeParse(value).success
                  ? Promise.resolve()
                  : Promise.reject(new Error(t('password.weak'))),
            },
          ]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>
        <Form.Item
          name="confirm_password"
          label={t('password.confirm')}
          dependencies={['new_password']}
          rules={[
            { required: true, message: t('password.confirmRequired') },
            ({ getFieldValue }) => ({
              validator: (_rule, value: string) =>
                !value || getFieldValue('new_password') === value
                  ? Promise.resolve()
                  : Promise.reject(new Error(t('password.mismatch'))),
            }),
          ]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
