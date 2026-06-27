import { useAuthStore } from './authStore'

/**
 * 会话事件总线：拦截器在 React 渲染树之外触发强制登出，由 App 顶层订阅并导航。
 * 避免在模块层硬编码 window.location 跳转（保留 SPA 路由与 from 回跳能力）。
 *
 * 跨标签页（FE-M1）：通过 BroadcastChannel 广播强制登出，其它标签页收到后清本地
 * 会话并同样导航到登录页，避免「A 标签登出、B 标签仍停留在半登录态」。
 */
export type ForceLogoutReason = 'UNAUTHENTICATED' | 'REFRESH_FAILED' | 'MANUAL'

type Listener = (reason: ForceLogoutReason) => void

const listeners = new Set<Listener>()

const CHANNEL_NAME = 'c5-session'
const channel = typeof BroadcastChannel !== 'undefined' ? new BroadcastChannel(CHANNEL_NAME) : null

function notify(reason: ForceLogoutReason): void {
  for (const listener of listeners) listener(reason)
}

if (channel) {
  channel.onmessage = (event: MessageEvent) => {
    const data = event.data as { type?: string; reason?: ForceLogoutReason } | null
    if (data?.type !== 'force-logout') return
    // Another tab forced logout: clear THIS tab's in-memory session, then run the
    // local listeners (App navigates to /login). Do NOT re-broadcast — avoids a loop.
    useAuthStore.getState().reset()
    notify(data.reason ?? 'UNAUTHENTICATED')
  }
}

export const sessionBus = {
  onForceLogout(listener: Listener): () => void {
    listeners.add(listener)
    return () => {
      listeners.delete(listener)
    }
  },
  emitForceLogout(reason: ForceLogoutReason): void {
    notify(reason)
    channel?.postMessage({ type: 'force-logout', reason })
  },
}
