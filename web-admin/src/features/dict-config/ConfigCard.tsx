import { useEffect } from 'react'
import { App as AntdApp, Button, Card, Descriptions, Form, Input, Space } from 'antd'
import { HistoryOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import type { ZodType } from 'zod'
import { ApiError } from '@/shared/api/apiError'
import { parseJsonWithSchema, stringifyJson } from '@/shared/util/jsonSchema'
import { useConfig, useUpdateConfig } from './api'

interface ConfigCardProps {
  configKey: string
  title: string
  /** 该 config_key 的 value 结构契约（zod）。 */
  schema: ZodType<Record<string, unknown>>
  onViewHistory: (key: string) => void
}

interface FormValues {
  value: string
}

/** 单个 config_key 的编辑卡片：JSON 编辑 + zod 校验 + 版本/哈希展示 + 历史入口。 */
export function ConfigCard({ configKey, title, schema, onViewHistory }: ConfigCardProps) {
  const { t } = useTranslation()
  const [form] = Form.useForm<FormValues>()
  const { message } = AntdApp.useApp()
  const { data, isLoading } = useConfig(configKey)
  const updateConfig = useUpdateConfig(configKey)

  useEffect(() => {
    if (data) form.setFieldsValue({ value: stringifyJson(data.value) })
  }, [data, form])

  const handleSave = async (): Promise<void> => {
    const values = await form.validateFields()
    const parsed = parseJsonWithSchema(values.value, schema)
    if (!parsed.ok) {
      form.setFields([{ name: 'value', errors: [parsed.message] }])
      return
    }
    try {
      await updateConfig.mutateAsync({ value: parsed.value })
      message.success(t('dict.configSaved'))
    } catch (err) {
      if (err instanceof ApiError) message.error(err.message)
    }
  }

  return (
    <Card
      title={title}
      variant="borderless"
      loading={isLoading}
      extra={
        <Button type="text" icon={<HistoryOutlined />} onClick={() => onViewHistory(configKey)}>
          {t('dict.viewHistory')}
        </Button>
      }
    >
      <Descriptions size="small" column={2} style={{ marginBottom: 'var(--space-5)' }}>
        <Descriptions.Item label={t('dict.configVersion')}>
          <span className="tabular">v{data?.version ?? '-'}</span>
        </Descriptions.Item>
        <Descriptions.Item label={t('dict.configHash')}>
          <span className="tabular" style={{ color: 'var(--color-ink-muted)' }}>
            {data?.content_hash ?? '-'}
          </span>
        </Descriptions.Item>
      </Descriptions>
      <Form form={form} layout="vertical">
        <Form.Item
          name="value"
          label={t('dict.configValue')}
          rules={[{ required: true, message: t('dict.configInvalid') }]}
        >
          <Input.TextArea rows={6} className="tabular" spellCheck={false} />
        </Form.Item>
        <Space>
          <Button type="primary" loading={updateConfig.isPending} onClick={handleSave}>
            {t('dict.configSave')}
          </Button>
        </Space>
      </Form>
    </Card>
  )
}
