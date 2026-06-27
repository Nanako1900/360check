# C5 网页管理后台（web-admin）

「360 相机巡查标注系统」管理后台。后端模块化单体（Go + Gin + oapi-codegen）的**纯消费者** —— 仅经 `/api/v1/*` REST + SSE 交互，不直连 DB、不持有 SDK、不做坐标持久化。

## 技术栈

React 19 · TypeScript 5.9（strict）· Vite 7 · Ant Design v5 + Pro Components · TanStack Query v5 · Axios · Zustand 5 · React Router 7 · 腾讯地图 JS API GL · Photo Sphere Viewer v5 · ECharts 6 · react-i18next（zh-CN）· MSW · Vitest · Playwright。

> **AntD 版本说明**：开发计划标注「AntD v6 + Pro Components」，但 `@ant-design/pro-components` 当前 peer 仅支持 antd v4/v5。本项目据此采用 **antd v5 + Pro Components**（保留 ProLayout/ProTable/ProForm 架构），待 pro-components 支持 v6 再评估升级。

## 环境要求

- Node `22.x`（见 `.nvmrc`，Vite 7 要求 Node ≥ 20.19 / 22.12）

## 快速开始

```bash
npm install
npm run gen:api      # 由 ../backend/api/openapi.yaml 生成 TS 客户端（勿手改 src/shared/api/generated）
cp .env.example .env.development   # 已内置：VITE_ENABLE_MSW=true，按 openapi.yaml mock
npm run dev          # http://localhost:5173
```

## 脚本

| 命令 | 说明 |
|---|---|
| `npm run dev` | 开发服务器（MSW 可选） |
| `npm run build` | 类型检查 + 生产构建 |
| `npm run typecheck` | `tsc --noEmit`（strict，零 `any`） |
| `npm run lint` | ESLint（flat config） |
| `npm run test` / `test:cov` | Vitest 单元/集成（含覆盖率门禁 ≥80%） |
| `npm run e2e` | Playwright E2E（需 `npx playwright install chromium`） |
| `npm run gen:api` | 重新生成 OpenAPI TS 客户端 |

## 架构（按 feature/domain 组织）

```
src/
├─ shared/
│  ├─ api/      http(axios 信封拆封 + JWT 单飞刷新) · apiError · queryClient · types(派生自 generated) · sse · generated/
│  ├─ auth/     authStore(zustand) · useLogin/useMe/useLogout/useChangePassword · guards · sessionBus
│  ├─ time/     dayjs(UTC↔Asia/Shanghai)
│  ├─ i18n/     zh-CN
│  └─ ui/       ErrorBoundary · Forbidden(403) · NotFound(404) · EmptyState · BuildingPlaceholder
├─ features/    login(LoginPage/ChangePasswordModal) · home · …(后续 Phase)
├─ layouts/     AppLayout(ProLayout 外壳 + 动态菜单 + 用户菜单)
├─ routes/      index(路由表 + 权限守卫) · menu(权限码过滤)
├─ styles/      theme(antd token, Swiss/data-console) · tokens.css · global.css
└─ mocks/       MSW handlers(按 openapi.yaml) · db · browser · server
```

## 关键契约约束

- **信封** `{success,data,error,meta}`：拦截器拆封返回 `T`；列表门面 `getList` 规整 `meta` 为 `{items,total,page,pageSize}`。
- **401 分支按 `error.code`**：`TOKEN_EXPIRED` → 单飞 `refresh` 透明重放；`UNAUTHENTICATED` → 直接登出（注意非 `UNAUTHORIZED`）。
- **错误码** 为冻结闭集（`apiError.ts` 的 `ApiErrorCode`），与生成 `ErrorCode` 有编译期一致性断言。
- **枚举逐字使用冻结值**（如 `task_status=PENDING/IN_PROGRESS/COMPLETED/ARCHIVED`）。
- **坐标**：后端永远 WGS84/4326；GCJ-02 仅在腾讯地图渲染边界转换（P4 `shared/geo/coordTransform`，单一来源 + 单测）。
- **字典退役项**（`is_active=false`）仍需正常渲染（历史容忍，P1）。

## 设计风格 —— Stripe 式现代风格

