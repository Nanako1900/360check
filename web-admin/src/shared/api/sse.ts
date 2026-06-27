/**
 * SSE 客户端（§4.2 同步 / §P8）——手写 fetch + ReadableStream 实现，带轮询回退。
 *
 * 设计要点：
 * - 原生 EventSource 不支持自定义请求头（无法直接带 Bearer），故用 fetch + ReadableStream
 *   手动解析 `text/event-stream`（可带 Authorization 头）。
 * - 必须有 ~2s 轮询回退：腾讯 CLB/CDN 可能缓冲 SSE 导致进度不实时。SSE 流出错/提前关闭
 *   （未收到终态 `done`）即自动降级为轮询 `GET /exports/{job_uuid}`，监听同一 export_jobs 对象。
 * - transport（fetch）可注入，默认全局 fetch，便于单测驱动。
 *
 * §6.4 关键路径：要求接近 100% 覆盖。
 */
import { env } from '@/env'
import { useAuthStore } from '@/shared/auth/authStore'
import { http } from './http'
import type { ExportJob, JobStatus } from './types'

export interface SseEvent {
  event: string
  data: string
}

/**
 * 解析 SSE 文本缓冲为「完整事件 + 剩余未完成缓冲」。
 * 事件以空行（\n\n）分隔；每个事件含 `event:` 与 `data:`（data 可多行）。
 */
export function parseSseBuffer(buffer: string): { events: SseEvent[]; rest: string } {
  const events: SseEvent[] = []
  // 规范化 CRLF → LF，使事件分隔（空行）在 CRLF 流下也能正确切分
  const segments = buffer.replace(/\r\n/g, '\n').split('\n\n')
  // 最后一段可能不完整，留作 rest
  const rest = segments.pop() ?? ''

  for (const segment of segments) {
    if (!segment.trim()) continue
    let eventName = 'message'
    const dataLines: string[] = []
    for (const rawLine of segment.split('\n')) {
      const line = rawLine.replace(/\r$/, '')
      if (line.startsWith(':')) continue // 注释行
      const idx = line.indexOf(':')
      const field = idx === -1 ? line : line.slice(0, idx)
      const value = idx === -1 ? '' : line.slice(idx + 1).replace(/^ /, '')
      if (field === 'event') eventName = value
      else if (field === 'data') dataLines.push(value)
    }
    events.push({ event: eventName, data: dataLines.join('\n') })
  }

  return { events, rest }
}

// —— 帧负载形状（SSE data JSON）——

/** `event: progress` 的 data 负载。 */
export interface ExportProgressPayload {
  progress: number
  processed_rows: number
  total_rows?: number | null
  status: JobStatus
}

/** 终态 `event: done` 的 data 负载。 */
export interface ExportDonePayload {
  status: 'SUCCEEDED' | 'FAILED' | 'CANCELLED'
  result_url?: string | null
  error?: string | null
}

const JOB_STATUSES: readonly JobStatus[] = [
  'PENDING',
  'RUNNING',
  'SUCCEEDED',
  'FAILED',
  'CANCELLED',
]

const TERMINAL_STATUSES: ReadonlySet<JobStatus> = new Set<JobStatus>([
  'SUCCEEDED',
  'FAILED',
  'CANCELLED',
])

