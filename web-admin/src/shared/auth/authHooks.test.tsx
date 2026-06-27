import { beforeEach, describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useMe } from './useMe'
import { useLogout } from './useLogout'
import { useChangePassword } from './useChangePassword'
import { useAuthStore } from './authStore'
import { makeTokens, USERS } from '@/mocks/db'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

beforeEach(() => {
  useAuthStore.getState().reset()
})

describe('auth hooks', () => {
  it('useMe fetches /auth/me and writes permission codes into the store', async () => {
    useAuthStore.getState().setTokens(makeTokens(USERS[0]))
    const { result } = renderHook(() => useMe(), { wrapper: makeWrapper() })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(useAuthStore.getState().permissionCodes.length).toBeGreaterThan(0)
  })

  it('useLogout clears the session even though backend returns ok', async () => {
    useAuthStore.getState().setTokens(makeTokens(USERS[0]))
    const { result } = renderHook(() => useLogout(), { wrapper: makeWrapper() })
    await result.current.mutateAsync()
    await waitFor(() => expect(useAuthStore.getState().accessToken).toBeNull())
  })

  it('useChangePassword resolves on a valid old password', async () => {
    const { result } = renderHook(() => useChangePassword(), { wrapper: makeWrapper() })
    await expect(
      result.current.mutateAsync({ old_password: 'admin12345', new_password: 'newpass12' }),
    ).resolves.toBeNull()
  })
})
