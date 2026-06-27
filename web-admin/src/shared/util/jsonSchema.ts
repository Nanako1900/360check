import { z } from 'zod'

/**
 * jsonb 边界校验工具（§7 永不信任手填 JSON）。
 * 解析 + zod 校验合一：先 JSON.parse（捕获语法错误），再按 schema 校验结构。
 */
export interface JsonParseSuccess<T> {
  ok: true
  value: T
}
export interface JsonParseFailure {
  ok: false
  /** 面向用户的中文错误（语法错误或首条 zod 校验信息）。 */
  message: string
}
export type JsonParseResult<T> = JsonParseSuccess<T> | JsonParseFailure

/** 把 zod 报错压成一条可读中文（取首个 issue 的 path + message）。 */
export function firstZodMessage(error: z.ZodError): string {
  const issue = error.issues[0]
  if (!issue) return '数据校验未通过'
  const path = issue.path.join('.')
  return path ? `${path}：${issue.message}` : issue.message
}

/** 解析 JSON 文本并按 schema 校验，返回结构化结果（不抛异常）。 */
export function parseJsonWithSchema<T>(text: string, schema: z.ZodType<T>): JsonParseResult<T> {
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    return { ok: false, message: 'JSON 语法错误，请检查格式' }
  }
  const result = schema.safeParse(raw)
  if (!result.success) {
    return { ok: false, message: firstZodMessage(result.error) }
  }
  return { ok: true, value: result.data }
}

/** 校验已解析的对象（用于已是对象的输入，如表单值）。 */
export function validateWithSchema<T>(value: unknown, schema: z.ZodType<T>): JsonParseResult<T> {
  const result = schema.safeParse(value)
  if (!result.success) {
    return { ok: false, message: firstZodMessage(result.error) }
  }
  return { ok: true, value: result.data }
}

/** 稳定缩进序列化（编辑器初值/对比用）。 */
export function stringifyJson(value: unknown): string {
  return JSON.stringify(value ?? {}, null, 2)
}

// —— 领域 schema：jsonb 结构契约（§P1 关键实现要点）——

/** 拍照预设 dict_item.extra：宽度 + 质量(1..100)。 */
export const capturePresetExtraSchema = z.object({
  width: z.number().int().positive(),
  quality: z.number().int().min(1).max(100),
})
export type CapturePresetExtra = z.infer<typeof capturePresetExtraSchema>

/** 导出图片配置 export.image：宽度 + 质量(1..100)。 */
export const exportImageSchema = z.object({
  width: z.number().int().positive(),
  quality: z.number().int().min(1).max(100),
})
export type ExportImageConfig = z.infer<typeof exportImageSchema>

/** 拍照默认 capture.default：宽松对象（至少是对象，键值不强约束）。 */
export const captureDefaultSchema = z.record(z.string(), z.unknown())
export type CaptureDefaultConfig = z.infer<typeof captureDefaultSchema>
