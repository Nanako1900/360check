import { z } from 'zod'

/**
 * 边界校验：用 zod 校验 import.meta.env（§3.1 / §7）。缺失或非法即 fail-fast，避免运行期隐性错误。
 */
const envSchema = z.object({
  VITE_API_BASE_URL: z.string().min(1).default('/api/v1'),
  VITE_MAP_KEY: z.string().default(''),
  VITE_CDN_BASE: z.string().default(''),
  VITE_ENABLE_MSW: z
    .string()
    .optional()
    .default('false')
    .transform((v) => v === 'true'),
})

export type Env = z.infer<typeof envSchema>

function parseEnv(): Env {
  const result = envSchema.safeParse(import.meta.env)
  if (!result.success) {
    const issues = result.error.issues
      .map((i) => `${i.path.join('.') || '(root)'}: ${i.message}`)
      .join('; ')
    throw new Error(`[env] 环境变量校验失败：${issues}`)
  }
  return result.data
}

export const env: Env = parseEnv()
