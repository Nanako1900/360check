import { describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useMedia } from './api'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
  return { qc, Wrapper }
}

describe('panorama api: useMedia', () => {
  it('reads a CONFIRMED web media with a signed_url', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useMedia(7101), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.tier).toBe('web')
    expect(result.current.data?.verified_at).not.toBeNull()
    expect(result.current.data?.signed_url).toContain('https://')
  })

  it('reads an UNCONFIRMED media (verified_at null, no signed_url)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useMedia(7103), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.verified_at).toBeNull()
    expect(result.current.data?.signed_url).toBeNull()
  })

  it('is disabled when id <= 0 (no fetch)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useMedia(0), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.fetchStatus).toBe('idle'))
    expect(result.current.data).toBeUndefined()
  })

  it('refetch obtains a fresh signed_url (signed URLs are time-limited)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useMedia(7101), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const first = result.current.data?.signed_url
    await result.current.refetch()
    await waitFor(() => expect(result.current.data?.signed_url).not.toBe(first))
  })
})
