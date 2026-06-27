import { describe, expect, it } from 'vitest'
import type { MediaAsset } from '@/shared/api/types'
import { isRenderable, pickThumbUrl, pickWebUrl } from './mediaTier'

const NOW = '2026-06-26T00:00:00Z'

function makeMedia(overrides: Partial<MediaAsset>): MediaAsset {
  return {
    id: 1,
    client_uuid: 'cccccccc-0000-0000-0000-000000000001',
    owner_type: 'problem',
    owner_id: 5001,
    tier: 'web',
    cos_bucket: 'b',
    cos_key: 'k',
    cos_region: 'ap-shanghai',
    capture_state: 'CONFIRMED',
    verified_at: NOW,
    signed_url: 'https://cdn.example.com/web/1.jpg?sign=x',
    created_at: NOW,
    updated_at: NOW,
    ...overrides,
  }
}

describe('mediaTier.isRenderable', () => {
  it('is true for a CONFIRMED media with a signed_url', () => {
    expect(isRenderable(makeMedia({}))).toBe(true)
  })

  it('is false when verified_at is null (unconfirmed)', () => {
    expect(isRenderable(makeMedia({ verified_at: null }))).toBe(false)
  })

  it('is false when verified_at is undefined', () => {
    expect(isRenderable(makeMedia({ verified_at: undefined }))).toBe(false)
  })

  it('is false when signed_url is missing or empty', () => {
    expect(isRenderable(makeMedia({ signed_url: null }))).toBe(false)
    expect(isRenderable(makeMedia({ signed_url: '' }))).toBe(false)
  })

  it('is false for null/undefined media', () => {
    expect(isRenderable(null)).toBe(false)
    expect(isRenderable(undefined)).toBe(false)
  })
})

describe('mediaTier.pickWebUrl / pickThumbUrl', () => {
  it('pickWebUrl returns the signed_url for a renderable web tier', () => {
    const m = makeMedia({ tier: 'web' })
    expect(pickWebUrl(m)).toBe(m.signed_url)
  })

  it('pickWebUrl returns null for a non-web tier (no cross-tier inference)', () => {
    expect(pickWebUrl(makeMedia({ tier: 'thumb' }))).toBeNull()
    expect(pickWebUrl(makeMedia({ tier: 'original' }))).toBeNull()
  })

  it('pickWebUrl returns null for an unconfirmed web media', () => {
    expect(pickWebUrl(makeMedia({ tier: 'web', verified_at: null }))).toBeNull()
  })

  it('pickThumbUrl returns the signed_url only for a renderable thumb tier', () => {
    const m = makeMedia({ tier: 'thumb', signed_url: 'https://cdn.example.com/thumb/1.jpg?sign=y' })
    expect(pickThumbUrl(m)).toBe(m.signed_url)
    expect(pickThumbUrl(makeMedia({ tier: 'web' }))).toBeNull()
  })
})
