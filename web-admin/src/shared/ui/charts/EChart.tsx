/**
 * EChart（§P7 / §10.1）：echarts 6 的薄 React 封装。
 *
 * 重依赖处理（对齐 PanoramaViewer / useTencentMap 思路）：
 * - `echarts` 体积大（含 zrender），**绝不进主包** —— 在 effect 内用动态 `import('echarts')` 懒加载；
 *   构建时被拆为独立 chunk，仅进入统计路由时加载。`echarts-for-react` 会 eager import echarts，故此处
 *   不用它，直接 init/setOption/dispose 控制更精简、更可测。
 * - 测试不加载真实 echarts：通过 `echartsModule` 注入 fake（最小 init 工厂），或对动态 import 做 vi.mock。
 *
 * 生命周期：挂载时 init → setOption；option 变化时 setOption；容器尺寸变化经 ResizeObserver → resize；
 * **卸载时 dispose()**（释放 canvas/zrender，防泄漏）。尊重 prefers-reduced-motion（§10.2）：禁用入场动画。
 */
import { useEffect, useRef, useState } from 'react'
import { Spin } from 'antd'
import { useTranslation } from 'react-i18next'
import type { EChartOption } from './options'

/** echarts 实例最小化结构子集（仅本组件用到的方法），避免在主包静态 import 其类型。 */
export interface EChartsInstanceLike {
  setOption(option: EChartOption, opts?: { notMerge?: boolean }): void
  resize(): void
  dispose(): void
}

/** echarts 模块最小化子集：仅需 init(container) → 实例。 */
export interface EChartsModuleLike {
  init(container: HTMLElement): EChartsInstanceLike
}

interface EChartProps {
  /** 由 options.ts 的纯函数构造的 option。 */
  option: EChartOption
  /** 固定高度（CLS）：默认 320px。 */
  height?: number | string
  /** 无障碍区域标签。 */
  ariaLabel?: string
  /** 仅供测试注入的 echarts 模块；生产为 undefined → 走动态 import('echarts')。 */
  echartsModule?: EChartsModuleLike
}

type ChartStatus = 'loading' | 'ready' | 'error'

/** 默认动态加载 echarts（按需进独立 chunk，不入主包）。 */
async function loadEcharts(): Promise<EChartsModuleLike> {
  const mod = await import('echarts')
  return mod as unknown as EChartsModuleLike
}

/** 是否启用动画：尊重 prefers-reduced-motion（§10.2）。SSR/无 matchMedia 时默认启用。 */
function animationEnabled(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return true
  return !window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

export function EChart({ option, height = 320, ariaLabel, echartsModule }: EChartProps) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement | null>(null)
  const chartRef = useRef<EChartsInstanceLike | null>(null)
  const [status, setStatus] = useState<ChartStatus>('loading')

  // option 走 ref，避免每次 option 变化重建实例（只 setOption）。
  const optionRef = useRef<EChartOption>(option)
  optionRef.current = option

  // 初始化：挂载时 init + 首个 setOption + ResizeObserver；卸载时 dispose（核心防泄漏）。
  useEffect(() => {
    let cancelled = false
    let observer: ResizeObserver | null = null
    setStatus('loading')

    async function init(): Promise<void> {
      try {
        const mod = echartsModule ?? (await loadEcharts())
        if (cancelled) return
        const container = containerRef.current
        if (!container) {
          setStatus('error')
          return
        }
        const chart = mod.init(container)
        if (cancelled) {
          chart.dispose()
          return
        }
        chartRef.current = chart
        chart.setOption(
          { animation: animationEnabled(), ...optionRef.current },
          { notMerge: true },
        )
        setStatus('ready')

        if (typeof ResizeObserver !== 'undefined') {
          observer = new ResizeObserver(() => {
            chartRef.current?.resize()
          })
          observer.observe(container)
        }
      } catch {
        if (!cancelled) setStatus('error')
      }
    }

    void init()

    return () => {
      cancelled = true
      if (observer) observer.disconnect()
      if (chartRef.current) {
        chartRef.current.dispose()
        chartRef.current = null
      }
    }
    // echartsModule 注入仅在测试时变化；option 更新走下方独立 effect（optionRef/setStatus 稳定）。
  }, [echartsModule])

  // option 变化 → 仅 setOption（不重建实例，避免闪烁/重分配 canvas）。
  useEffect(() => {
    if (chartRef.current) {
      chartRef.current.setOption(
        { animation: animationEnabled(), ...option },
        { notMerge: true },
      )
    }
  }, [option])

  const resolvedHeight = typeof height === 'number' ? `${height}px` : height

  return (
    <div
      className="echart-shell"
      style={{ position: 'relative', width: '100%', height: resolvedHeight }}
      role="img"
      aria-label={ariaLabel ?? t('stats.chartRegion')}
    >
      <div ref={containerRef} className="echart-canvas" style={{ width: '100%', height: '100%' }} />

      {status === 'loading' ? (
        <div className="echart-overlay" aria-live="polite">
          <Spin />
          <span>{t('stats.chartLoading')}</span>
        </div>
      ) : null}

      {status === 'error' ? (
        <div className="echart-overlay" role="alert">
          <span>{t('stats.loadFailedTitle')}</span>
        </div>
      ) : null}
    </div>
  )
}
