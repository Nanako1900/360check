import { lazy, Suspense } from 'react'
import { Spin } from 'antd'
import { Route, Routes } from 'react-router-dom'
import { LoginPage } from '@/features/login/LoginPage'
import { HomePage } from '@/features/home/HomePage'
import { DictConfigPage } from '@/features/dict-config/DictConfigPage'
import { RbacPage } from '@/features/rbac/RbacPage'
import { ProjectListPage } from '@/features/projects/ProjectListPage'
import { ProjectDetailPage } from '@/features/projects/ProjectDetailPage'
import { InspectionListPage } from '@/features/inspections/InspectionListPage'
import { InspectionDetailPage } from '@/features/inspections/InspectionDetailPage'
import { ProblemListPage } from '@/features/problems/ProblemListPage'
import { ExportsPage } from '@/features/exports/ExportsPage'
import { AppLayout } from '@/layouts/AppLayout'
import { RequireAuth, RequirePermission } from '@/shared/auth/guards'
import { NotFound } from '@/shared/ui/NotFound'

// 地图页懒加载：腾讯地图 GL SDK 经 useTencentMap 运行时 <script> 注入，页面本身也仅在进入路由时加载（§10.1）。
const InspectionMapPage = lazy(() =>
  import('@/features/inspections/InspectionMapPage').then((m) => ({
    default: m.InspectionMapPage,
  })),
)

// 全景页懒加载：PSV core（含 three.js）在 PanoramaViewer 内动态 import()，仅进入本路由时加载（§10.1）。
const PanoramaPage = lazy(() =>
  import('@/features/panorama/PanoramaPage').then((m) => ({
    default: m.PanoramaPage,
  })),
)

// 统计页懒加载：ECharts 在 EChart 内动态 import()，仅进入本路由时加载（§10.1，echarts 不进主包）。
const StatsOverviewPage = lazy(() =>
  import('@/features/stats/StatsOverviewPage').then((m) => ({
    default: m.StatsOverviewPage,
  })),
)

function LazyFallback() {
  return (
    <div className="app-page" style={{ display: 'grid', placeItems: 'center', minHeight: 240 }}>
      <Spin />
    </div>
  )
}

/**
 * 路由表（含 meta.permission，§3.2）。受保护区在 RequireAuth 外壳下；
 * 各功能页按权限码守卫，P0 用占位页让动态菜单/守卫即可联调，后续 Phase 替换实现。
 */
export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<HomePage />} />
        <Route
          path="/projects"
          element={
            <RequirePermission code="project:read">
              <ProjectListPage />
            </RequirePermission>
          }
        />
        <Route
          path="/projects/:id"
          element={
            <RequirePermission code="project:read">
              <ProjectDetailPage />
            </RequirePermission>
          }
        />
        <Route
          path="/inspections"
          element={
            <RequirePermission code="inspection:read">
              <InspectionListPage />
            </RequirePermission>
          }
        />
        <Route
          path="/inspections/:id"
          element={
            <RequirePermission code="inspection:read">
              <InspectionDetailPage />
            </RequirePermission>
          }
        />
        <Route
          path="/inspections/:id/map"
          element={
            <RequirePermission code="inspection:read">
              <Suspense fallback={<LazyFallback />}>
                <InspectionMapPage />
              </Suspense>
            </RequirePermission>
          }
        />
        <Route
          path="/problems"
          element={
            <RequirePermission code="problem:read">
              <ProblemListPage />
            </RequirePermission>
          }
        />
        <Route
          path="/panorama"
          element={
            <RequirePermission code="media:read">
              <Suspense fallback={<LazyFallback />}>
                <PanoramaPage />
              </Suspense>
            </RequirePermission>
          }
        />
        <Route
          path="/stats"
          element={
            <RequirePermission code="stats:read">
              <Suspense fallback={<LazyFallback />}>
                <StatsOverviewPage />
              </Suspense>
            </RequirePermission>
          }
        />
        <Route
          path="/exports"
          element={
            <RequirePermission code="export:read">
              <ExportsPage />
            </RequirePermission>
          }
        />
        <Route
          path="/dict"
          element={
            <RequirePermission code={['dict:read', 'config:read']}>
              <DictConfigPage />
            </RequirePermission>
          }
        />
        <Route
          path="/rbac"
          element={
            <RequirePermission code={['user:read', 'role:read', 'permission:read']}>
              <RbacPage />
            </RequirePermission>
          }
        />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  )
}