视觉方向为 **Stripe 式现代风格**（refined SaaS / data-console），纯视觉层、不影响任何操作：
- **品牌色**：blurple `#635bff`（`BRAND_PRIMARY`）；中性骨架用精炼 slate；语义色专用于状态。
- **设计令牌单一来源**：`src/styles/theme.ts`（antd `theme.token` + 组件 token，`cssVar` 输出）与 `src/styles/tokens.css`（`--color-*`/`--shadow-*`/`--radius-*`/`--focus-ring`）；组件 CSS 一律 `var()` 引用，禁硬编码。
- **要素**：Inter 字体（`index.html` 加载，系统字体回退）、8px 圆角、层叠柔和阴影、blurple 焦点环、tabular-nums 数字、浅色侧边导航（靛蓝选中态）、登录页 blurple 渐变 mesh。
- **图表**：`charts/options.ts` 的 `CHART_PALETTE` 与品牌色系协调，弱网格线。
- 改造后 328 单测 + 2 E2E 全绿、`tsc`/`eslint`/`build` 通过——**功能零变更**。

## 进度 —— P0–P8 全部完成 ✅

| Phase | 内容 | 状态 |
|---|---|---|
| **P0** | 工程外壳 + OpenAPI TS 客户端 + Axios 信封/JWT 单飞刷新 + 认证外壳（登录/刷新/登出/改密/`/auth/me` + ProLayout 动态菜单 + 路由守卫 + ErrorBoundary） | ✅ |
| **P1** | 字典/系统配置 CRUD + ETag/304 缓存（`useDict`/`dictCache`）+ `DictTag` 退役容忍 + 拍照预设/配置 jsonb zod 校验 | ✅ |
| **P2** | RBAC：用户/角色/权限树 + casbin p/g-rules；is_system 保护；改自身角色/权限重拉 `/auth/me` | ✅ |
| **P3** | 项目管理（custom_fields 动态表单）+ 任务 + 巡查记录（时长/里程 km/状态筛选；RESTRICT 409） | ✅ |
| **P4** | 腾讯地图 GL：`coordTransform`（唯一 WGS84↔GCJ-02，往返<1e-6）+ 轨迹 MultiPolyline + 问题点 MarkerCluster + InfoWindow | ✅ |
| **P5** | Photo Sphere Viewer 360（equirectangular，懒加载独立 chunk）+ 关联上下文；仅渲染 CONFIRMED；签名 URL 过期重拉 | ✅ |
| **P6** | 问题管理工作流：8 维筛选 + 详情抽屉 + 改分类/状态/备注 + 处理记录追加；**D3：状态变更单 PUT、前端绝不 POST STATUS_CHANGE** | ✅ |
| **P7** | 数据统计 ECharts（懒加载独立 chunk）：指标卡 + 类型/状态/人员/项目分布；后端聚合前端只渲染 | ✅ |
| **P8** | 数据导出三种 Excel（INSPECTION_RECORDS/PROBLEM_LIST/PROJECT_STATS）+ SSE 进度 + ~2s 轮询回退 + 自动下载 | ✅ |

**质量门禁（全绿）**：`tsc` strict 零 `any` · ESLint · **328 单测/集成 + 2 E2E** · 全局覆盖率 **~93.5%**（关键路径 `coordTransform` 100% / `sse` 100%行 / `http`·`apiError`·`authStore` 近 100%） · `vite build` 通过。

> E2E 备注：`auth.spec` 偶发「冷启动竞态」—— dev server 刚启动且并行负载高时首个用例可能假失败；warm 状态下稳定 2/2。CI 建议 `retries: 2`（已配）。

## P5 全景渲染结论

