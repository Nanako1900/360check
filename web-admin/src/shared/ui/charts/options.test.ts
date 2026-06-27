import { describe, expect, it } from 'vitest'
import type { CountBucket } from '@/shared/api/types'
import { barOption, CHART_PALETTE, pieOption } from './options'

// 测试用最小化 option 形状（仅断言用到的字段；运行期由 echarts 消费完整契约）。
interface PieDatum {
  name: string
  value: number
  itemStyle: { color: string }
}
interface PieSeries {
  type: string
  data: PieDatum[]
}
interface BarDatum {
  value: number
  itemStyle: { color: string }
}
interface BarSeries {
  type: string
  data: BarDatum[]
}

function pieSeries(option: Record<string, unknown>): PieSeries {
  return (option.series as PieSeries[])[0]
}
function barSeries(option: Record<string, unknown>): BarSeries {
  return (option.series as BarSeries[])[0]
}

const TYPE_BUCKETS: CountBucket[] = [
  { item_id: 201, label: '裂缝', color: '#fa541c', count: 180 },
  { item_id: 202, label: '坑洼', color: '#d48806', count: 96 },
  // 退役类型：color + label 齐全，统计仍须计入（历史容忍）。
  { item_id: 910, label: '历史类型（已退役）', color: '#999999', count: 64 },
]

describe('pieOption', () => {
  it('maps each bucket to a sector with its dict label and color (including retired)', () => {
    const option = pieOption(TYPE_BUCKETS, '问题类型分布')
    const series = pieSeries(option)
    expect(series.type).toBe('pie')
    expect(series.data).toHaveLength(3)
    expect(series.data.map((d) => d.name)).toEqual(['裂缝', '坑洼', '历史类型（已退役）'])
    expect(series.data.map((d) => d.value)).toEqual([180, 96, 64])
    // 退役桶颜色取自 dict_item.color，不丢失。
    expect(series.data[2].itemStyle.color).toBe('#999999')
  })

  it('falls back to the token palette when a bucket has no color', () => {
    const buckets: CountBucket[] = [
      { inspector_id: 2, label: '李巡查', count: 10 },
      { inspector_id: 3, label: '王巡查', color: null, count: 5 },
    ]
    const series = pieSeries(pieOption(buckets, '人员'))
    expect(series.data[0].itemStyle.color).toBe(CHART_PALETTE[0])
    expect(series.data[1].itemStyle.color).toBe(CHART_PALETTE[1])
  })

  it('puts the title text into the option', () => {
    const option = pieOption(TYPE_BUCKETS, '问题类型分布')
    expect((option.title as { text: string }).text).toBe('问题类型分布')
  })

  it('handles an empty bucket list without throwing', () => {
    const series = pieSeries(pieOption([], '空'))
    expect(series.data).toHaveLength(0)
  })
})

describe('barOption', () => {
  it('builds category axis from labels and value series from counts (incl retired color)', () => {
    const option = barOption(TYPE_BUCKETS, '按项目分布')
    const xAxis = option.xAxis as { type: string; data: string[] }
    expect(xAxis.type).toBe('category')
    expect(xAxis.data).toEqual(['裂缝', '坑洼', '历史类型（已退役）'])

    const series = barSeries(option)
    expect(series.type).toBe('bar')
    expect(series.data.map((d) => d.value)).toEqual([180, 96, 64])
    expect(series.data[2].itemStyle.color).toBe('#999999')
  })

  it('falls back to the token palette for colorless buckets', () => {
    const buckets: CountBucket[] = [{ project_id: 1, label: '项目A', count: 3 }]
    const series = barSeries(barOption(buckets, 'p'))
    expect(series.data[0].itemStyle.color).toBe(CHART_PALETTE[0])
  })

  it('handles an empty bucket list without throwing', () => {
    const option = barOption([], '空')
    expect((option.xAxis as { data: string[] }).data).toHaveLength(0)
    expect(barSeries(option).data).toHaveLength(0)
  })
})
