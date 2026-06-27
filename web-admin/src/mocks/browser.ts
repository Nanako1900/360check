import { setupWorker } from 'msw/browser'
import { handlers } from './handlers'

/** 浏览器端 MSW（dev：VITE_ENABLE_MSW=true 时在 main.tsx 启动）。 */
export const worker = setupWorker(...handlers)
