import { beforeEach, describe, expect, it, vi } from 'vitest'
import { clearDictCache, findCachedItem, getCachedDict, setCachedDict } from './dictCache'
import type { CachedDict } from './dictCache'
import type { DictItem } from '@/shared/api/types'

const items: DictItem[] = [
  {
    id: 1,
    dict_type_id: 10,
    code: 'A',
    label: '甲',
    color: '#111',
    sort_order: 0,
    is_active: true,
  },
  { id: 2, dict_type_id: 10, code: 'B', label: '乙', color: null, sort_order: 1, is_active: false },
]
const dict: CachedDict = { items, version: 3, contentHash: 'h3' }

beforeEach(() => {
  clearDictCache()
})

describe('dictCache', () => {
  it('stores and reads back immutably (new array, not the input ref)', () => {
    setCachedDict('problem_status', dict)
    const got = getCachedDict('problem_status')
    expect(got?.version).toBe(3)
    expect(got?.items).toHaveLength(2)
    expect(got?.items).not.toBe(items)
  })

  it('persists to sessionStorage and rehydrates after memory is cleared', () => {
    setCachedDict('problem_status', dict)
    // 直接读 sessionStorage 证明已落盘
    expect(sessionStorage.getItem('c5-dict:problem_status')).toContain('h3')
  })

  it('findCachedItem returns retired (is_active=false) items too', () => {
    setCachedDict('problem_status', dict)
    expect(findCachedItem('problem_status', 2)?.label).toBe('乙')
    expect(findCachedItem('problem_status', 999)).toBeUndefined()
  })

  it('clearDictCache empties memory and storage', () => {
    setCachedDict('problem_status', dict)
    clearDictCache()
    expect(getCachedDict('problem_status')).toBeUndefined()
    expect(sessionStorage.getItem('c5-dict:problem_status')).toBeNull()
  })

  it('rehydrates from sessionStorage when memory is empty', () => {
    sessionStorage.setItem(
      'c5-dict:fresh',
      JSON.stringify({ items: [], version: 7, contentHash: 'h7' }),
    )
    expect(getCachedDict('fresh')?.version).toBe(7)
  })

  it('returns undefined on a corrupt sessionStorage payload', () => {
    sessionStorage.setItem('c5-dict:bad', '{not json')
    expect(getCachedDict('bad')).toBeUndefined()
  })

  it('does not throw and keeps memory cache when sessionStorage write fails', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementationOnce(() => {
      throw new Error('quota exceeded')
    })
    expect(() => setCachedDict('x', dict)).not.toThrow()
    expect(getCachedDict('x')?.version).toBe(3) // 内存回退仍可用
    spy.mockRestore()
  })
})
