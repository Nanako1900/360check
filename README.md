# 360check — 360 相机巡查标注系统

基于 Insta360 全景相机的现场巡查与标注平台。巡查员用 360 相机采集现场全景，APP 离线记录轨迹与问题点，云端做媒体派生与统计，网页后台审核与导出。

系统为**三端架构**，以后端 OpenAPI 契约为单一事实来源（DB schema / 坐标系 / 离线同步 / 媒体上传一经后端冻结，全系统对齐）。

| 端 | 目录 | 技术栈 | 状态 |
|---|---|---|---|
| **后端 API**（首建层，契约源） | [`backend/`](backend/) | Go 1.24 · Gin · oapi-codegen v2 · pgx/v5 · sqlc · PostGIS · Redis · 腾讯云 COS · JWT | ✅ 完成 + 生产加固 |
| **网页管理后台** | [`web-admin/`](web-admin/) | React 19 · TS 5.9 · Vite 7 · Ant Design v5 Pro · TanStack Query · 腾讯地图 GL · Photo Sphere Viewer · ECharts | ✅ 完成 + 生产加固 |
| **Android APP**（现场端） | 见 [`docs/03`](docs/03-APP开发文档.md) | Kotlin · Insta360 SDK · Room · WorkManager · FusedLocation | ⏳ 待建 |

## 仓库结构

```
.
├── backend/        Go 模块化单体（c5-api + c5-worker）；含契约源 api/openapi.yaml 与 db/migrations
├── web-admin/      React 管理后台（后端 REST + SSE 的纯消费者）
├── docs/           三端共享权威文档（契约 + 各端开发计划）
│   ├── 00-数据模型与API契约.md     冻结契约：DB / OpenAPI / 错误码 / 坐标系 / 同步
│   ├── 01-后端开发文档.md          后端计划 P0–P7
│   ├── 02-网页开发文档.md          网页计划 P0–P8
│   └── 03-APP开发文档.md           Android APP 计划 P0–P7（待实现）
└── .github/workflows/ci.yaml      后端 CI：vet · gen-drift · build · 单测
```

> **Insta360 SDK 不在本仓库内。** 厂商 SDK 与 demo（含 275MB 演示 APK，超出 GitHub 100MB 限制，且受厂商授权约束不公开再分发）由 `.gitignore` 排除。构建 Android APP 时按 [`docs/03`](docs/03-APP开发文档.md) 单独获取并接入私有 Maven。

## 快速开始

### 后端

```bash
cd backend
cp .env.example .env          # 填入本地 DSN / Redis / COS / JWT（≥32 字节）
make gen                      # oapi-codegen + sqlc（gen-sqlc 需 Docker）
make migrate-up               # 应用迁移（管理员账号以 LOCKED 哨兵出厂）
go run ./cmd/api create-admin # 用 C5_BOOTSTRAP_ADMIN_PASSWORD 设置首位管理员
make run                      # 启动 c5-api
```

### 网页后台

```bash
cd web-admin
npm ci
npm run dev                   # 默认 MSW 按 openapi.yaml mock；后端就绪后置 VITE_ENABLE_MSW=false
```

## 安全与生产

- **无默认口令**：种子管理员出厂为 LOCKED（`password_hash='!'`，任何密码均无法登录），首位管理员经一次性 `create-admin` 子命令 / K8s Job 设置。
- **密钥仅经环境变量 / K8s Secret 注入**，源码与仓库零硬编码；`*.example` 仅含 `CHANGE_ME` 占位。
- **后端加固**：登录限流（Redis 固定窗口，按 IP + 用户名）、安全响应头（HSTS / nosniff / XFO / Referrer / Permissions / CORP）、JWT 防算法混淆、Refresh 单次轮换、所有 SQL 参数化、graceful shutdown、livez/readyz、K8s securityContext 收敛。
- **网页加固**：Refresh token 永不落盘（仅内存）、CSP + 安全头（nginx）、自托管字体、跨标签登出。
- 详见各端 `README.md` 与 [`backend/SECURITY-DECISIONS.md`](backend/SECURITY-DECISIONS.md)。

## 部署

