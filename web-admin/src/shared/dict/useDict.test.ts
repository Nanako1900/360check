import { beforeEach, describe, expect, it } from 'vitest'
import { http as msw, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { clearDictCache } from './dictCache'
import { fetchDict } from './useDict'

const BASE = '*/api/v1'

beforeEach(() => {
  clearDictCache()
})

describe('fetchDict ETag/304 flow', () => {
  it('fetches 200 first (incl. retired items), then sends If-None-Match and uses cache on 304', async () => {
    let count200 = 0
    let count304 = 0
    server.use(
      msw.get(`${BASE}/dict/types/:code/items`, ({ request }) => {
        const etag = '"hash-x"'
        if (request.headers.get('If-None-Match') === etag) {
          count304 += 1
          return new HttpResponse(null, { status: 304, headers: { ETag: etag } })
        }
        count200 += 1
        return HttpResponse.json(
          {
            success: true,
            data: {
              type: {
                id: 1,
                code: 'problem_status',
                name: 'x',
                scope: 'problem_status',
                version: 2,
                content_hash: 'hash-x',
                is_active: true,
              },
              version: 2,
              content_hash: 'hash-x',
              items: [
                {
                  id: 101,
                  dict_type_id: 1,
                  code: 'OPEN',
                  label: '待处理',
                  color: '#888',
                  sort_order: 0,
                  is_active: true,
                },
                {
                  id: 199,
                  dict_type_id: 1,
                  code: 'LEGACY',
                  label: '历史',
                  color: '#999',
                  sort_order: 9,
                  is_active: false,
                },
              ],
            },
            error: null,
            meta: null,
          },
          { headers: { ETag: etag } },
        )
      }),
    )

    const first = await fetchDict('problem_status')
    expect(first.items).toHaveLength(2)
    expect(first.contentHash).toBe('hash-x')
    expect(first.items.some((i) => i.is_active === false)).toBe(true) // 退役项保留

    const second = await fetchDict('problem_status')
    expect(second).toEqual(first)
    expect(count200).toBe(1)
    expect(count304).toBe(1)
  })

  it('rejects with NOT_FOUND for an unknown dict type', async () => {
    server.use(
      msw.get(`${BASE}/dict/types/:code/items`, () =>
        HttpResponse.json(
          {
            success: false,
            data: null,
            error: { code: 'NOT_FOUND', message: '无', details: [] },
            meta: null,
          },
          { status: 404 },
        ),
      ),
    )
    await expect(fetchDict('nope')).rejects.toMatchObject({ code: 'NOT_FOUND' })
  })

  it('degrades a 404 to an empty dict when optional (e.g. unconfigured project_field)', async () => {
    let calls = 0
    server.use(
      msw.get(`${BASE}/dict/types/:code/items`, () => {
        calls += 1
        return HttpResponse.json(
          {
            success: false,
            data: null,
            error: { code: 'NOT_FOUND', message: '字典类型不存在', details: [] },
            meta: null,
          },
          { status: 404 },
        )
      }),
    )
    // optional=true → 不抛错，退化为空字典（projects 自定义字段未配置时不应报错弹窗）。
    const result = await fetchDict('project_field', true)
    expect(result).toEqual({ items: [], version: 0, contentHash: '' })
    expect(calls).toBe(1)
  })
})
