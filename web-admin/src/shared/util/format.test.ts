import { describe, expect, it } from 'vitest'
import { formatDuration, formatMileageKm } from './format'

describe('formatDuration', () => {
  it('returns 0s for zero, negative and non-finite input', () => {
    expect(formatDuration(0)).toBe('0s')
    expect(formatDuration(-10)).toBe('0s')
    expect(formatDuration(Number.NaN)).toBe('0s')
    expect(formatDuration(Number.POSITIVE_INFINITY)).toBe('0s')
  })

  it('renders seconds only under one minute', () => {
    expect(formatDuration(5)).toBe('5s')
    expect(formatDuration(59)).toBe('59s')
  })

  it('renders minutes and seconds under one hour', () => {
    expect(formatDuration(60)).toBe('1m 0s')
    expect(formatDuration(125)).toBe('2m 5s')
  })

  it('renders hours, minutes and seconds at or above one hour', () => {
    expect(formatDuration(3600)).toBe('1h 0m 0s')
    expect(formatDuration(3725)).toBe('1h 2m 5s')
    expect(formatDuration(7384)).toBe('2h 3m 4s')
  })

  it('floors fractional seconds', () => {
    expect(formatDuration(90.9)).toBe('1m 30s')
  })
})

describe('formatMileageKm', () => {
  it('returns 0.00 km for zero, negative and non-finite input', () => {
    expect(formatMileageKm(0)).toBe('0.00 km')
    expect(formatMileageKm(-100)).toBe('0.00 km')
    expect(formatMileageKm(Number.NaN)).toBe('0.00 km')
  })

  it('converts meters to km with two decimals (no geo math)', () => {
    expect(formatMileageKm(1000)).toBe('1.00 km')
    expect(formatMileageKm(1234)).toBe('1.23 km')
    expect(formatMileageKm(12567.8)).toBe('12.57 km')
    expect(formatMileageKm(500)).toBe('0.50 km')
  })
})
