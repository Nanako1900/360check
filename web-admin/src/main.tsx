import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { App as AntdApp, ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import '@ant-design/v5-patch-for-react-19'
import { App } from './App'
import { env } from './env'
import { createQueryClient } from '@/shared/api/queryClient'
import { appTheme } from '@/styles/theme'
import '@/shared/i18n'
// Self-hosted Inter (FE-H3): bundled, not fetched from fonts.googleapis.com —
// unreachable behind the GFW in mainland China. Only the weights in use.
import '@fontsource/inter/400.css'
import '@fontsource/inter/500.css'
import '@fontsource/inter/600.css'
import '@fontsource/inter/700.css'
import '@/styles/global.css'

/** 后端未就绪时按 openapi.yaml mock（VITE_ENABLE_MSW=true）。 */
async function enableMocking(): Promise<void> {
  if (!env.VITE_ENABLE_MSW) return
  const { worker } = await import('./mocks/browser')
  await worker.start({ onUnhandledRequest: 'bypass' })
}

const queryClient = createQueryClient()
const rootEl = document.getElementById('root')
if (!rootEl) throw new Error('未找到挂载节点 #root')

void enableMocking().then(() => {
  createRoot(rootEl).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <ConfigProvider locale={zhCN} theme={appTheme}>
          <AntdApp>
            <BrowserRouter>
              <App />
            </BrowserRouter>
          </AntdApp>
        </ConfigProvider>
      </QueryClientProvider>
    </StrictMode>,
  )
})
