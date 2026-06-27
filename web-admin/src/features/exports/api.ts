import { useCallback, useEffect, useRef, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import {
  isTerminalStatus,
  pollExportJob,
  streamExportEvents,
  type ExportDonePayload,
  type ExportProgressPayload,
} from '@/shared/api/sse'
import type { ExportJob, ExportJobCreate, JobStatus } from '@/shared/api/types'

export const exportJobKey = (jobUuid: string | null) => ['export-job', jobUuid ?? ''] as const

/** `POST /exports`（§P8）：入队异步导出任务，返回 PENDING 的 ExportJob。 */
export function useCreateExport() {
  return useMutation<ExportJob, unknown, ExportJobCreate>({
    mutationFn: (body) => http.post<ExportJob>('/exports', body),
  })
}

/** `GET /exports/{job_uuid}`（§P8 轮询门面）：单次查询任务状态。 */
export function useExportJob(jobUuid: string | null, enabled = true) {
  return useQuery<ExportJob>({
    queryKey: exportJobKey(jobUuid),
    queryFn: () => http.get<ExportJob>(`/exports/${jobUuid}`),
    enabled: enabled && !!jobUuid,
  })
}

/** `useExportProgress` 对外暴露的进度状态。 */
export interface ExportProgressState {
  status: JobStatus | null
  progress: number
  processedRows: number
  totalRows: number | null
  resultUrl: string | null
  error: string | null
  /** 当前是否已回退到轮询（SSE 断开后）。 */
  polling: boolean
}

const INITIAL_STATE: ExportProgressState = {
  status: null,
  progress: 0,
  processedRows: 0,
  totalRows: null,
  resultUrl: null,
  error: null,
  polling: false,
}

/**
 * 导出进度 Hook（§P8 强约束）：SSE 优先，断开自动轮询回退，两条路监听同一 export_jobs 对象。
 *
 * - jobUuid 非空即订阅 SSE；流出错/提前关闭（未达终态）→ 自动 `pollExportJob` 每 ~2s 轮询。
 * - 到达终态（SUCCEEDED/FAILED/CANCELLED）即停止；卸载/重置经 AbortController 清理。
 *
 * @param jobUuid 任务句柄（null 表示空闲，不订阅）。
 * @param pollIntervalMs 轮询间隔，默认 2000ms。
 */
export function useExportProgress(
  jobUuid: string | null,
  pollIntervalMs = 2000,
): ExportProgressState {
  const [state, setState] = useState<ExportProgressState>(INITIAL_STATE)
  // 用 ref 跟踪「是否已切换轮询」，避免回退被重复触发。
  const fellBackRef = useRef(false)

  const applyProgress = useCallback((p: ExportProgressPayload): void => {
    setState((prev) => ({
      ...prev,
      status: p.status,
      progress: p.progress,
      processedRows: p.processed_rows,
      totalRows: p.total_rows ?? prev.totalRows,
    }))
  }, [])

  const applyJob = useCallback((job: ExportJob): void => {
    setState((prev) => ({
      ...prev,
      status: job.status,
      progress: job.progress,
      processedRows: job.processed_rows,
      totalRows: job.total_rows ?? prev.totalRows,
      resultUrl: job.result_url ?? prev.resultUrl,
      error: job.error ?? prev.error,
    }))
  }, [])

  const applyDone = useCallback((d: ExportDonePayload): void => {
    setState((prev) => ({
      ...prev,
      status: d.status,
      progress: d.status === 'SUCCEEDED' ? 100 : prev.progress,
      resultUrl: d.result_url ?? prev.resultUrl,
      error: d.error ?? prev.error,
    }))
  }, [])

  useEffect(() => {
    if (!jobUuid) {
      setState(INITIAL_STATE)
      return
    }
    fellBackRef.current = false
    setState({ ...INITIAL_STATE })
    const controller = new AbortController()

    // —— 轮询回退（与 SSE 共享同一 job 对象）——
    const startPolling = (): void => {
      if (fellBackRef.current) return
      fellBackRef.current = true
      setState((prev) => ({ ...prev, polling: true }))
      void pollExportJob(jobUuid, {
        intervalMs: pollIntervalMs,
        signal: controller.signal,
        onUpdate: applyJob,
      }).catch((err: unknown) => {
        if (controller.signal.aborted) return
        setState((prev) => ({
          ...prev,
          status: prev.status ?? 'FAILED',
          error: prev.error ?? (err instanceof Error ? err.message : '导出进度获取失败'),
        }))
      })
    }

    // —— SSE 优先 ——
    const cancelStream = streamExportEvents(jobUuid, {
      signal: controller.signal,
      onProgress: applyProgress,
      onDone: applyDone,
      onError: () => {
        if (controller.signal.aborted) return
        startPolling()
      },
    })

    return () => {
      controller.abort()
      cancelStream()
    }
  }, [jobUuid, pollIntervalMs, applyProgress, applyDone, applyJob])

  return state
}

export { isTerminalStatus }