/** 是否为终态（不再变化），SSE/轮询据此停止。 */
export function isTerminalStatus(status: JobStatus): boolean {
  return TERMINAL_STATUSES.has(status)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function asJobStatus(value: unknown): JobStatus | undefined {
  return typeof value === 'string' && (JOB_STATUSES as readonly string[]).includes(value)
    ? (value as JobStatus)
    : undefined
}

/** 安全解析 progress 帧的 data JSON（非法/缺字段返回 undefined，不抛）。 */
export function parseProgressPayload(data: string): ExportProgressPayload | undefined {
  let raw: unknown
  try {
    raw = JSON.parse(data)
  } catch {
    return undefined
  }
  if (!isRecord(raw)) return undefined
  const status = asJobStatus(raw.status)
  if (!status) return undefined
  const progress = typeof raw.progress === 'number' ? raw.progress : 0
  const processed = typeof raw.processed_rows === 'number' ? raw.processed_rows : 0
  const total =
    typeof raw.total_rows === 'number' ? raw.total_rows : raw.total_rows === null ? null : undefined
  return { progress, processed_rows: processed, total_rows: total, status }
}

/** 安全解析 done 帧的 data JSON（非法返回 undefined，不抛）。 */
export function parseDonePayload(data: string): ExportDonePayload | undefined {
  let raw: unknown
  try {
    raw = JSON.parse(data)
  } catch {
    return undefined
  }
  if (!isRecord(raw)) return undefined
  const status = asJobStatus(raw.status)
  if (!status || status === 'PENDING' || status === 'RUNNING') return undefined
  const resultUrl = typeof raw.result_url === 'string' ? raw.result_url : undefined
  const error = typeof raw.error === 'string' ? raw.error : undefined
  return { status, result_url: resultUrl, error }
}

// —— 传输层（可注入）——

/** fetch 注入点：默认全局 fetch，单测可替换以驱动 SSE 流。 */
export type FetchLike = (input: string, init?: RequestInit) => Promise<Response>

export interface StreamExportHandlers {
  onProgress?: (payload: ExportProgressPayload) => void
  /** 终态：收到 `event: done` 帧时调用。 */
  onDone?: (payload: ExportDonePayload) => void
  /** 流出错/提前关闭（未达终态）时调用 —— 调用方据此切换轮询回退。 */
  onError?: (error: unknown) => void
  signal?: AbortSignal
  fetchImpl?: FetchLike
}

/** 拼接 SSE 端点绝对/相对路径（与 http baseURL 同源）。 */
function eventsUrl(jobUuid: string): string {
  const base = env.VITE_API_BASE_URL.replace(/\/$/, '')
  return `${base}/exports/${jobUuid}/events`
}

/**
 * 订阅导出进度 SSE（`GET /exports/{job_uuid}/events`，text/event-stream）。
 *
 * - 用 fetch + ReadableStream 读取流，附 `Authorization: Bearer <token>`（原生 EventSource 不支持）。
 * - 逐块 `parseSseBuffer` 分帧；`progress` → onProgress、`done` → onDone（终态）。
 * - 若流在收到终态前出错/关闭（CDN 缓冲、网络抖动），调用 onError —— 调用方应回退轮询。
 *
 * 返回一个取消函数（abort 流）。终态/错误后自身清理。
 */
export function streamExportEvents(jobUuid: string, handlers: StreamExportHandlers): () => void {
  const { onProgress, onDone, onError, signal, fetchImpl } = handlers
  const doFetch: FetchLike = fetchImpl ?? ((input, init) => globalThis.fetch(input, init))
  const controller = new AbortController()

  // 外部 signal 透传到内部 controller，统一取消。
  if (signal) {
    if (signal.aborted) controller.abort()
    else signal.addEventListener('abort', () => controller.abort(), { once: true })
  }

  let settled = false
  const finishDone = (payload: ExportDonePayload): void => {
    if (settled) return
    settled = true
    onDone?.(payload)
  }
  const finishError = (error: unknown): void => {
    if (settled) return
    settled = true
    onError?.(error)
  }

  void (async () => {
    try {
      const token = useAuthStore.getState().accessToken
      const headers: Record<string, string> = { Accept: 'text/event-stream' }
      if (token) headers.Authorization = `Bearer ${token}`

      const response = await doFetch(eventsUrl(jobUuid), {
        method: 'GET',
        headers,
        signal: controller.signal,
      })

      if (!response.ok || !response.body) {
        finishError(new Error(`SSE stream failed with status ${response.status}`))
        return
      }

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      for (;;) {
        const { value, done } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const { events, rest } = parseSseBuffer(buffer)
        buffer = rest
        for (const evt of events) {
          if (evt.event === 'progress') {
            const payload = parseProgressPayload(evt.data)
            if (payload) onProgress?.(payload)
          } else if (evt.event === 'done') {
            const payload = parseDonePayload(evt.data)
            if (payload) {
              finishDone(payload)
              return
            }
          }
        }
        if (settled) return
      }

      // 流自然结束但未收到终态 → 视为断开，回退轮询。
      finishError(new Error('SSE stream closed before a terminal done event'))
    } catch (error) {
      // AbortError（主动取消）不触发回退。
      if (controller.signal.aborted) {
        settled = true
        return
      }
      finishError(error)
    }
  })()

  return () => controller.abort()
}

export interface PollExportHandlers {
  intervalMs?: number
  onUpdate?: (job: ExportJob) => void
  signal?: AbortSignal
}

/**
 * 轮询回退（`GET /exports/{job_uuid}` 每 ~2s），监听与 SSE 同一 export_jobs 对象。
 *
 * - 每次拉取调用 onUpdate；到达终态（SUCCEEDED/FAILED/CANCELLED）即 resolve（停止轮询）。
 * - 支持 AbortSignal 取消（卸载/手动取消）。
 *
 * 返回一个 Promise，resolve 为最终的 ExportJob（终态）；abort 时 resolve null。
 */
export function pollExportJob(
  jobUuid: string,
  handlers: PollExportHandlers = {},
): Promise<ExportJob | null> {
  const { intervalMs = 2000, onUpdate, signal } = handlers

  return new Promise<ExportJob | null>((resolve, reject) => {
    let timer: ReturnType<typeof setTimeout> | null = null
    let stopped = false

    const cleanup = (): void => {
      stopped = true
      if (timer) {
        clearTimeout(timer)
        timer = null
      }
      signal?.removeEventListener('abort', onAbort)
    }

    function onAbort(): void {
      cleanup()
      resolve(null)
    }

    if (signal) {
      if (signal.aborted) {
        resolve(null)
        return
      }
      signal.addEventListener('abort', onAbort, { once: true })
    }

    const tick = async (): Promise<void> => {
      if (stopped) return
      try {
        const job = await http.get<ExportJob>(`/exports/${jobUuid}`, {
          headers: { 'X-Skip-Refresh': '1' },
        })
        if (stopped) return
        onUpdate?.(job)
        if (isTerminalStatus(job.status)) {
          cleanup()
          resolve(job)
          return
        }
        timer = setTimeout(() => void tick(), intervalMs)
      } catch (error) {
        if (stopped) return
        cleanup()
        reject(error)
      }
    }

    void tick()
  })
}
