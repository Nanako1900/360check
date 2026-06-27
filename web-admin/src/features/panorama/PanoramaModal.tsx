/**
 * PanoramaModal（§P5）：弹层全景查看器，承载某 media id 的 360 球，并在标题区展示关联上下文
 * —— 所属项目（名称）、巡查（时间 / 巡查员）、问题（类型 via DictTag + 状态）。上下文由打开方
 * 以 props 传入（地图 InfoWindow / 问题详情等各自已有这些字段）。
 *
 * 约束：
 * - 仅渲染 **已确认** 媒体（`verified_at` 非空 / `isRenderable`）；否则示「照片处理中 / 未确认」空态。
 * - 仅用 `tier=web` 的 equirectangular JPG 作球（pickWebUrl）。
 * - 签名 URL 过期 → PanoramaViewer.onExpired → 这里 `refetch()` 取新 signed_url。
 * - 标题区文案对用户派生文本（项目名 / 巡查员）经 antd 文本节点渲染（React 默认转义），不用
 *   dangerouslySetInnerHTML。
 */
import { useCallback } from 'react'
import { Alert, Descriptions, Modal, Space, Spin, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/shared/ui/EmptyState'
import { DictTag } from '@/shared/ui/DictTag'
import { fmt } from '@/shared/time/dayjs'
import { PanoramaViewer } from '@/shared/psv/PanoramaViewer'
import type { PanoViewerFactory } from '@/shared/psv/PanoramaViewer'
import { isRenderable, pickWebUrl } from '@/shared/psv/mediaTier'
import { useMedia } from './api'

const { Text } = Typography

/** 关联上下文（打开方提供；任意字段可缺省）。 */
export interface PanoramaContext {
  projectName?: string | null
  /** 巡查开始时间（UTC ISO，渲染时转 Asia/Shanghai）。 */
  inspectionStartedAt?: string | null
  inspectorName?: string | null
  /** 问题类型字典项 id（软 FK，经 DictTag 渲染 label/color）。 */
  problemTypeItemId?: number | null
  /** 问题状态字典项 id。 */
  problemStatusItemId?: number | null
  problemTitle?: string | null
}

interface PanoramaModalProps {
  open: boolean
  /** 要查看的媒体 id（应为 `tier=web` 的 media）。 */
  mediaId: number | null
  context?: PanoramaContext
  onClose: () => void
  /** 仅供测试注入 PSV Viewer 工厂（透传给 PanoramaViewer）。 */
  viewerFactory?: PanoViewerFactory
}

export function PanoramaModal({
  open,
  mediaId,
  context,
  onClose,
  viewerFactory,
}: PanoramaModalProps) {
  const { t } = useTranslation()
  const id = mediaId ?? 0
  const { data: media, isLoading, isError, refetch } = useMedia(id, open && id > 0)

  // 签名 URL 过期：重拉 media 取新 signed_url（不缓存到持久存储）。
  const handleExpired = useCallback(() => {
    void refetch()
  }, [refetch])

  const webUrl = pickWebUrl(media)
  const renderable = isRenderable(media)

  function renderBody() {
    if (isLoading) {
      return (
        <div style={{ display: 'grid', placeItems: 'center', minHeight: 240 }}>
          <Spin />
        </div>
      )
    }
    if (isError) {
      return (
        <Alert
          type="warning"
          showIcon
          message={t('pano.fetchFailedTitle')}
          description={t('pano.fetchFailedHint')}
        />
      )
    }
    // 未确认（verified_at 为空）或非 web tier / 无签名 URL：不渲染球，给空态。
    if (!renderable || !webUrl) {
      return <EmptyState description={t('pano.unconfirmed')} />
    }
    return <PanoramaViewer panorama={webUrl} onExpired={handleExpired} viewerFactory={viewerFactory} />
  }

  const hasContext =
    !!context &&
    (context.projectName ||
      context.inspectionStartedAt ||
      context.inspectorName ||
      context.problemTitle ||
      context.problemTypeItemId != null ||
      context.problemStatusItemId != null)

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width={960}
      destroyOnClose
      title={
        <Space direction="vertical" size={2} style={{ maxWidth: '100%' }}>
          <Text strong>{context?.problemTitle || t('pano.title')}</Text>
          {context?.problemTitle ? null : (
            <Text type="secondary" style={{ fontWeight: 400, fontSize: 12 }}>
              {t('pano.subtitle')}
            </Text>
          )}
        </Space>
      }
    >
      {hasContext ? (
        <Descriptions
          size="small"
          column={1}
          style={{ marginBlockEnd: 'var(--space-4, 16px)' }}
          colon={false}
        >
          {context?.projectName ? (
            <Descriptions.Item label={t('pano.ctxProject')}>
              <Text>{context.projectName}</Text>
            </Descriptions.Item>
          ) : null}
          {context?.inspectionStartedAt || context?.inspectorName ? (
            <Descriptions.Item label={t('pano.ctxInspection')}>
              <Space size={8} wrap>
                {context?.inspectionStartedAt ? (
                  <Text>{fmt(context.inspectionStartedAt)}</Text>
                ) : null}
                {context?.inspectorName ? (
                  <Text type="secondary">{context.inspectorName}</Text>
                ) : null}
              </Space>
            </Descriptions.Item>
          ) : null}
          {context?.problemTypeItemId != null || context?.problemStatusItemId != null ? (
            <Descriptions.Item label={t('pano.ctxProblem')}>
              <Space size={6} wrap>
                {context?.problemTypeItemId != null ? (
                  <DictTag code="problem_type" itemId={context.problemTypeItemId} />
                ) : null}
                {context?.problemStatusItemId != null ? (
                  <DictTag code="problem_status" itemId={context.problemStatusItemId} />
                ) : null}
              </Space>
            </Descriptions.Item>
          ) : null}
        </Descriptions>
      ) : null}

      {renderBody()}
    </Modal>
  )
}
