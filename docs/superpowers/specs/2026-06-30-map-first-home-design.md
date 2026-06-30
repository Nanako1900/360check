# C5 网页后台「地图主页」重构设计（导航 App 风 · 全屏地图 + 悬浮菜单）

> 状态：设计稿（待实现）｜日期：2026-06-30｜范围：`web-admin` 前端重构（**后端零改动**）
> 关联：[前后端分离部署设计](./2026-06-29-frontend-backend-split-deploy-design.md)、[网页开发文档](../../02-网页开发文档.md)

## 0. 背景与动机

当前后台是「左侧菜单 + 表格」式管理后台，功能完整但对**地理数据**（项目区域、巡查轨迹、问题点位）不直观——地图只藏在「巡查详情 → 查看轨迹地图」里，没有独立入口，用户反馈“不实用”。

本设计把**地图升级为主视图**：打开即全屏地图，功能菜单/筛选/详情卡**悬浮**在地图之上（类似导航/打车 App），项目区域、巡查轨迹、问题点直接标在地图上。

**关键前提（务必周知）**：
- 后端接口已就绪，**纯前端重构**：`GET /projects` 已返回每项目 `area_geom`（WGS84 GeoJSON MultiPolygon，见 `backend/internal/project/repo.go` 的 `ST_AsGeoJSON(area_geom)`）；`GET /problems/map` 返回问题点 GeoJSON（带上限、支持按 `project_id`/`inspection_id` 筛选）；`GET /inspections/:id/trajectory` 返回轨迹。
- **数据依赖**：轨迹/问题点需安卓 APP 上传后才有；项目多边形需在「项目编辑」里设置 `area`。初期地图可能偏空，属正常，不是缺陷。
- **地图前提**：`VITE_MAP_KEY` 必须配置，且前端域名加入腾讯地图 key 白名单。

## 1. 已确认决策

| 项 | 决策 |
|---|---|
| 布局 | **全屏地图 + 悬浮菜单**（导航 App 风） |
| 首屏范围 | **所有项目总览**（画出全部项目区域，点项目再下钻） |
| 落地方式 | **新增地图主页，保留现有所有页面**（增量、低风险、可分阶段） |

待定项见 §11（已给默认取值，可在评审时调整）。

## 2. 总体布局

所有悬浮层叠在全屏地图之上（半透明/毛玻璃）：

```
┌──────────────────────────────────────────────────────────┐
│ ☰ C5      🔍 搜索项目 / 问题…                  👤 admin ▾  │ ← 顶部悬浮条
│ ╭────────╮                                                │
│ │图层     │                                       ╭─────╮ │
│ │☑项目范围│                                       │ ＋  │ │ ← 缩放
│ │☑巡查轨迹│          🗺  全屏地图                  │ －  │ │
│ │☑问题点  │   ▰▰项目多边形  ╱╲轨迹  ●●问题聚合     │ ◎定位│ │
│ ├────────┤                                       ╰─────╯ │
│ │快捷入口 │                                                │
│ │项目/巡查│         ╭──────────────────────────────╮      │
│ │问题/统计│         │ 滨江绿道巡查   状态:进行中     │      │ ← 选中卡
│ │导出/字典│         │ 3 巡查 · 12 问题 · 25.6km     │      │
│ ╰────────╯         │ [查看巡查] [问题列表] [详情▸]  │      │
│  左侧悬浮栏         ╰──────────────────────────────╯      │
└──────────────────────────────────────────────────────────┘
```

- **顶部悬浮条 `MapTopBar`**：☰ 折叠左栏；中间**搜索框**（项目/问题）；右侧用户菜单（改密、登出）。
- **左侧悬浮栏 `MapLauncherRail`**（半透明、可折叠）：
  - 上半 **图层开关 `LayerToggles`**：项目范围 / 巡查轨迹 / 问题点 显隐。
  - 下半 **快捷入口**：链到现有页面（项目 / 巡查 / 问题 / 统计 / 导出 / 字典 / 权限），按 `permissionCodes` 过滤（复用 `routes/menu.tsx` 的 `MENU` + `isMenuItemVisible`）。
- **右下控件**：缩放（＋/－）、定位、回到全部范围（fitBounds）。
- **选中卡 `MapSelectionCard`**（底部居中浮出）：点项目多边形或问题点时显示摘要 + 跳现有详情页。

## 3. 组件结构（复用为主，新增少量）

**复用（已存在，勿重写）**：
- `src/shared/map/tmap.ts`（SDK 加载）、`useTencentMap.ts`（地图实例 + 状态机）
- `src/shared/map/toLatLng.ts`（WGS84→GCJ-02 **唯一**转换边界，绝不外泄 GCJ-02）
- `src/shared/map/TrajectoryLayer.tsx`（轨迹折线）、`ProblemMarkerLayer.tsx`（问题点 + 聚合）
- `src/shared/map/MapInfoWindow.tsx` + `infoWindowContent.ts`（点位信息窗，已做 XSS 转义）
- `src/features/panorama/PanoramaModal.tsx`（全景）、`src/shared/map/map.css`

**新增**：
- `src/shared/map/ProjectAreaLayer.tsx`：渲染项目 `area_geom`（MultiPolygon）为多边形 + 点击事件；无 `area` 的项目不画。
- `src/shared/map/fitBounds.ts`：根据一组 GeoJSON 计算包围盒并 `map.fitBounds`。
- `src/features/map-home/MapHome.tsx`：地图主页外壳（组合地图 + 悬浮层 + 图层状态 + 选中态）。
- 悬浮层组件：`MapTopBar.tsx`、`MapLauncherRail.tsx`、`LayerToggles.tsx`、`MapSelectionCard.tsx`、`MapSearch.tsx`。
- `src/features/map-home/useProjectsGeo.ts`：复用 `useProjects`，分出「有 area」与「未定位」两组。

