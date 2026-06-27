import { App as AntdApp, Form, Input, Modal, Select, Switch } from 'antd'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { ApiError } from '@/shared/api/apiError'
import type { Role, User } from '@/shared/api/types'
import { useCreateUser, useRoles, useUpdateUser } from './api'

interface UserFormModalProps {
  open: boolean
  /** 传入则为编辑模式；否则为新建模式。 */
  editing?: User | null
  onClose: () => void
  onSuccess?: () => void
}

interface FormValues {
  username: string
  password?: string
  display_name?: string
  phone?: string
  email?: string
  is_active: boolean
  role_ids?: number[]
}

/** 用户名：字母 / 数字 / 下划线，3–32 位（边界校验，§7）。 */
const USERNAME_PATTERN = /^[A-Za-z][A-Za-z0-9_]{2,31}$/

/** 密码强度：至少 8 位且含字母与数字（与 ChangePasswordModal 同规则，§7）。 */
const strongPassword = z.string().min(8).regex(/[A-Za-z]/).regex(/\d/)

export function UserFormModal({ open, editing, onClose, onSuccess }: UserFormModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const { message } = AntdApp.useApp()
  const createUser = useCreateUser()
  const updateUser = useUpdateUser()
  const isEdit = Boolean(editing)
  const { data: rolesData } = useRoles()
  const roleOptions = (rolesData?.items ?? []).map((r: Role) => ({ value: r.id, label: r.name }))

  useEffect(() => {
    if (!open) return
    if (editing) {
      form.setFieldsValue({
        username: editing.username,
        display_name: editing.display_name,
        phone: editing.phone ?? undefined,
        email: editing.email ?? undefined,
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
    // 校验失败时安静返回（错误已挂到字段上），避免 onOk 抛出未处理的 promise rejection。
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      if (editing) {
        await updateUser.mutateAsync({
          id: editing.id,
          body: {
            display_name: values.display_name,
            phone: values.phone ?? null,
            email: values.email ?? null,
            is_active: values.is_active,
          },
        })
      } else {
        await createUser.mutateAsync({
          username: values.username,
          password: values.password ?? '',
          display_name: values.display_name,
          phone: values.phone,
          email: values.email,
          is_active: values.is_active,
          role_ids: values.role_ids,
        })
      }
      message.success(t('rbac.userSaved'))
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
      title={isEdit ? t('rbac.editUser') : t('rbac.newUser')}
      onOk={handleOk}
      onCancel={close}
      confirmLoading={createUser.isPending || updateUser.isPending}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" requiredMark>
        <Form.Item
          name="username"
          label={t('rbac.username')}
          rules={[
            { required: true, message: t('rbac.usernameRequired') },
            { pattern: USERNAME_PATTERN, message: t('rbac.usernamePattern') },
          ]}
        >
          <Input disabled={isEdit} placeholder="inspector_li" autoComplete="off" />
        </Form.Item>
        {!isEdit ? (
          <Form.Item
            name="password"
            label={t('rbac.password')}
            rules={[
              { required: true, message: t('rbac.passwordRequired') },
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
        ) : null}
        <Form.Item name="display_name" label={t('rbac.displayName')}>
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="phone" label={t('rbac.phone')}>
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item
          name="email"
          label={t('rbac.email')}
          rules={[{ type: 'email', message: t('rbac.emailInvalid') }]}
        >
          <Input autoComplete="off" />
        </Form.Item>
        {!isEdit ? (
          <Form.Item name="role_ids" label={t('rbac.initialRoles')}>
            <Select
              mode="multiple"
              allowClear
              options={roleOptions}
              placeholder={t('rbac.initialRolesPlaceholder')}
            />
          </Form.Item>
        ) : null}
        <Form.Item name="is_active" label={t('rbac.colActive')} valuePropName="checked">
          <Switch checkedChildren={t('rbac.active')} unCheckedChildren={t('rbac.inactive')} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
