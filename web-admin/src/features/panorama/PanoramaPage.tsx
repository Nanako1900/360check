/**
 * PanoramaPage（§P5）：全景查看入口页。替换原 `/panorama` 占位页，演示按 media id 打开
 * PanoramaModal。支持 URL `?media_id=` 直接打开，或手动输入 media id 后打开。
 *
 * 这是一个轻量入口（真正的进入点是 P4 地图 InfoWindow「看全景」与 P6 问题详情）；此处保留路由
 * 可用并提供可操作的演示，无需引入额外抽象。
 */
import { useMemo, useState } from 'react'
import { Button, Card, Form, InputNumber, Space, Typography } from 'antd'
import { PictureOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { PanoramaModal } from './PanoramaModal'
import type { PanoramaContext } from './PanoramaModal'

const { Title, Paragraph } = Typography

/** 演示用上下文（真实进入点由问题/巡查/项目提供）。 */
const DEMO_CONTEXT: PanoramaContext = {
  projectName: '滨江绿道巡查',
  inspectionStartedAt: '2026-06-20T01:00:00Z',
  inspectorName: '李巡查',
  problemTypeItemId: 201,
  problemStatusItemId: 101,
  problemTitle: '步道路面横向裂缝，长约 1.2m',
}

export function PanoramaPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const initialId = useMemo(() => {
    const raw = searchParams.get('media_id')
    const n = raw ? Number(raw) : NaN
    return Number.isFinite(n) && n > 0 ? n : 7101
  }, [searchParams])

  const [inputId, setInputId] = useState<number | null>(initialId)
  const [openId, setOpenId] = useState<number | null>(searchParams.has('media_id') ? initialId : null)

  return (
    <div className="app-page">
      <Title level={3} style={{ marginBlockEnd: 'var(--space-2)' }}>
        <PictureOutlined style={{ marginInlineEnd: 'var(--space-3)' }} />
        {t('pano.pageTitle')}
      </Title>
      <Paragraph type="secondary">{t('pano.pageHint')}</Paragraph>

      <Card>
        <Form layout="inline" onFinish={() => setOpenId(inputId)}>
          <Form.Item label={t('pano.mediaId')}>
            <InputNumber
              min={1}
              value={inputId}
              onChange={(v) => setInputId(typeof v === 'number' ? v : null)}
              style={{ width: 160 }}
            />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" disabled={!inputId}>
                {t('pano.open')}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <PanoramaModal
        open={openId !== null}
        mediaId={openId}
        context={DEMO_CONTEXT}
        onClose={() => setOpenId(null)}
      />
    </div>
  )
}