## 4. 数据与接口（后端不改）

| 用途 | 接口 | 备注 |
|---|---|---|
| 项目区域（首屏） | `GET /projects`（已含 `area_geom`） | 拉一页较大 `page_size`；分「有 area / 未定位」 |
| 项目问题点 | `GET /problems/map?project_id=` | GeoJSON FeatureCollection，带上限（见 `MapLimitDefault/Max`） |
| 巡查列表（下钻） | `GET /inspections?project_id=` | 选某次巡查再取轨迹 |
| 巡查轨迹 | `GET /inspections/:id/trajectory` | 仅「已结束」巡查有 `route_geom` |
| 全景 | `GET /media/:id` | 复用 PanoramaModal |

## 5. 交互流（项目总览 → 下钻）

1. **进入 `/`** → 拉项目，画所有带 `area_geom` 的多边形（可点击），`fitBounds` 到全部；未定位项目列入左栏「未定位」分组（不丢失）。
2. **点项目** → 飞到该项目，加载 `/problems/map?project_id=`（问题点）+ 巡查列表；弹**项目选中卡**（统计 + 入口）。
3. **选一次巡查** → `TrajectoryLayer` 画轨迹 + 该巡查问题点。
4. **点问题点** → `MapInfoWindow` → 看全景（PanoramaModal）/ 问题详情（跳 `/problems?problem_id=`）。
5. **搜索** → 命中项目/问题后飞行定位。

## 6. 状态与降级（必须实现，避免“看起来坏了”）

| 状态 | 表现 |
|---|---|
| 未配 `VITE_MAP_KEY` | 全屏降级提示「地图未配置 key」，左栏快捷入口照常可用 |
| 有项目但都无 area | 居中提示「项目尚未设置地理范围，去项目编辑设置 area」，入口照常 |
| 完全无数据（当前阶段） | 空底图 + 引导文案（正常，等 APP 上传） |
| SDK 加载失败 / 加载中 | 复用 §10.2 优雅降级 / 骨架 |

## 7. 路由与集成

- `/` → **`MapHome`**（新主页）。
- 现有工作台仪表盘移到 `/dashboard`（**保留**，默认从左栏入口进入；见 §11 待定）。
- **现有功能页面全部保留**原路由；左栏快捷入口跳转。
- 权限：图层与入口按现有 `useAuthStore().permissionCodes` 过滤（项目范围=`project:read`、轨迹=`inspection:read`、问题点=`problem:read`）。

## 8. 坐标与安全

- 坐标转换**只走** `toLatLng.ts`（WGS84 ↔ 渲染用 GCJ-02），持久化与回传一律 WGS84，GCJ-02 不外泄（§4.3）。
- 信息窗内容继续走 `infoWindowContent.ts` 的转义（无 `dangerouslySetInnerHTML`）。
- CSP：地图主页不引入新外域；腾讯地图域名已在 `edgeone.json` 的 `script-src`/`img-src`/`connect-src`（`*.map.qq.com`/`*.gtimg.com`）白名单内。

## 9. 分阶段实施计划（增量，每步可单独上线）

- **P0 地图外壳**：`/` 全屏地图 + 顶栏 + 左栏快捷入口 + 缩放/定位 + 无 key/空态降级。（先把“主页是地图”立起来）
- **P1 项目总览图层**：`ProjectAreaLayer` 画所有项目多边形 + `fitBounds` + 点击选中卡 + 飞行定位。
- **P2 下钻**：项目 → 问题点图层 + 巡查选择 → 轨迹；点位 → 全景/详情（复用）。
- **P3 搜索 + 图层开关 + 视觉打磨**（图例、毛玻璃悬浮层）。
- **P4（以后）**：视口范围按需加载、问题热力图、移动端底部抽屉变体（见 §11）。

## 10. 风险与前提

- **数据**：轨迹/问题点依赖安卓 APP 上传；项目多边形依赖在项目编辑里设 `area`。P1 上线初期多数项目可能“未定位”，属正常。
- **地图 key**：`VITE_MAP_KEY` + 域名白名单缺一不可。
- **性能**：项目/问题点用上限 + 聚合（`ProblemMarkerLayer` 已带）；后续可做视口按需加载。
- **工作量**：中等，纯前端；P0+P1 是主体，能较快出可看效果。

## 11. 待定项（已给默认取值，可评审调整）

1. **旧工作台仪表盘归属**：默认移到 `/dashboard`（保留，左栏入口进入）。备选：并入左栏一个浮层面板。
2. **目标端**：默认**桌面优先**（office 后台）；移动/平板的底部抽屉变体留到 P4。

## 12. 验收标准

- `/` 打开即全屏地图，悬浮顶栏 + 左栏 + 缩放/定位齐全；无 key/空数据时有清晰降级文案，其余功能不受影响。
- 有 `area` 的项目全部以多边形呈现并可点击；`fitBounds` 正确包含全部项目。
- 点项目 → 选中卡 + 问题点加载；选巡查 → 轨迹绘制；点问题 → 全景/详情可达。
- 现有页面与路由不受影响；权限过滤生效；坐标转换仅经 `toLatLng`。
- 关键交互有组件测试（复用现有 map 测试模式：fake TMap 注入）。
