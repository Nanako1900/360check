import { describe, expect, it } from 'vitest'
import { dayjs, fmt, fmtDate, toUtcIso } from './dayjs'

describe('time helpers', () => {
  it('fmt renders UTC into Asia/Shanghai (+8)', () => {
    expect(fmt('2026-06-26T00:00:00Z')).toBe('2026-06-26 08:00')
  })

  it('fmtDate crosses midnight when +8 pushes to next day', () => {
    expect(fmtDate('2026-06-26T20:00:00Z')).toBe('2026-06-27')
  })

  it('fmt returns "-" for empty / invalid input', () => {
    expect(fmt(null)).toBe('-')
    expect(fmt(undefined)).toBe('-')
    expect(fmt('')).toBe('-')
    expect(fmt('not-a-date')).toBe('-')
  })

  it('toUtcIso converts a local (Asia/Shanghai) Dayjs to UTC ISO', () => {
    const local = dayjs.tz('2026-06-26 08:00', 'Asia/Shanghai')
    expect(toUtcIso(local)).toBe('2026-06-26T00:00:00.000Z')
  })

  it('toUtcIso interprets a naive wall-clock string in Asia/Shanghai (not browser tz)', () => {
    // 不随运行环境时区漂移：08:00 Asia/Shanghai = 00:00Z
    expect(toUtcIso('2026-06-26 08:00')).toBe('2026-06-26T00:00:00.000Z')
  })

  it('toUtcIso treats a Date/epoch as an absolute instant', () => {
    expect(toUtcIso(new Date('2026-06-26T00:00:00Z'))).toBe('2026-06-26T00:00:00.000Z')
  })

  it('toUtcIso returns undefined for empty input', () => {
    expect(toUtcIso(null)).toBeUndefined()
    expect(toUtcIso('')).toBeUndefined()
  })
})