- **GPano 非必需（D5）**：Photo Sphere Viewer v5 的默认 **equirectangular** adapter **不解析也不依赖** GPano XMP；只要图为 **2:1 等距柱状（equirectangular）** 即可作为完整球体渲染。App 端 `ExportUtils.exportImage(ExportMode.PANORAMA)` 产出的 JPG 已是该格式（如 4096×2048），并按跨端约定仍写入 GPano XMP 仅作互操作冗余兜底；Web 既不解析也不依赖它。
- **只渲染已确认媒体**：仅显示 `verified_at` 非空（`capture_state=CONFIRMED`）且携带 `signed_url` 的媒体（`shared/psv/mediaTier.ts:isRenderable`）；未确认媒体显示「照片处理中/未确认」空态，绝不送入 PSV。
- **tier 选择（D4）**：全景球用 `tier=web`（`pickWebUrl`），导航缩略图用 `tier=thumb`（`pickThumbUrl`），`original` Web 不渲染。Web 不派生任何 tier、不在客户端拼 URL。
- **签名 URL 时效**：`GET /media/{id}` 返回的签名 CDN URL 有时效；PSV `panorama-error` 事件触发 `onExpired` → `PanoramaModal` 调 `useMedia` 的 `refetch()` 取新 `signed_url`，签名不落持久存储。
- **懒加载 / 资源释放**：`@photo-sphere-viewer/core`（含 three.js，~610kB）经 `PanoramaViewer` 内动态 `import()` 拆为独立 chunk，**不进主包**；卸载 / URL 变更时 `viewer.destroy()` 释放 WebGL 上下文防泄漏。
- **真机 X3 导出验收为人工步骤（不可自动化）**：CI 仅以 mock PSV 验证「以正确 URL 构造 Viewer / 卸载调 destroy / 过期重拉」等逻辑路径（不加载真实 WebGL）。用真实 X3 `ExportMode.PANORAMA` 导出图确认球面无撕裂、地平线水平、可拖拽/缩放，须在浏览器中**手动验收**（见开发文档 §P5 验证任务与 §13 上线清单）。

## 部署 —— Coolify / Docker Compose（与后端同源）

部署资产在 `deploy/`：`Dockerfile`（多阶段，nginx-unprivileged 非 root :8080）、`nginx.conf`（静态托管 + `/api` 同源反代 + CSP/安全头）、`compose.web.yaml`（`web` 服务块，交后端团队合并进【根】`docker-compose.yaml`）。详见开发文档 §8.5。

**拓扑**：仅 `web`（前端 nginx）对外暴露公网域名；web 内 nginx 把 `/api/*` 反代到内部 `api:8080`（保留 `/api/v1/...` 原路径）→ 单域名、免 CORS、CSP 对 API 维持 `connect-src 'self'`；SSE 关缓冲 + 长读超时。

**构建期变量（Vite 内联，实测坑）**：`import.meta.env.VITE_*` 在 `vite build` 时内联进产物 → 必须作为 docker **build ARG** 传入，并在 Coolify 里逐个勾选 **"Build Variable"**；多阶段时已在 builder stage 内部再次 `ARG`。

| 变量 | 值 | 说明 |
|---|---|---|
| `VITE_API_BASE_URL` | `/api/v1` | 同源相对路径，axios `baseURL` |
| `VITE_MAP_KEY` | `<真实 key>` | 公开值；**须在腾讯地图控制台把 Coolify 域名加入该 key 域名白名单**，否则不出图 |
| `VITE_CDN_BASE` | `<CDN 基址>` | 预留；媒体 URL 由 `GET /api/v1/media/{id}` 返回完整签名 |
| `VITE_ENABLE_MSW` | `false` | 生产必须关闭 |

> **生产构建结构性剔除 MSW**：`main.tsx` 的 mock 分支以 `import.meta.env.DEV` 守卫，`vite build` 下整段被静态消除，MSW(~300KB) 不进生产包（与 `VITE_ENABLE_MSW=false` 双保险）。

**CSP 媒体域**：全景/缩略图签名 URL 从 CDN 加载 → `nginx.conf` 的 `connect-src` 与 `img-src` 须含该 CDN 域；上线前把 `*.myqcloud.com` 收敛到确切 bucket/CDN。若腾讯地图 GL 报 CSP eval 错，再在 `script-src` 追加 `'wasm-unsafe-eval'`（先测再加）。

```bash
# 本地验证镜像（产物已内联 VITE_*、MSW 不在包）
docker build -f deploy/Dockerfile \
  --build-arg VITE_API_BASE_URL=/api/v1 \
  --build-arg VITE_MAP_KEY=<key> \
  --build-arg VITE_ENABLE_MSW=false \
  -t c5-web-admin .
```
