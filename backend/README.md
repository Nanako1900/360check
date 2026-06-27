# C5 Backend — 360 相机巡查标注系统

Go 模块化单体（`c5-api` + `c5-worker`），腾讯云 / 中国大陆部署。后端是三端中**首建层**，
一旦完成即冻结全系统契约（OpenAPI 3 / DB schema / 坐标系 / 离线同步 / 媒体上传）。

- 权威契约：[`../docs/00-数据模型与API契约.md`](../docs/00-数据模型与API契约.md)
- 开发计划（P0–P7）：[`../docs/01-后端开发文档.md`](../docs/01-后端开发文档.md)
- 冻结 API：[`api/openapi.yaml`](api/openapi.yaml)（oapi-codegen v2 → Go/TS/Kotlin 客户端单一来源）

> 目录布局：后端（含 `api/openapi.yaml` 契约源与 `db/migrations`）位于仓库 `backend/`；
> `docs/` 在仓库根（三端共享）；web 前端在 `web-admin/`，App 在 `Android-SDK*/`。

## 技术栈

Go 1.24 · Gin · oapi-codegen v2 · pgx/v5 · sqlc · paulmach/orb+ewkb · golang-migrate ·
golang-jwt/v5 + Redis 不透明 refresh · casbin v2 (DB 策略 + redis-watcher) ·
cos-go-sdk-v5 (STS) · hibiken/asynq · excelize/v2 · OTel/Prometheus/slog ·
TencentDB PG16 (PostGIS 3.4) / Cloud Redis / COS / TKE。

## 快速开始

```bash
cp .env.example .env            # 填本地 DSN/Redis/JWT（COS 留空即可，P5 才需要）
make compose-up                 # 起 PostGIS 16-3.4 + Redis 7
set -a; source .env; set +a
make run                        # 启动 c5-api，GET /api/v1/healthz 应返回 200 信封
```

## 常用命令

```bash
make gen           # 代码生成（oapi-codegen；CI 跑 make gen && git diff --exit-code 防漂移）
make build         # 编译全部
make test          # 单元测试（-race -cover）
make test-integration  # 集成测试（testcontainers PostGIS+Redis，需 Docker）
make lint          # golangci-lint
make migrate-up    # 迁移（P1+；DB_DSN/C5_DB_DSN）
make docker-build  # 构建 c5-api 镜像
```

## 目录

```
api/openapi.yaml          冻结契约（spec-first）
cmd/api                   c5-api（gin server）
cmd/worker                c5-worker（asynq，P5/P6）
internal/gen/oapi         oapi-codegen 产物（DO NOT EDIT）
internal/gen/db           sqlc 产物（P1，DO NOT EDIT）
internal/httpx            统一信封 + 错误码目录 + 分页
internal/config           viper 配置（启动期 fail-fast）
internal/server           gin 引擎 / 中间件链 / 路由
internal/platform/*       db / redis / geo / cos / sts / jwt / asynqx 适配器
internal/<domain>/        auth rbac user dict project inspection problem media sync stats export
db/migrations             golang-migrate 原生 SQL（000001 schema / 000002 seed，已校验）
db/queries                sqlc 输入（P1+）
deploy/                   Dockerfile.api / docker-compose.dev / k8s / 云资源清单
```

## 阶段进度

- [x] **P0** — 脚手架 + CI + 代码生成管线 + 云资源清单
- [x] **P1** — 数据访问层（migrate + PostGIS + sqlc + orb/ewkb + SRID）
- [ ] P2 — JWT 鉴权 + casbin RBAC + 用户管理
- [ ] P3 — 项目/任务/巡查/轨迹 + 里程（geography）
- [ ] P4 — 问题 + 仅追加处理日志 + 状态流转 + 字典 ETag
- [ ] P5 — 媒体（COS STS + HeadObject）+ 幂等同步 + 派生 worker + reaper
- [ ] P6 — 统计聚合（D2）+ asynq Excel 导出（三种 CJK）+ SSE/轮询
- [ ] P7 — 可观测性 + TKE 部署 + 上线/ICP

## 强制约束（摘要）

- 里程必须 `ST_Length(route_geom::geography)`（米）；所有几何 WGS84/4326，GCJ-02 永不入库。
- 错误码用契约 10 条目录（`UNAUTHENTICATED`，**不是** `UNAUTHORIZED`）。
- 离线 `/sync/batch`：`client_uuid` 幂等 + 逐项 SAVEPOINT，返回 accepted/rejected/duplicate。
- 媒体：STS 6 分片 action + HeadObject 校验后才置 `verified_at`；App 只传 original，worker 派生 web/thumb（D4）。
- D3：`PUT /problems/{id}` 改状态同事务原子写 `STATUS_CHANGE` 日志；客户端只 POST COMMENT/REASSIGN。
- 依赖安全：`x/net`（GO-2026-4559）与 `pgx` CVE 的延后/接受决策见 [`SECURITY-DECISIONS.md`](./SECURITY-DECISIONS.md)（go 1.24 锁定下不可达/不可利用）。
