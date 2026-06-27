import { describe, expect, it } from 'vitest'
import {
  capturePresetExtraSchema,
  parseJsonWithSchema,
  stringifyJson,
  validateWithSchema,
} from './jsonSchema'

describe('jsonSchema helpers', () => {
  it('parseJsonWithSchema returns ok for valid input', () => {
    const result = parseJsonWithSchema('{"width":4096,"quality":80}', capturePresetExtraSchema)
    expect(result.ok).toBe(true)
    if (result.ok) expect(result.value).toEqual({ width: 4096, quality: 80 })
  })

  it('parseJsonWithSchema reports JSON syntax errors', () => {
    const result = parseJsonWithSchema('not json', capturePresetExtraSchema)
    expect(result).toEqual({ ok: false, message: 'JSON 语法错误，请检查格式' })
  })

  it('parseJsonWithSchema reports the first zod issue with a path', () => {
    const result = parseJsonWithSchema('{"width":10,"quality":500}', capturePresetExtraSchema)
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.message).toMatch(/^quality：/)
  })

  it('validateWithSchema validates an already-parsed object', () => {
    const ok = validateWithSchema({ width: 1, quality: 1 }, capturePresetExtraSchema)
    expect(ok.ok).toBe(true)
    const bad = validateWithSchema({ width: 1 }, capturePresetExtraSchema)
    expect(bad.ok).toBe(false)
  })

  it('stringifyJson handles nullish input', () => {
    expect(stringifyJson(null)).toBe('{}')
    expect(stringifyJson({ a: 1 })).toContain('"a": 1')
  })
})
