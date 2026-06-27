import { App as AntdApp, DatePicker, Form, Input, InputNumber, Modal, Select } from 'antd'
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { ApiError } from '@/shared/api/apiError'
import type { DictItem, Project, ProjectStatus } from '@/shared/api/types'
import { useDict } from '@/shared/dict/useDict'
import { validateWithSchema } from '@/shared/util/jsonSchema'
import type { Dayjs } from 'dayjs'
import { dayjs } from '@/shared/time/dayjs'
import { useCreateProject, useUpdateProject } from './api'
import { PROJECT_STATUS_META, PROJECT_STATUS_ORDER } from './status'

interface ProjectFormModalProps {
  open: boolean
  editing?: Project | null
  onClose: () => void
  onSuccess?: () => void
}

interface BaseFormValues {
  code: string
  name: string
  description?: string
  status: ProjectStatus
  date_range?: [Dayjs, Dayjs] | null
}

/** custom_fields 字段定义（由 project_field 字典项驱动）：number 字段读 extra.type==='number'。 */
interface CustomFieldDef {
  key: string
  label: string
  isNumber: boolean
}

function toFieldDefs(items: DictItem[]): CustomFieldDef[] {
  return items
    .filter((i) => i.is_active !== false)
    .map((i) => ({
      key: i.code,
      label: i.label,
      isNumber:
        typeof i.extra === 'object' && i.extra !== null && i.extra.type === 'number',
    }))
}

/** 动态表单字段在 antd Form 中的命名空间，避免与基础字段键冲突。 */
const CF = 'cf'

export function ProjectFormModal({ open, editing, onClose, onSuccess }: ProjectFormModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm()
  const { message } = AntdApp.useApp()
  const createProject = useCreateProject()
  const updateProject = useUpdateProject()
  const isEdit = Boolean(editing)

  const { data: fieldDict } = useDict('project_field', open)
  const fieldDefs = useMemo(() => toFieldDefs(fieldDict?.items ?? []), [fieldDict])

  useEffect(() => {
    if (!open) return
    if (editing) {
      const cf: Record<string, unknown> = {}
      for (const def of fieldDefs) {
        cf[def.key] = editing.custom_fields?.[def.key] ?? undefined
      }
      form.setFieldsValue({
        code: editing.code,
        name: editing.name,
        description: editing.description,
        status: editing.status,
        date_range:
          editing.start_date && editing.end_date
            ? [dayjs(editing.start_date), dayjs(editing.end_date)]
            : null,
        [CF]: cf,
      })
    } else {
      form.resetFields()
      form.setFieldsValue({ status: 'ACTIVE' })
    }
  }, [open, editing, form, fieldDefs])

  const close = (): void => {
    form.resetFields()
    onClose()
  }

  /** 按字段定义构造 zod schema，校验动态 custom_fields（§7 边界校验）。 */
  const buildCustomFieldsSchema = (): z.ZodType<Record<string, unknown>> => {
    const shape: Record<string, z.ZodTypeAny> = {}
    for (const def of fieldDefs) {
      shape[def.key] = def.isNumber
        ? z.number({ message: t('proj.cfNumberInvalid') }).optional()
        : z.string().optional()
    }
    return z.object(shape).partial()
  }

  const collectCustomFields = (
    raw: Record<string, unknown> | undefined,
  ): Record<string, unknown> => {
    const out: Record<string, unknown> = {}
    for (const def of fieldDefs) {
      const value = raw?.[def.key]
      if (value !== undefined && value !== null && value !== '') out[def.key] = value
    }
    return out
  }

  const handleOk = async (): Promise<void> => {
    const values = (await form.validateFields().catch(() => null)) as
      | (BaseFormValues & { [CF]?: Record<string, unknown> })
      | null
    if (!values) return

    const customRaw = collectCustomFields(values[CF])
    const parsed = validateWithSchema(customRaw, buildCustomFieldsSchema())
    if (!parsed.ok) {
      message.error(parsed.message)
      return
    }
    const custom_fields = Object.keys(parsed.value).length > 0 ? parsed.value : undefined
    const [start, end] = values.date_range ?? [null, null]
    // date 字段为纯日期：直接取本地墙钟日期，避免 UTC 转换跨日漂移（§P3 时间约定）。
    const start_date = start ? start.format('YYYY-MM-DD') : null
    const end_date = end ? end.format('YYYY-MM-DD') : null

    try {
      if (editing) {
        await updateProject.mutateAsync({
          id: editing.id,
          body: {
            name: values.name,
            description: values.description,
            status: values.status,
            custom_fields,
            start_date,
            end_date,
          },
        })
      } else {
        await createProject.mutateAsync({
          code: values.code,
          name: values.name,
          description: values.description,
          status: values.status,
          custom_fields,
          ...(start_date ? { start_date } : {}),
          ...(end_date ? { end_date } : {}),
        })
      }
      message.success(t('proj.saved'))
      close()
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError) {
        const entries = Object.entries(err.fieldErrors())
        if (entries.length > 0) {
          form.setFields(entries.map(([name, msg]) => ({ name, errors: [msg] })))
        } else {
          message.error(err.message)
        }
      }
    }
  }

  return (
    <Modal
      open={open}
      title={isEdit ? t('proj.editProject') : t('proj.newProject')}
      onOk={handleOk}
      onCancel={close}
      confirmLoading={createProject.isPending || updateProject.isPending}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      destroyOnHidden
      width={560}
    >
      <Form form={form} layout="vertical" requiredMark>
        <Form.Item
          name="code"
          label={t('proj.code')}
          rules={[{ required: true, message: t('proj.codeRequired') }]}
        >
          <Input disabled={isEdit} placeholder="PRJ-2026-001" autoComplete="off" />
        </Form.Item>
        <Form.Item
          name="name"
          label={t('proj.name')}
          rules={[{ required: true, message: t('proj.nameRequired') }]}
        >
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="description" label={t('proj.description')}>
          <Input.TextArea rows={2} autoComplete="off" />
        </Form.Item>
        <Form.Item
          name="status"
          label={t('proj.status')}
          rules={[{ required: true, message: t('proj.statusRequired') }]}
        >
          <Select
            options={PROJECT_STATUS_ORDER.map((s) => ({
              value: s,
              label: PROJECT_STATUS_META[s].label,
            }))}
          />
        </Form.Item>
        <Form.Item name="date_range" label={t('proj.dateRange')}>
          <DatePicker.RangePicker style={{ width: '100%' }} />
        </Form.Item>
        {fieldDefs.length > 0 ? (
          <fieldset
            style={{
              border: '1px solid var(--ant-color-border, #e2e8f0)',
              borderRadius: 'var(--radius-md)',
              padding: 'var(--space-5)',
              marginBlockEnd: 'var(--space-4)',
            }}
          >
            <legend style={{ padding: '0 var(--space-3)', color: 'var(--color-ink-muted)' }}>
              {t('proj.customFields')}
            </legend>
            {fieldDefs.map((def) => (
              <Form.Item key={def.key} name={[CF, def.key]} label={def.label}>
                {def.isNumber ? (
                  <InputNumber style={{ width: '100%' }} className="tabular" />
                ) : (
                  <Input autoComplete="off" />
                )}
              </Form.Item>
            ))}
          </fieldset>
        ) : null}
      </Form>
    </Modal>
  )
}
