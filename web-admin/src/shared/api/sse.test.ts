import { afterEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { advanceExportJob, createExportJob, db, EXPORT_RESULT_URL } from '@/mocks/db'
import { useAuthStore } from '@/shared/auth/authStore'
import {
  isTerminalStatus,
  parseDonePayload,
  parseProgressPayload,
  parseSseBuffer,
  pollExportJob,
  streamExportEvents,
  type FetchLike,
} from './sse'

const BASE = '*/api/v1'

/** 构造一个发出给定字符串帧、随后关闭的 text/event-stream Response。 */
function streamResponse(frames: string[], { ok = true, status = 200 } = {}): Response {
  const encoder = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const frame of frames) controller.enqueue(encoder.encode(frame))
      controller.close()
    },
  })
  return new Response(ok ? body : null, {
    status,
    headers: { 'Content-Type': 'text/event-stream' },
  })
}

describe('parseSseBuffer', () => {
  it('parses complete events and keeps a trailing partial as rest', () => {
    const buf =
      'event: progress\ndata: {"progress":10}\n\nevent: done\ndata: {"status":"SUCCEEDED"}\n\nevent: prog'
    const { events, rest } = parseSseBuffer(buf)
    expect(events).toEqual([
      { event: 'progress', data: '{"progress":10}' },
      { event: 'done', data: '{"status":"SUCCEEDED"}' },
    ])
    expect(rest).toBe('event: prog')
  })

  it('joins multi-line data and defaults the event name to message', () => {
    const { events } = parseSseBuffer('data: a\ndata: b\n\n')
    expect(events[0]).toEqual({ event: 'message', data: 'a\nb' })
  })

  it('ignores comment lines and tolerates CRLF', () => {
    const { events } = parseSseBuffer(': keep-alive\r\nevent: ping\r\ndata: 1\r\n\r\n')
    expect(events).toEqual([{ event: 'ping', data: '1' }])
  })
})

describe('parseProgressPayload / parseDonePayload', () => {
  it('parses a well-formed progress payload', () => {
    expect(
      parseProgressPayload(
        '{"progress":40,"processed_rows":40,"total_rows":100,"status":"RUNNING"}',
      ),
    ).toEqual({ progress: 40, processed_rows: 40, total_rows: 100, status: 'RUNNING' })
  })

  it('defaults missing numeric fields and tolerates null total_rows', () => {
    expect(parseProgressPayload('{"status":"PENDING","total_rows":null}')).toEqual({
      progress: 0,
      processed_rows: 0,
      total_rows: null,
      status: 'PENDING',
    })
  })

  it('returns undefined for invalid JSON or unknown status', () => {
    expect(parseProgressPayload('not-json')).toBeUndefined()
    expect(parseProgressPayload('{"status":"NOPE"}')).toBeUndefined()
    expect(parseProgressPayload('42')).toBeUndefined()
  })

  it('leaves total_rows undefined when the field is absent', () => {
    expect(parseProgressPayload('{"status":"RUNNING","progress":20,"processed_rows":20}')).toEqual({
      progress: 20,
      processed_rows: 20,
      total_rows: undefined,
      status: 'RUNNING',
    })
  })

  it('rejects a non-object done payload (array/number)', () => {
    expect(parseDonePayload('[1,2,3]')).toBeUndefined()
    expect(parseDonePayload('7')).toBeUndefined()
  })

  it('parses a terminal done payload (SUCCEEDED with result_url)', () => {
    expect(parseDonePayload(`{"status":"SUCCEEDED","result_url":"${EXPORT_RESULT_URL}"}`)).toEqual({
      status: 'SUCCEEDED',
      result_url: EXPORT_RESULT_URL,
      error: undefined,
    })
  })

  it('parses a FAILED done payload with error', () => {
    expect(parseDonePayload('{"status":"FAILED","error":"boom"}')).toEqual({
      status: 'FAILED',
      result_url: undefined,
      error: 'boom',
    })
  })

  it('rejects non-terminal or invalid done payloads', () => {
    expect(parseDonePayload('{"status":"RUNNING"}')).toBeUndefined()
    expect(parseDonePayload('nope')).toBeUndefined()
  })
})

describe('isTerminalStatus', () => {
  it('marks SUCCEEDED/FAILED/CANCELLED terminal and PENDING/RUNNING non-terminal', () => {
    expect(isTerminalStatus('SUCCEEDED')).toBe(true)
    expect(isTerminalStatus('FAILED')).toBe(true)
    expect(isTerminalStatus('CANCELLED')).toBe(true)
    expect(isTerminalStatus('PENDING')).toBe(false)
    expect(isTerminalStatus('RUNNING')).toBe(false)
  })
})

