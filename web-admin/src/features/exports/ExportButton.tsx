import { useState } from 'react'
import { App as AntdApp, Button, Tooltip } from 'antd'
import type { ButtonProps } from 'antd'
import { DownloadOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import { useAuthStore } from '@/shared/auth/authStore'
import type { ExportType } from '@/shared/api/types'
import { useCreateExport } from './api'
import { ExportProgress } from './ExportProgress'

interface ExportButtonProps {
  /** 导出类型（三种之一）。 */
  type: ExportType
  /** 当前页筛选条件（所见即所导）；空值由后端忽略。 */
  params: Record<string, unknown>
  /** 按钮文案，默认「导出 Excel」。 */
  label?: string
  buttonProps?: Omit<ButtonProps, 'onClick' | 'loading' | 'disabled'>
  /** 进度弹层轮询回退间隔（ms），默认 2000；测试可调小。 */
  pollIntervalMs?: number
}

/** 仅保留有意义的筛选键（去除 undefined/null/空串），避免把空值塞进 params。 */
function compactParams(params: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    out[key] = value
  }
  return out
}

/**
 * 导出按钮（§P8）：点击 `POST /exports`（type + 当前筛选 params）→ 打开进度弹层。
 *
 * 前端权限仅 UI 隐藏（`export:create`）；真正鉴权在后端 casbin，仍可能收 FORBIDDEN（§7）。
 * 可跨巡查/问题/统计/项目列表复用。
 */
export function ExportButton({
  type,
  params,
  label,
  buttonProps,
  pollIntervalMs,
}: ExportButtonProps) {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const canCreate = useAuthStore((s) => s.hasPermission('export:create'))
  const createExport = useCreateExport()
  const [jobUuid, setJobUuid] = useState<string | null>(null)
  const [open, setOpen] = useState(false)

  const handleClick = (): void => {
    const cleaned = compactParams(params)
    createExport.mutate(
      { type, params: cleaned },
      {
        onSuccess: (job) => {
          setJobUuid(job.job_uuid)
          setOpen(true)
        },
        onError: (err) => {
          if (err instanceof ApiError && err.code === 'FORBIDDEN') {
            message.error(t('exports.forbidden'))
            return
          }
          message.error(err instanceof ApiError ? err.message : t('exports.createFailed'))
        },
      },
    )
  }

  const handleClose = (): void => {
    setOpen(false)
    setJobUuid(null)
  }

  const button = (
    <Button
      icon={<DownloadOutlined />}
      loading={createExport.isPending}
      disabled={!canCreate}
      onClick={handleClick}
      data-testid={`export-button-${type}`}
      {...buttonProps}
    >
      {label ?? t('exports.button')}
    </Button>
  )

  return (
    <>
      {canCreate ? button : <Tooltip title={t('exports.noPermission')}>{button}</Tooltip>}
      <ExportProgress
        jobUuid={jobUuid}
        open={open}
        onClose={handleClose}
        pollIntervalMs={pollIntervalMs}
      />
    </>
  )
}
