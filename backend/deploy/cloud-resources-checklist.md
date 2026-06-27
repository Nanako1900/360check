# 腾讯云资源开通清单（P0 · DevOps 负责）

> 本清单是 P0「腾讯云资源开通」的交付物。**实际开通需要 DevOps 持有腾讯云账号/CAM 凭证**，
> 不在本仓库（后端代码）职责内执行；后端只消费这些资源的连接信息（经 K8s Secret / 密管注入，
> 见 `.env.example` 的 `C5_*` 变量），**绝不**把任何密钥写入代码库。
>
> 完成每项后在 `[ ]` 打勾，并把连接信息登记到密管（非本仓库）。

## 1. TencentDB for PostgreSQL（**非 TDSQL-C**）
- [ ] 实例类型确认为 **TencentDB for PostgreSQL**（PostgreSQL **16**）。
- [ ] 控制台「扩展」开启 **PostGIS 3.4** 与 **pgcrypto**（`CREATE EXTENSION` 由 000001 迁移兜底，但需实例支持）。
- [ ] 创建库 `c5`，创建**最小权限**应用账号（仅 DML + 必要 DDL 由迁移 Job 账号执行）。
- [ ] 记录 DSN → `C5_DB_DSN`（`postgres://user:pass@host:5432/c5?sslmode=require`）。
- 验证：`SELECT postgis_full_version();` 返回 3.4.x；`ST_Length('LINESTRING(...)'::geography)` 返回米。

## 2. Cloud Redis（6.x/7.x）
- [ ] 开通 Cloud Redis；refresh / casbin-watcher / asynq 共用实例或分 DB index。
- [ ] 记录 `C5_REDIS_ADDR` / `C5_REDIS_PASSWORD` / `C5_REDIS_DB`。

## 3. COS 对象存储（三层）
- [ ] 单桶三前缀或三桶：`original/`、`web/`、`thumb/`，区域如 `ap-guangzhou`。
- [ ] 记录 `C5_COS_BUCKET_ORIGINAL/WEB/THUMB`、`C5_COS_REGION`。

## 4. CAM — STS 受限角色（P5 媒体直传）
- [ ] 创建 STS 角色，策略**仅** 6 个分片 action：
      `InitiateMultipartUpload, UploadPart, CompleteMultipartUpload, AbortMultipartUpload, ListMultipartUploads, ListParts`。
- [ ] `resource` 限定到 per-upload `prefix/*`；TTL 短（30–60min）。
- [ ] 记录 `C5_COS_SECRET_ID/SECRET_KEY`（签发 STS 用的主账号或子账号）、`C5_COS_STS_ROLE_ARN`。
- [ ] 记录 **`C5_COS_APP_ID`**（腾讯云 APPID，数字）——`stsimpl` 用它拼 `resource` ARN（`qcs::cos:<region>:uid/<appid>:<bucket>/<prefix>*`），缺失则 STS 签发失败。

## 5. CDN（媒体下行，签名 URL）
- [ ] 加速域名回源 COS；开启 URL 鉴权（签名）。
- [ ] 记录 `C5_COS_CDN_DOMAIN`。

## 6. 容器与发布
- [ ] **TCR** 镜像仓库（清单镜像名 `ccr.ccs.tencentyun.com/c5/c5-api`、`.../c5-worker`，用 `kustomize edit set image` 改 tag）；**TKE**（或 Lighthouse）集群。
- [ ] 探针（k8s manifests 已就绪，见 `deploy/k8s/`）：api liveness `/livez`（仅进程，不连依赖，避免依赖抖动触发重启）、readiness `/readyz`（连 DB+Redis）；worker liveness/readiness 在 `:9091`（`/healthz`、`/readyz`）。`/api/v1/healthz` 为契约健康端点保留。
- [ ] 迁移：上线前先跑 `c5-migrate` Job（`c5-api migrate`，golang-migrate 持咨询锁、幂等），等待 complete 再滚 Deployment。

## 7. ICP 备案（mainland 硬性前置）
- [ ] **Day-1 启动** mainland 域名 ICP 备案；上线前完成（未备案 CLB 不放行对外，Ingress host 见 `deploy/k8s/ingress.yaml`）。

## 8. 可观测性后端（P7 已接入代码）
- [ ] OTLP collector 端点（gRPC `:4317`）→ `C5_OBSERV_OTLP_ENDPOINT`；为空则 tracing 自动降级为 no-op（本地/dev 可无 collector 启动）。
- [ ] Prometheus 抓取：api `/metrics`（主端口）、worker `/metrics`（`:9091`）；pod 已带 `prometheus.io/scrape` 注解。

---
**安全红线**：所有密钥仅存密管 / K8s Secret；`cloud-resources-checklist.md` 只记录“需要哪些资源 + 哪个 `C5_*` 变量承载”，不写入任何真实值。
