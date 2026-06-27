/**
 * PanoramaViewer（§P5）：用 Photo Sphere Viewer v5 渲染后端给出的 equirectangular JPG。
 *
 * 重依赖处理（对齐 §10.1 / useTencentMap 的思路）：
 * - `@photo-sphere-viewer/core` 体积大（含 three.js），**绝不进主包** —— 在 effect 内用动态
 *   `import('@photo-sphere-viewer/core')` 懒加载；构建时被拆为独立 chunk，仅进入本组件时加载。
 * - 测试不加载真实 WebGL：通过 `viewerFactory` 注入 fake，或对该模块做 vi.mock。
 *
 * 生命周期：挂载 / `panorama` 变化时创建 Viewer；卸载或 url 变化时 `viewer.destroy()`（释放 WebGL
 * 上下文，防泄漏）。`ready` 事件 → 渲染完成；`panorama-error` 事件（签名 URL 过期 / 403）→ 调
 * `onExpired`，由上层重拉新 `signed_url`（§P5 坑：签名 CDN URL 有时效）。
 *
 * adapter 默认即 equirectangular（D5：2:1 等距柱状即可渲染球体，不依赖 GPano），无需显式传入。
 */
import { useEffect, useRef, useState } from 'react'
import { Spin } from 'antd'
import { useTranslation } from 'react-i18next'

/** PSV `Viewer` 的最小化结构子集（避免在主包静态 import 其类型）。 */
export interface PanoViewerLike {
  addEventListener(type: string, cb: (e: unknown) => void): void
  destroy(): void
}

/** PSV `Viewer` 构造器子集签名（仅本组件使用的字段）。 */
export interface PanoViewerConfig {
  container: HTMLElement
  panorama: string
  navbar?: boolean | string
  loadingTxt?: string
}

export type PanoViewerFactory = (config: PanoViewerConfig) => PanoViewerLike

type PanoStatus = 'loading' | 'ready' | 'error'

interface PanoramaViewerProps {
  /** `GET /media/{id}` 返回的 `tier=web` 签名 CDN URL。 */
  panorama: string
  /** 固定高度（CLS）：默认 `min(72vh, 720px)`。 */
  height?: string
  /** 加载失败（签名 URL 过期 / 网络）回调，上层据此重拉新 signed_url。 */
  onExpired?: () => void
  /**
   * 仅供测试注入的 Viewer 工厂；生产环境为 undefined → 走动态 import('@photo-sphere-viewer/core')。
   */
  viewerFactory?: PanoViewerFactory
}

/** 默认动态加载 PSV core 并返回真实 Viewer（按需进独立 chunk，不入主包）。 */
async function loadPsvFactory(): Promise<PanoViewerFactory> {
  const mod = await import('@photo-sphere-viewer/core')
  return (config: PanoViewerConfig): PanoViewerLike =>
    new mod.Viewer({
      container: config.container,
      panorama: config.panorama,
      navbar: config.navbar,
      loadingTxt: config.loadingTxt,
    }) as unknown as PanoViewerLike
}

export function PanoramaViewer({
  panorama,
  height = 'min(72vh, 720px)',
  onExpired,
  viewerFactory,
}: PanoramaViewerProps) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement | null>(null)
  const viewerRef = useRef<PanoViewerLike | null>(null)
  const [status, setStatus] = useState<PanoStatus>('loading')

  // onExpired 走 ref，避免回调身份变化触发 Viewer 重建（重建会闪烁 + 重新分配 WebGL 上下文）。
  const onExpiredRef = useRef<typeof onExpired>(onExpired)
  onExpiredRef.current = onExpired

  useEffect(() => {
    let cancelled = false
    setStatus('loading')

    async function init(): Promise<void> {
      try {
        const factory = viewerFactory ?? (await loadPsvFactory())
        if (cancelled) return
        const container = containerRef.current
        if (!container) {
          setStatus('error')
          return
        }
        const viewer = factory({
          container,
          panorama,
          navbar: 'zoom move fullscreen',
          loadingTxt: t('pano.loading'),
        })
        if (cancelled) {
          viewer.destroy()
          return
        }
        viewerRef.current = viewer
        viewer.addEventListener('ready', () => {
          if (!cancelled) setStatus('ready')
        })
        viewer.addEventListener('panorama-error', () => {
          if (cancelled) return
          setStatus('error')
          onExpiredRef.current?.()
        })
      } catch {
        if (!cancelled) setStatus('error')
      }
    }

    void init()

    return () => {
      cancelled = true
      if (viewerRef.current) {
        viewerRef.current.destroy()
        viewerRef.current = null
      }
    }
    // t 仅作初始 loading 文案，变化不应重建 Viewer。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [panorama, viewerFactory])

  return (
    <div
      className="pano-shell"
      style={{ position: 'relative', width: '100%', height }}
      role="region"
      aria-label={t('pano.regionLabel')}
    >
      <div
        ref={containerRef}
        className="pano-canvas"
        style={{ width: '100%', height: '100%', background: '#000' }}
      />

      {status === 'loading' ? (
        <div
          aria-live="polite"
          style={{
            position: 'absolute',
            inset: 0,
            display: 'grid',
            placeItems: 'center',
            background: 'rgba(0,0,0,0.45)',
            color: '#fff',
            gap: 8,
          }}
        >
          <Spin />
          <span>{t('pano.loading')}</span>
        </div>
      ) : null}

      {status === 'error' ? (
        <div
          role="alert"
          style={{
            position: 'absolute',
            inset: 0,
            display: 'grid',
            placeItems: 'center',
            background: 'rgba(0,0,0,0.55)',
            color: '#fff',
            padding: 'var(--space-6, 24px)',
            textAlign: 'center',
          }}
        >
          <span>{t('pano.loadFailed')}</span>
        </div>
      ) : null}
    </div>
  )
}
