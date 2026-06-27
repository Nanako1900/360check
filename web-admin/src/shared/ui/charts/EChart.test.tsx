import { describe, expect, it, vi } from 'vitest'
import { waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { EChart } from './EChart'
import type { EChartsInstanceLike, EChartsModuleLike } from './EChart'
import type { EChartOption } from './options'

interface SetOptionCall {
  option: EChartOption
}

/** 构造一个最小化 fake echarts 模块：记录 init/setOption/resize/dispose 调用。 */
function makeFakeModule() {
  const setOptionCalls: SetOptionCall[] = []
  const disposeSpy = vi.fn()
  const resizeSpy = vi.fn()
  let initCount = 0

  const instance: EChartsInstanceLike = {
    setOption: (option) => {
      setOptionCalls.push({ option })
    },
    resize: resizeSpy,
    dispose: disposeSpy,
  }
  const module: EChartsModuleLike = {
    init: () => {
      initCount += 1
      return instance
    },
  }
  return {
    module,
    setOptionCalls,
    disposeSpy,
    resizeSpy,
    getInitCount: () => initCount,
  }
}

const OPTION: EChartOption = { series: [{ type: 'pie', data: [] }] }

describe('EChart (injected echarts module, no real echarts load)', () => {
  it('initializes the chart and calls setOption with the given option', async () => {
    const fake = makeFakeModule()
    renderWithProviders(<EChart option={OPTION} echartsModule={fake.module} />)

    await waitFor(() => expect(fake.getInitCount()).toBe(1))
    expect(fake.setOptionCalls.length).toBeGreaterThanOrEqual(1)
    // option 透传（合并 animation 字段）。
    expect(fake.setOptionCalls[0].option.series).toEqual(OPTION.series)
  })

  it('enables animation by default (prefers-reduced-motion: false in test env)', async () => {
    const fake = makeFakeModule()
    renderWithProviders(<EChart option={OPTION} echartsModule={fake.module} />)
    await waitFor(() => expect(fake.setOptionCalls.length).toBeGreaterThanOrEqual(1))
    expect(fake.setOptionCalls[0].option.animation).toBe(true)
  })

  it('disposes the chart instance on unmount (no leak)', async () => {
    const fake = makeFakeModule()
    const { unmount } = renderWithProviders(
      <EChart option={OPTION} echartsModule={fake.module} />,
    )
    await waitFor(() => expect(fake.getInitCount()).toBe(1))

    unmount()
    expect(fake.disposeSpy).toHaveBeenCalledTimes(1)
  })

  it('calls setOption again when the option changes (no re-init)', async () => {
    const fake = makeFakeModule()
    const { rerender } = renderWithProviders(
      <EChart option={OPTION} echartsModule={fake.module} />,
    )
    await waitFor(() => expect(fake.setOptionCalls.length).toBeGreaterThanOrEqual(1))
    const before = fake.setOptionCalls.length

    const nextOption: EChartOption = { series: [{ type: 'bar', data: [{ value: 1 }] }] }
    rerender(<EChart option={nextOption} echartsModule={fake.module} />)

    await waitFor(() => expect(fake.setOptionCalls.length).toBeGreaterThan(before))
    // 不重建实例：init 仍只调用一次。
    expect(fake.getInitCount()).toBe(1)
    expect(fake.setOptionCalls[fake.setOptionCalls.length - 1].option.series).toEqual(
      nextOption.series,
    )
  })
})
