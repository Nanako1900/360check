import { App as AntdApp, Form, Input, Modal, Select } from 'antd'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import type { InspectionTask, TaskStatus, User } from '@/shared/api/types'
import { useUsers } from '@/features/rbac/api'
import { useCreateTask, useUpdateTask } from './api'
import { TASK_STATUS_META, TASK_STATUS_ORDER } from './status'

interface TaskFormModalProps {
  open: boolean
  projectId: number
  editing?: InspectionTask | null
  onClose: () => void
}

interface FormValues {
  title: string
  description?: string
  status: TaskStatus
  assignee_id?: number | null
}

export function TaskFormModal({ open, projectId, editing, onClose }: TaskFormModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const { message } = AntdApp.useApp()
  const createTask = useCreateTask()
  const updateTask = useUpdateTask()
  const isEdit = Boolean(editing)

  // 巡查员候选：复用 rbac useUsers（§P3 指派复用）。
  const { data: usersData } = useUsers({ page: 1, page_size: 100 })
  const assigneeOptions = (usersData?.items ?? []).map((u: User) => ({
    value: u.id,
    label: u.display_name || u.username,
  }))

  useEffect(() => {
    if (!open) return
    if (editing) {
      form.setFieldsValue({
        title: editing.title,
        description: editing.description,
        status: editing.status,
        assignee_id: editing.assignee_id ?? undefined,
      })
    } else {
      form.resetFields()
      form.setFieldsValue({ status: 'PENDING' })
    }
  }, [open, editing, form])

  const close = (): void => {
    form.resetFields()
    onClose()
  }

  const handleOk = async (): Promise<void> => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      if (editing) {
        await updateTask.mutateAsync({
          id: editing.id,
          body: {
            title: values.title,
            description: values.description,
            status: values.status,
            assignee_id: values.assignee_id ?? null,
          },
        })
      } else {
        await createTask.mutateAsync({
          project_id: projectId,
          title: values.title,
          description: values.description,
          status: values.status,
          assignee_id: values.assignee_id ?? null,
        })
      }
      message.success(t('proj.taskSaved'))
      close()
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
      title={isEdit ? t('proj.editTask') : t('proj.newTask')}
      onOk={handleOk}
      onCancel={close}
      confirmLoading={createTask.isPending || updateTask.isPending}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" requiredMark>
        <Form.Item
          name="title"
          label={t('proj.taskTitle')}
          rules={[{ required: true, message: t('proj.taskTitleRequired') }]}
        >
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="description" label={t('proj.description')}>
          <Input.TextArea rows={2} autoComplete="off" />
        </Form.Item>
        <Form.Item
          name="status"
          label={t('proj.taskStatus')}
          rules={[{ required: true, message: t('proj.statusRequired') }]}
        >
          <Select
            options={TASK_STATUS_ORDER.map((s) => ({
              value: s,
              label: TASK_STATUS_META[s].label,
            }))}
          />
        </Form.Item>
        <Form.Item name="assignee_id" label={t('proj.assignee')}>
          <Select allowClear placeholder={t('proj.assigneePlaceholder')} options={assigneeOptions} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
