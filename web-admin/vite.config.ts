import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// 注意：重依赖（腾讯地图 GL / PSV / ECharts）按路由动态 import()，不进首屏主包（见 §10）。
// manualChunks 仅拆分稳定 vendor，便于长缓存。
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    target: 'es2022',
    sourcemap: true,
    rollupOptions: {
      output: {
        manualChunks: {
          'react-vendor': ['react', 'react-dom', 'react-router-dom'],
          'antd-vendor': ['antd', '@ant-design/pro-components', '@ant-design/icons'],
          'query-vendor': ['@tanstack/react-query', 'axios', 'zustand'],
        },
      },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    // E2E（tests/e2e/**）由 Playwright 运行，vitest 仅收集 src 下单元/集成测试
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    css: false,
    // antd 弹窗/表格类组件测试在 --coverage 插桩下 CPU 争用会变慢；放宽超时避免负载型假超时
    // （不改变任何断言，仅给重渲染留出余量）。
    testTimeout: 20_000,
    clearMocks: true,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/**/*.test.{ts,tsx}',
        'src/**/*.d.ts',
        'src/shared/api/generated/**',
        'src/mocks/**',
        'src/test/**',
        'src/main.tsx',
        'src/App.tsx',
        'src/routes/index.tsx',
        'src/layouts/**',
        'src/shared/i18n/**',
        'src/styles/**',
      ],
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 80,
        statements: 80,
        // 关键路径要求近 100%（§6.4）
        'src/shared/api/http.ts': { lines: 90, functions: 90, branches: 80, statements: 90 },
        'src/shared/api/apiError.ts': { lines: 100, functions: 100, branches: 90, statements: 100 },
        'src/shared/auth/authStore.ts': { lines: 95, functions: 90, branches: 85, statements: 95 },
      },
    },
  },
})
