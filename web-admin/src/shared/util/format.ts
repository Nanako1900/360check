/**
 * 巡查度量格式化（§P3 强约束）—— 纯函数，无副作用、无 I/O，便于单测。
 * 单位规则在此集中：里程已是米（后端 ST_Length(::geography)），时长是秒。
 */

/**
 * 时长格式化：`duration_seconds` → 人类可读「Xh Ym Zs」。
 * - 0 或负数 → 「0s」。
 * - 不足 1 分钟只显示秒；不足 1 小时只显示分+秒；满 1 小时显示时+分+秒。
 * - 省略为 0 的高位单位，但保留中间为 0 的单位（如 1h 0m 5s）。
 */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s'
  const total = Math.floor(seconds)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const parts: string[] = []
  if (h > 0) {
    parts.push(`${h}h`, `${m}m`, `${s}s`)
  } else if (m > 0) {
    parts.push(`${m}m`, `${s}s`)
  } else {
    parts.push(`${s}s`)
  }
  return parts.join(' ')
}

/**
 * 里程格式化：`mileage_meters`（已是米）→ km 文案，保留 2 位小数。
 * **不做任何投影/地理换算**（后端已用 geography 给出米）。
 * 非有限值或负数 → 「0.00 km」。
 */
export function formatMileageKm(meters: number): string {
  if (!Number.isFinite(meters) || meters <= 0) return '0.00 km'
  return `${(meters / 1000).toFixed(2)} km`
}
