import { App as AntdApp, Alert, Form, Input, Modal } from 'antd'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { ApiError } from '@/shared/api/apiError'
import type { User } from '@/shared/api/types'
import { useResetUserPassword } from './api'

interface ResetPasswordModalProps {
  open: boolean
  /** 目标用户（为 null 时不渲染表单逻辑）。 */
  user: User | null
  onClose: () => void
  onSuccess?: () => void
}

interface FormValues {
  new_password: string
  confirm_password: string
}

/** 密码强度：至少 8 位且含字母与数字（与 ChangePasswordModal 同规则，§7）。 */
const strongPassword = z.string().min(8).regex(/[A-Za-z]/).regex(/\d/)

/** 管理员重置「他人」密码（区别于自助 PUT /auth/password）。 */
export function ResetPasswordModal({ open, user, onClose, onSuccess }: ResetPasswordModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const { message } = AntdApp.useApp()
  const resetPassword = useResetUserPassword(user?.id ?? 0)

  const close = (): void => {
    form.resetFields()
    onClose()
  }

  const handleOk = async (): Promise<void> => {
    if (!user) return
    // 校验失败时安静返回（错误已挂到字段上），避免 onOk 抛出未处理的 promise rejection。
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      await resetPassword.mutateAsync({ new_password: values.new_password })
      message.success(t('rbac.passwordReset'))
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
      title={user ? `${t('rbac.resetPasswordTitle')} · ${user.display_name}` : t('rbac.resetPasswordTitle')}
      onOk={handleOk}
      onCancel={close}
      confirmLoading={resetPassword.isPending}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      destroyOnHidden
    >
      <Alert
        type="info"
        showIcon
        banner
        message={t('rbac.resetPasswordHint')}
        style={{ marginBottom: 'var(--space-5)' }}
      />
      <Form form={form} layout="vertical" requiredMark>
        <Form.Item
          name="new_password"
          label={t('rbac.newPassword')}
          rules={[
            { required: true, message: t('rbac.newPasswordRequired') },
            {
              validator: (_rule, value: string) =>
                !value || strongPassword.safeParse(value).success
                  ? Promise.resolve()
                  : Promise.reject(new Error(t('rbac.passwordWeak'))),
            },
          ]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>
        <Form.Item
          name="confirm_password"
          label={t('rbac.confirmPassword')}
          dependencies={['new_password']}
          rules={[
            { required: true, message: t('rbac.confirmPasswordRequired') },
            ({ getFieldValue }) => ({
              validator: (_rule, value: string) =>
                !value || getFieldValue('new_password') === value
                  ? Promise.resolve()
                  : Promise.reject(new Error(t('rbac.passwordMismatch'))),
            }),
          ]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
