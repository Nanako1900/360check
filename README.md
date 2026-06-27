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

腾讯云 / 中国大陆：TencentDB PostgreSQL 16 + PostGIS · Cloud Redis · COS（媒体）· TKE + TCR · 腾讯地图 GL · CDN。镜像化部署顺序：`migrate` → `create-admin` → `rollout`。上线前置：ICP 备案、域名/TLS。
