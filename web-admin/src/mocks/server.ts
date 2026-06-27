import { setupServer } from 'msw/node'
import { handlers } from './handlers'

/** 测试端 MSW（vitest，node）。 */
export const server = setupServer(...handlers)