describe('streamExportEvents', () => {
  afterEach(() => {
    useAuthStore.getState().reset()
  })

  it('parses progress + done frames and attaches the Bearer header', async () => {
    useAuthStore.setState({ accessToken: 'tok-123' })
    let sentAuth: string | null = null
    const fetchImpl: FetchLike = (_input, init) => {
      const headers = new Headers(init?.headers)
      sentAuth = headers.get('Authorization')
      return Promise.resolve(
        streamResponse([
          'event: progress\ndata: {"progress":50,"processed_rows":50,"total_rows":100,"status":"RUNNING"}\n\n',
          `event: done\ndata: {"status":"SUCCEEDED","result_url":"${EXPORT_RESULT_URL}"}\n\n`,
        ]),
      )
    }

    const progresses: number[] = []
    const done = await new Promise<{ status: string; result_url?: string | null }>(
      (resolve, reject) => {
        streamExportEvents('job-x', {
          fetchImpl,
          onProgress: (p) => progresses.push(p.progress),
          onDone: resolve,
          onError: reject,
        })
      },
    )

    expect(sentAuth).toBe('Bearer tok-123')
    expect(progresses).toEqual([50])
    expect(done.status).toBe('SUCCEEDED')
    expect(done.result_url).toBe(EXPORT_RESULT_URL)
  })

  it('calls onError when the stream closes before a terminal done event', async () => {
    const fetchImpl: FetchLike = () =>
      Promise.resolve(
        streamResponse([
          'event: progress\ndata: {"progress":10,"processed_rows":10,"total_rows":100,"status":"RUNNING"}\n\n',
        ]),
      )

    const err = await new Promise<unknown>((resolve) => {
      streamExportEvents('job-y', { fetchImpl, onError: resolve })
    })
    expect(err).toBeInstanceOf(Error)
  })

  it('calls onError when the response is not ok', async () => {
    const fetchImpl: FetchLike = () =>
      Promise.resolve(streamResponse([], { ok: false, status: 500 }))
    const err = await new Promise<unknown>((resolve) => {
      streamExportEvents('job-z', { fetchImpl, onError: resolve })
    })
    expect(err).toBeInstanceOf(Error)
  })

  it('calls onError when fetch itself rejects', async () => {
    const fetchImpl: FetchLike = () => Promise.reject(new Error('network down'))
    const err = await new Promise<unknown>((resolve) => {
      streamExportEvents('job-r', { fetchImpl, onError: resolve })
    })
    expect((err as Error).message).toBe('network down')
  })

  it('aborts immediately and never fetches when the signal is already aborted', async () => {
    const controller = new AbortController()
    controller.abort()
    const fetchImpl = vi.fn<FetchLike>()
    const onError = vi.fn()
    streamExportEvents('job-pre', { fetchImpl, onError, signal: controller.signal })
    await new Promise((r) => setTimeout(r, 5))
    // controller 已 abort → 内部 fetch 即便发起也会因 aborted 信号短路；onError 不触发。
    expect(onError).not.toHaveBeenCalled()
  })

  it('does not call onError when aborted before settling', async () => {
    const controller = new AbortController()
    const fetchImpl: FetchLike = (_input, init) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () =>
          reject(new DOMException('aborted', 'AbortError')),
        )
      })
    const onError = vi.fn()
    streamExportEvents('job-a', { fetchImpl, onError, signal: controller.signal })
    controller.abort()
    await Promise.resolve()
    await new Promise((r) => setTimeout(r, 5))
    expect(onError).not.toHaveBeenCalled()
  })
})

describe('pollExportJob (SSE → polling fallback)', () => {
  it('polls GET /exports/:uuid every interval until SUCCEEDED', async () => {
    const job = createExportJob('PROBLEM_LIST', { project_id: 1 })
    const updates: number[] = []
    const final = await pollExportJob(job.job_uuid, {
      intervalMs: 5,
      onUpdate: (j) => updates.push(j.progress),
    })
    expect(final?.status).toBe('SUCCEEDED')
    expect(final?.result_url).toBe(EXPORT_RESULT_URL)
    // 进度单调递增到 100。
    expect(updates[updates.length - 1]).toBe(100)
    expect(updates.length).toBeGreaterThan(1)
  })

  it('takes over after the SSE stream errors and reaches SUCCEEDED', async () => {
    const job = createExportJob('INSPECTION_RECORDS', {})

    // 1) SSE 流提前出错（未达终态）→ onError 触发回退。
    const sseError = await new Promise<unknown>((resolve) => {
      streamExportEvents(job.job_uuid, {
        fetchImpl: () => Promise.resolve(streamResponse([], { ok: false, status: 502 })),
        onError: resolve,
      })
    })
    expect(sseError).toBeInstanceOf(Error)

    // 2) 回退轮询同一 job 对象直至 SUCCEEDED。
    const final = await pollExportJob(job.job_uuid, { intervalMs: 5 })
    expect(final?.status).toBe('SUCCEEDED')
    expect(final?.result_url).toBe(EXPORT_RESULT_URL)
  })

  it('reaches a terminal state without an onUpdate callback', async () => {
    const job = createExportJob('PROJECT_STATS', {})
    const final = await pollExportJob(job.job_uuid, { intervalMs: 5 })
    expect(final?.status).toBe('SUCCEEDED')
  })

  it('rejects when the poll request fails', async () => {
    const job = createExportJob('PROJECT_STATS', {})
    server.use(
      http.get(`${BASE}/exports/:jobUuid`, () =>
        HttpResponse.json(
          {
            success: false,
            data: null,
            error: { code: 'INTERNAL', message: 'x', details: [] },
            meta: null,
          },
          { status: 500 },
        ),
      ),
    )
    await expect(pollExportJob(job.job_uuid, { intervalMs: 5 })).rejects.toBeTruthy()
  })

  it('resolves null immediately when the signal is already aborted', async () => {
    const job = createExportJob('PROBLEM_LIST', {})
    const controller = new AbortController()
    controller.abort()
    const result = await pollExportJob(job.job_uuid, { signal: controller.signal })
    expect(result).toBeNull()
  })

  it('resolves null when aborted mid-poll', async () => {
    const job = createExportJob('PROBLEM_LIST', {})
    // 库里只推进一步，保持 RUNNING（未终态）以便中途取消。
    advanceExportJob(job.job_uuid)
    const controller = new AbortController()
    const promise = pollExportJob(job.job_uuid, { intervalMs: 50, signal: controller.signal })
    setTimeout(() => controller.abort(), 10)
    const result = await promise
    expect(result).toBeNull()
    // 任务仍存在于库中（未被破坏）。
    expect(db.exportJobs.has(job.job_uuid)).toBe(true)
  })
})