**主路径：前后端分离**——前端 → **腾讯云 EdgeOne Pages**（CDN 托管静态 SPA），后端 api → **Coolify / Docker Compose** 公网暴露，浏览器与 APP **跨域**调用（后端 CORS 白名单）。整栈自托管（web nginx 同源反代）为备选。完整设计与安全加固见 [`docs/superpowers/specs/2026-06-29-frontend-backend-split-deploy-design.md`](docs/superpowers/specs/2026-06-29-frontend-backend-split-deploy-design.md)。另保留 **腾讯云 TKE**（`deploy/k8s/`）。

### 前端：EdgeOne Pages（主路径）

绑定 Git 仓库，构建 `npm run build`，产物目录 `dist/`，Node 22（`web-admin/.nvmrc`）。前端构建在 EdgeOne CI 进行，不占用后端服务器。

- **Build 变量**（EdgeOne 后台）：`VITE_API_BASE_URL=https://api.你的域名/api/v1`、`VITE_MAP_KEY`、`VITE_CDN_BASE`、`VITE_ENABLE_MSW=false`。
- **头 / CSP / SPA 回退**：[`web-admin/edgeone.json`](web-admin/edgeone.json)（EdgeOne 专有配置，非 Cloudflare 的 `_headers`/`_redirects`）。⚠️ 把其中 CSP `connect-src` 的 `api.example.com` 替换为真实 API 源。
- **地图 / 备案**：把前端域名加入腾讯地图 key 白名单；大陆域名需 **ICP 备案**（上线硬前置）。

### 后端：Coolify（Docker Compose，公网 api）

Coolify「资源类型 = Docker Compose」指向仓库根 [`docker-compose.yaml`](docker-compose.yaml)。栈含 `postgres`(PostGIS) · `redis`(AOF) · `migrate`(一次性) · `api` · `worker`，**仅 `api` 分配公网域名**（`api.你的域名`）。

- **不写自定义 `networks:`**（会令 Traefik 路由 504）；同栈用服务名 DNS。
- **Magic 变量**：`SERVICE_PASSWORD_DB`、`SERVICE_PASSWORD_REDIS`、`SERVICE_BASE64_64_JWT`(→ `C5_JWT_SECRET`)、`SERVICE_FQDN_API_8080`(api 域名)。
- **CORS**：`C5_CORS_ALLOWED_ORIGINS=https://前端域`（精确、禁 `*`；prod 下后端 fail-fast 校验）。
- **限流安全**：`C5_SERVER_TRUSTED_PROXIES` 必须收窄到 Coolify 代理(Traefik) 的确切子网/IP，否则伪造 `X-Forwarded-For` 可绕过登录限流；留空=不信任代理=取直连 IP（最安全）。
- **`/metrics`**：仅在 api 内网端口 `:9090` 暴露（不经 Traefik），公网 `8080` 不含 `/metrics`。
- **腾讯云 COS / CDN**：`C5_COS_*` 经 Coolify Secret 注入；宿主需能出网 `*.myqcloud.com` 及 CDN 域。COS 桶需对前端域配 CORS（全景图 WebGL 跨域加载）。
- **迁移幂等**：`migrate` 容器每次部署重跑 `c5-api migrate`（golang-migrate 按 version 幂等，持 advisory lock）；`restart:"no"` → Coolify 自动排除出栈健康聚合。`api` 经 `depends_on: migrate(service_completed_successfully)` 等其完成。
- **首位管理员（一次性）**：种子管理员出厂 LOCKED（`'!'`，无默认凭据）。部署后在 Coolify 的 `api` 容器终端跑【一次】 `/c5-api create-admin`（读 `C5_BOOTSTRAP_ADMIN_PASSWORD`），**随后删除该变量**。
- **本地验证**：`cp .env.example .env && docker compose up --build`。`api` 的 Dockerfile 内置 `HEALTHCHECK`（distroless 无 curl，二进制自带 `healthcheck` 子命令探 `/livez`）。

### 自托管整栈（备选）

[`web-admin/deploy/*`](web-admin/deploy/) 保留 nginx 同源反代镜像：把 `web` 服务加回 `docker-compose.yaml`（含 `SERVICE_FQDN_WEB_8080`）即恢复单域名、同源、免 CORS 的整栈形态。

### 腾讯云 TKE（k8s）

`deploy/k8s/`：TencentDB PostgreSQL 16 + PostGIS · Cloud Redis · COS（媒体）· TKE + TCR · 腾讯地图 GL · CDN。Job 顺序：`migrate` → `create-admin` → `rollout`。上线前置：ICP 备案、域名/TLS。
