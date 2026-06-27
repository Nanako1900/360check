/**
 * ECharts option 构造（§P7 / §10.4：图表纳入设计系统）—— **纯函数、无 React、无 echarts import**，便于单测。
 *
 * 输入是后端聚合好的 CountBucket[]（D2，前端不做大数据聚合，仅渲染）。扇区/柱条的 label 与 color
 * 取自 dict_item（含退役项；retired 仍计入历史统计，颜色来自 bucket.color）。无颜色时回退到
 * 令牌调色板。网格线弱化、主色取自品牌主色（§10.4：同一调色板/字体/弱网格线）。
 *
 * 这里返回宽松结构的 option 对象（避免在主包静态 import echarts 的重类型）；运行期由 EChart 透传给
 * echarts.setOption。option 的形状以 ECharts pie/bar series 契约为准。
 */
import type { CountBucket } from '@/shared/api/types'
import { BRAND_PRIMARY } from '@/styles/theme'

/** option 为序列化友好的 JSON 结构；用 unknown 值避免引入 echarts 重类型，同时不退化为 any。 */
export type EChartOption = Record<string, unknown>

/**
 * 设计系统调色板（§10.4）：bucket.color 缺省时按序回退；与品牌主色/语义色系协调。
 * 不以颜色为唯一信息载体 —— label 始终展示（§10.2 无障碍）。
 */
export const CHART_PALETTE: readonly string[] = [
  BRAND_PRIMARY, // #635bff blurple
  '#00b3a6', // teal
  '#f5a623', // amber
  '#9061f9', // violet
  '#3ecf8e', // stripe green
  '#ec4899', // pink
  '#0ea5e9', // sky
  '#f97316', // orange
] as const

/** 弱化网格/轴线颜色（§10.4：弱网格线，Stripe 低对比）。 */
const AXIS_LINE = 'rgba(26, 31, 54, 0.10)'
const SPLIT_LINE = 'rgba(26, 31, 54, 0.05)'
const TEXT_MUTED = '#697386'
const TEXT_TITLE = '#1a1f36'

/** 桶颜色：优先 dict_item.color；缺省按索引回退到令牌调色板。 */
function bucketColor(bucket: CountBucket, index: number): string {
  const c = bucket.color
  if (typeof c === 'string' && c.trim() !== '') return c
  return CHART_PALETTE[index % CHART_PALETTE.length]
}

/**
 * 饼图 option：扇区 = 各计数桶；label/color 取自桶（含退役项）。
 * 中性主题、外侧标签带数值，便于「问题类型 / 状态分布」。
 */
export function pieOption(buckets: readonly CountBucket[], title: string): EChartOption {
  const data = buckets.map((b, i) => ({
    name: b.label,
    value: b.count,
    itemStyle: { color: bucketColor(b, i) },
  }))
  return {
    title: {
      text: title,
      left: 'center',
      textStyle: { fontSize: 14, fontWeight: 600, color: TEXT_TITLE },
    },
    tooltip: { trigger: 'item', confine: true },
    legend: {
      type: 'scroll',
      bottom: 0,
      textStyle: { color: TEXT_MUTED },
    },
    series: [
      {
        type: 'pie',
        radius: ['38%', '64%'],
        center: ['50%', '46%'],
        avoidLabelOverlap: true,
        itemStyle: { borderColor: '#fff', borderWidth: 2 },
        label: { show: true, formatter: '{b}: {c}', color: TEXT_MUTED },
        labelLine: { length: 8, length2: 8 },
        data,
      },
    ],
  }
}

/**
 * 柱状 option：x 轴 = 桶 label，y 轴 = count；用于「按人员 / 按项目分布」。
 * 单色主色或按桶色（若桶带 dict 颜色，如状态分布也可走柱状）。弱网格线、轴标签倾斜避免重叠。
 */
export function barOption(buckets: readonly CountBucket[], title: string): EChartOption {
  const categories = buckets.map((b) => b.label)
  const data = buckets.map((b, i) => ({
    value: b.count,
    itemStyle: { color: bucketColor(b, i) },
  }))
  return {
    title: {
      text: title,
      left: 'center',
      textStyle: { fontSize: 14, fontWeight: 600, color: TEXT_TITLE },
    },
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' }, confine: true },
    grid: { left: 12, right: 16, bottom: 24, top: 48, containLabel: true },
    xAxis: {
      type: 'category',
      data: categories,
      axisLine: { lineStyle: { color: AXIS_LINE } },
      axisTick: { show: false },
      axisLabel: { color: TEXT_MUTED, interval: 0, rotate: categories.length > 4 ? 30 : 0 },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLine: { show: false },
      axisLabel: { color: TEXT_MUTED },
      splitLine: { lineStyle: { color: SPLIT_LINE } },
    },
    series: [
      {
        type: 'bar',
        barMaxWidth: 36,
        itemStyle: { borderRadius: [4, 4, 0, 0] },
        data,
      },
    ],
  }
}
