import { App as AntdApp, ColorPicker, Form, Input, InputNumber, Modal } from 'antd'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import type { DictItem, DictType } from '@/shared/api/types'
import {
  capturePresetExtraSchema,
  parseJsonWithSchema,
  stringifyJson,
} from '@/shared/util/jsonSchema'
import { useCreateDictItem, useUpdateDictItem } from './api'

interface DictItemFormModalProps {
  open: boolean
  dictType: DictType
  editing?: DictItem | null
  onClose: () => void
  onSuccess?: () => void
}

interface FormValues {
  code: string
  label: string
  color?: string
  sort_order?: number
  extra?: string
}

/** 把 antd ColorPicker 的值统一成 #rrggbb 字符串（兼容字符串/对象两种形态）。 */
function colorToHex(value: unknown): string | undefined {
  if (typeof value === 'string') return value || undefined
  if (value && typeof value === 'object' && 'toHexString' in value) {
    const fn = (value as { toHexString: () => string }).toHexString
    return typeof fn === 'function' ? fn.call(value) : undefined
  }
  return undefined
}

export function DictItemFormModal({
  open,
  dictType,
  editing,
  onClose,
  onSuccess,
}: DictItemFormModalProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const { message } = AntdApp.useApp()
  const createItem = useCreateDictItem()
  const updateItem = useUpdateDictItem()
  const isEdit = Boolean(editing)
  const isCapturePreset = dictType.scope === 'capture_preset'

  useEffect(() => {
    if (!open) return
    if (editing) {
      form.setFieldsValue({
        code: editing.code,
        label: editing.label,
        color: editing.color ?? undefined,
        sort_order: editing.sort_order,
        extra: isCapturePreset
          ? stringifyJson(editing.extra ?? { width: 4096, quality: 80 })
          : undefined,
      })
    } else {
      form.resetFields()
      form.setFieldsValue({
        sort_order: 0,
        extra: isCapturePreset ? stringifyJson({ width: 4096, quality: 80 }) : undefined,
      })
    }
  }, [open, editing, form, isCapturePreset])

  const close = (): void => {
    form.resetFields()
    onClose()
  }

  const handleOk = async (): Promise<void> => {
    const values = await form.validateFields()
    const color = colorToHex(values.color)

    // 拍照预设：extra jsonb 必须通过 zod 结构校验，永不信任手填 JSON（§7）。
    let extra: Record<string, unknown> | undefined
    if (isCapturePreset) {
      const parsed = parseJsonWithSchema(values.extra ?? '', capturePresetExtraSchema)
      if (!parsed.ok) {
        form.setFields([{ name: 'extra', errors: [parsed.message] }])
        return
      }
      extra = parsed.value
    }

    try {
      if (editing) {
        await updateItem.mutateAsync({
          id: editing.id,
          code: dictType.code,
          body: {
            label: values.label,
            color: color ?? null,
            sort_order: values.sort_order,
            ...(extra ? { extra } : {}),
          },
        })
      } else {
        await createItem.mutateAsync({
          code: dictType.code,
          body: {
            dict_type_id: dictType.id,
            code: values.code,
            label: values.label,
            color,
            sort_order: values.sort_order,
            ...(extra ? { extra } : {}),
          },
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
      title={isEdit ? t('dict.editItem') : t('dict.newItem')}
      onOk={handleOk}
      onCancel={close}
      confirmLoading={createItem.isPending || updateItem.isPending}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" requiredMark>
        <Form.Item
          name="code"
          label={t('dict.itemCode')}
          rules={[{ required: true, message: t('dict.itemCodeRequired') }]}
        >
          <Input disabled={isEdit} placeholder="OPEN" autoComplete="off" />
        </Form.Item>
        <Form.Item
          name="label"
          label={t('dict.itemLabel')}
          rules={[{ required: true, message: t('dict.itemLabelRequired') }]}
        >
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="color" label={t('dict.itemColor')}>
          <ColorPicker format="hex" allowClear />
        </Form.Item>
        <Form.Item name="sort_order" label={t('dict.itemSort')}>
          <InputNumber min={0} max={999} style={{ width: '100%' }} />
        </Form.Item>
        {isCapturePreset ? (
          <Form.Item
            name="extra"
            label={t('dict.itemExtra')}
            extra={t('dict.extraHintCapture')}
            rules={[{ required: true, message: t('dict.extraInvalid') }]}
          >
            <Input.TextArea rows={4} className="tabular" spellCheck={false} />
          </Form.Item>
        ) : null}
      </Form>
    </Modal>
  )
}
