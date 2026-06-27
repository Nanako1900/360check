import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import timezone from 'dayjs/plugin/timezone'

dayjs.extend(utc)
dayjs.extend(timezone)

/** 后端均为 UTC timestamptz；渲染统一 Asia/Shanghai（§P0 / §4 时间约定）。 */
export const APP_TZ = 'Asia/Shanghai'
dayjs.tz.setDefault(APP_TZ)

/** 渲染：UTC ISO/时间戳 → Asia/Shanghai 文案。空值返回 '-'。 */
export function fmt(
  ts: string | number | Date | null | undefined,
  pattern = 'YYYY-MM-DD HH:mm',
): string {
  if (ts === null || ts === undefined || ts === '') return '-'
  const d = dayjs.utc(ts).tz(APP_TZ)
  if (!d.isValid()) return '-'
  return d.format(pattern)
}

/** 渲染日期（不含时间）。 */
export function fmtDate(ts: string | number | Date | null | undefined): string {
  return fmt(ts, 'YYYY-MM-DD')
}

/**
 * 提交：本地（Asia/Shanghai）时间 → UTC ISO8601，用于 from/to 等筛选参数。
 * 接受 Dayjs（如 antd RangePicker 值）或可被 dayjs 解析的输入。
 */
export function toUtcIso(
  local: string | number | Date | dayjs.Dayjs | null | undefined,
): string | undefined {
  if (local === null || local === undefined || local === '') return undefined
  let d: dayjs.Dayjs
  if (dayjs.isDayjs(local)) {
    // 已携带时区/偏移（如 antd RangePicker 的值）
    d = local
  } else if (typeof local === 'string') {
    // 朴素墙钟字符串按应用时区（Asia/Shanghai）解释，不随浏览器时区漂移。
    // 注意：dayjs.tz.setDefault 只影响 dayjs.tz()，不影响裸 dayjs()。
    d = dayjs.tz(local, APP_TZ)
  } else {
    // number(epoch) / Date 为绝对时刻
    d = dayjs(local)
  }
  if (!d.isValid()) return undefined
  return d.utc().toISOString()
}

export { dayjs }
