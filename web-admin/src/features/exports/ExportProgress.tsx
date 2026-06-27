import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Modal, Progress, Space, Typography } from 'antd'
import { DownloadOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import { http } from '@/shared/api/http'
import type { ExportJob } from '@/shared/api/types'
import { useExportProgress } from './api'
import { triggerDownload } from './download'

const { Text } = Typography

interface ExportProgressProps {
  /** 任务句柄；null 表示弹层关闭/无任务。 */
  jobUuid: string | null
  open: boolean
  onClose: () => void
  /** 轮询回退间隔（ms），默认 2000；测试可调小以加速。 */
  pollIntervalMs?: number
}

/**
 * 导出进度弹层（§P8）：SSE 优先 + ~2s 轮询回退（useExportProgress 内部处理）。
 *
 * - 进度条展示 processed/total + %；SUCCEEDED 自动下载 result_url（并保留手动下载兜底）。
 * - FAILED 展示 error；URL 过期则「重新获取下载链接」重查 `GET /exports/{job_uuid}` 拿新 result_url。
 * - 终态前可取消/关闭（中断订阅与轮询）。
 */
export function ExportProgress({
  jobUuid,
  open,
  onClose,
  pollIntervalMs = 2000,
}: ExportProgressProps) {
  const { t } = useTranslation()
  const { status, progress, processedRows, totalRows, resultUrl, error, polling } =
    useExportProgress(open ? jobUuid : null, pollIntervalMs)

  // 自动下载只触发一次（每个 job）。
  const autoDownloadedRef = useRef<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [refreshError, setRefreshError] = useState<string | null>(null)

  const isSucceeded = status === 'SUCCEEDED'
  const isFailed = status === 'FAILED'
  const isCancelled = status === 'CANCELLED'
  const isTerminal = isSucceeded || isFailed || isCancelled

  // SUCCEEDED + result_url → 自动触发一次下载。
  useEffect(() => {
    if (isSucceeded && resultUrl && autoDownloadedRef.current !== resultUrl) {
      autoDownloadedRef.current = resultUrl
      triggerDownload(resultUrl)
    }
  }, [isSucceeded, resultUrl])

  // 关闭后重置一次性下载守卫，便于下次同弹层复用。
  useEffect(() => {
    if (!open) {
      autoDownloadedRef.current = null
      setRefreshError(null)
    }
  }, [open])

  // 进度文案：有 total 时显示 processed/total，否则只显示百分比。
  const detailText =
    totalRows && totalRows > 0
      ? t('exports.progressRows', { processed: processedRows, total: totalRows })
      : t('exports.progressPercent', { percent: progress })

  const progressStatus = isFailed || isCancelled ? 'exception' : isSucceeded ? 'success' : 'active'

  // 签名 URL 过期：重查 job 拿新 result_url 后手动下载。
  const refreshAndDownload = async (): Promise<void> => {
    if (!jobUuid) return
    setRefreshing(true)
    setRefreshError(null)
    try {
      const job = await http.get<ExportJob>(`/exports/${jobUuid}`)
      if (job.result_url) {
        triggerDownload(job.result_url)
      } else {
        setRefreshError(t('exports.noResultUrl'))
      }
    } catch (err) {
      setRefreshError(err instanceof ApiError ? err.message : t('exports.refreshFailed'))
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <Modal
      open={open}
      title={t('exports.progressTitle')}
      onCancel={onClose}
      maskClosable={false}
      footer={
        <Space>
          {isSucceeded && resultUrl ? (
            <Button
              type="primary"
              icon={<DownloadOutlined />}
              loading={refreshing}
              onClick={() => triggerDownload(resultUrl)}
              data-testid="export-download-button"
            >
              {t('exports.download')}
            </Button>
          ) : null}
          {isSucceeded ? (
            <Button loading={refreshing} onClick={() => void refreshAndDownload()}>
              {t('exports.refreshUrl')}
            </Button>
          ) : null}
          <Button onClick={onClose}>{isTerminal ? t('common.ok') : t('exports.cancel')}</Button>
        </Space>
      }
    >
      <div data-testid="export-progress" data-status={status ?? 'PENDING'}>
        <Progress
          percent={progress}
          status={progressStatus}
          aria-label={t('exports.progressTitle')}
        />
        <Space direction="vertical" size="small" style={{ width: '100%' }}>
          <Text type="secondary" className="tabular">
            {detailText}
          </Text>
          {polling ? (
            <Text type="warning" data-testid="export-polling">
              {t('exports.pollingFallback')}
            </Text>
          ) : null}
          {isFailed ? (
            <Alert
              type="error"
              showIcon
              message={t('exports.failedTitle')}
              description={error ?? t('exports.failedDefault')}
              data-testid="export-error"
            />
          ) : null}
          {isCancelled ? <Alert type="info" showIcon message={t('exports.cancelled')} /> : null}
          {isSucceeded ? <Alert type="success" showIcon message={t('exports.succeeded')} /> : null}
          {refreshError ? <Alert type="error" showIcon message={refreshError} /> : null}
        </Space>
      </div>
    </Modal>
  )
}
