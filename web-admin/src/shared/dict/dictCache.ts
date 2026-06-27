import type { DictItem } from '@/shared/api/types'

/**
 * 字典缓存条目（§P1）。**必须保留 is_active=false（已退役）项** —— 离线回传的问题可能引用退役类型
 * （dict_version_used 旧版本），渲染时需回退到退役项，绝不显示「未知」。
 */
export interface CachedDict {
  items: DictItem[]
  version: number
  contentHash: string
}

const STORAGE_PREFIX = 'c5-dict:'

const memory = new Map<string, CachedDict>()

function storageKey(code: string): string {
  return `${STORAGE_PREFIX}${code}`
}

/** 读缓存：先内存，回退 sessionStorage（页面刷新保活）。 */
export function getCachedDict(code: string): CachedDict | undefined {
  const inMemory = memory.get(code)
  if (inMemory) return inMemory
  try {
    const raw = sessionStorage.getItem(storageKey(code))
    if (!raw) return undefined
    const parsed = JSON.parse(raw) as CachedDict
    memory.set(code, parsed)
    return parsed
  } catch {
    return undefined
  }
}

/** 写缓存：不可变存入内存 + sessionStorage。 */
export function setCachedDict(code: string, value: CachedDict): void {
  const next: CachedDict = {
    items: [...value.items],
    version: value.version,
    contentHash: value.contentHash,
  }
  memory.set(code, next)
  try {
    sessionStorage.setItem(storageKey(code), JSON.stringify(next))
  } catch {
    // sessionStorage 不可用（隐私模式/超额）时仅用内存缓存
  }
}

/** 在缓存里按 item id 查找字典项（含退役项）。 */
export function findCachedItem(code: string, itemId: number): DictItem | undefined {
  return getCachedDict(code)?.items.find((i) => i.id === itemId)
}

/** 测试/登出时清空。 */
export function clearDictCache(): void {
  for (const key of memory.keys()) {
    try {
      sessionStorage.removeItem(storageKey(key))
    } catch {
      // ignore
    }
  }
  memory.clear()
}
