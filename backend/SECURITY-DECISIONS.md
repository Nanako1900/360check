# C5 后端 — 安全决策记录（Security Decisions Log）

> 仅记录**有意为之、需要复审**的安全权衡。每条含：背景、决策、理由、剩余风险、退出/复审触发条件。
> 已落地的安全控制（admin LOCKED 哨兵、create-admin Job、鉴权限流、安全响应头）见
> `docs/01-后端开发文档.md` 与对应代码，不在此重复。

---

## SD-1 (BE-MED2) — 接受延后升级 `golang.org/x/net`（GO-2026-4559，HTTP/2 DoS）

- **状态**：ACCEPTED（延后升级，记录在案）
- **日期**：2026-06-27
- **严重度**：MEDIUM（在本服务的实际可达性下为 INFORMATIONAL，见“剩余风险”）

### 背景
- `govulncheck` / OSV 报告 **GO-2026-4559**：`golang.org/x/net/http2` 存在 HTTP/2 资源耗尽型 DoS。
- 修复版本为 `golang.org/x/net v0.51.0`，但该版本的最低 Go 要求为 **Go ≥ 1.25**。
- 本仓库被**硬约束锁定在 Go 1.24**（`go.mod`：`go 1.24.0`；多个 latest 直接依赖的更高版本同样要求 ≥1.25）。
  升级 `x/net` 到 0.51.0 会强制抬升 Go 语言版本下限，违反锁定约束。

### 决策
- **保持 `golang.org/x/net v0.50.0`**（`go.mod` 中为 `// indirect`），不升级到 0.51.0。
- **不**抬升 `go.mod` 的 `go` 指令（维持 `go 1.24.0`）。

### 理由（为什么这是可接受的）
1. **不在服务路径上**：本服务的 HTTP 服务器走标准库 `net/http`（Gin 构建于 `net/http` 之上），
   **从不直接 import `golang.org/x/net/http2`**。
   核验：`grep -rn 'golang.org/x/net' internal/ cmd/ → 空`；`x/net` 仅为传递性 `// indirect` 依赖。
   GO-2026-4559 的脆弱代码路径（`x/net/http2` 的 server 实现）在生产部署中**不可达**。
2. **入口前置缓解**：生产入口为 Ingress/网关终止 TLS 并做 HTTP/2 限流；服务自身的 `http.Server`
   设置了 `ReadHeaderTimeout`，进一步收敛慢速/半开连接面。
3. **工具链自动取最新补丁**：CI 用 `actions/setup-go` + `go-version-file: backend/go.mod`，
   始终以最新的 **1.24.x** 补丁工具链构建，自动纳入标准库（含 `net/http` HTTP/2 栈）的安全修复。

### 剩余风险
- 若未来**直接** import `golang.org/x/net/http2`（如自管 h2c/gRPC-Web 网关），则 GO-2026-4559 立即变为可达，
  本决策失效。
- `x/net` 仍停留在 0.50.0，OSV 扫描会持续标红该传递依赖——属**已知并接受**，非遗漏。

### 退出 / 复审触发条件（满足任一即重新评估并升级）
- 项目将 Go 版本下限抬升到 **≥ 1.25**（届时直接 `go get golang.org/x/net@v0.51.0` 并 `go mod tidy`）。
- 任何代码**直接** import `golang.org/x/net/http2` 或 `x/net/http2/h2c`。
- 出现针对 `x/net v0.50.0` 的、本服务可达路径上的新增高危公告。

---

## SD-2 (BE-MED2) — 接受 `pgx` 相关 CVE（参数化查询，不可利用）

- **状态**：ACCEPTED（不可利用，记录在案）
- **日期**：2026-06-27
- **严重度**：MEDIUM 公告 → 本服务下为 INFORMATIONAL

### 背景
- OSV 对 `github.com/jackc/pgx` 系列存在公告（多与 SQL 注入 / 协议解析相关）。

### 决策
- **维持当前 `pgx/v5` 锁定版本**，不为该公告做计划外升级。

### 理由
- 全部数据库访问经 **sqlc 生成的参数化查询**（`internal/gen/db`），
  代码中**无 SQL 字符串拼接**、无动态拼 SQL 的查询构造路径。
- 因此以“注入”为前提的利用面在本服务中不存在；公告在本使用方式下**不可利用**。

### 退出 / 复审触发条件
- 引入任何手写动态 SQL / 字符串拼接查询。
- 出现影响 `pgx` 连接层/协议解析、且与查询参数化无关的可达高危公告。
- 常规依赖升级窗口（随 Go 下限抬升一并处理）。

---

## SD-3 (BE-HIGH1 加固) — 反向代理信任边界（防 X-Forwarded-For 伪造绕过 per-IP 限流）

- **状态**：IMPLEMENTED（代码 fail-secure 默认；生产需配置真实入口网段）
- **日期**：2026-06-27
- **严重度**：HIGH（未加固时 per-IP 限流可被请求头伪造完全绕过）

### 背景
- Gin 默认 `trustedProxies = 0.0.0.0/0` + `::/0` 且 `ForwardedByClientIP=true`，
  `c.ClientIP()` 直接采信客户端可控的 `X-Forwarded-For`。攻击者每请求轮换 XFF，
  即可让 `rl:ip:<path>:<ip>` 每次都是新 key，**完全绕过** 20/5min 的 per-IP 限流
  （per-username 维度不受影响，仍对已知用户名生效）。

### 决策 / 实现
- 新增 `C5_SERVER_TRUSTED_PROXIES`（逗号分隔 CIDR/IP；启动时 `netip` 校验，非法即 fail-fast）。
- `server.New` 在 `gin.New()` 后 `r.SetTrustedProxies(cfg.Server.TrustedProxies)`。
- **默认空 → 不信任任何代理 → `ClientIP()` 取直连对端（不可伪造）→ fail-secure**。

### 生产部署要求（MUST）
- 将 `C5_SERVER_TRUSTED_PROXIES` 设为真实入口（CLB/Ingress）出口网段。
- 否则 LB 后所有客户端折叠成 LB 单 IP：per-IP 限流退化为「全局限流」
  （仍 **不可被绕过**，但失去逐客户端粒度）。`configmap.yaml` 已带占位 `10.0.0.0/8`，
  按实际网段替换；切勿填过宽网段（会重新打开 XFF 伪造面）。

### 剩余风险 / 复审触发
- 入口拓扑变化（新增/更换 LB、引入多层代理）→ 重新核对 CIDR。
- 若未来在入口前再加一层代理而未同步该配置，per-IP 粒度可能再次失真（非绕过）。

---

### 复审节奏
- 每次抬升 Go 版本下限时，逐条复核本表的“退出/复审触发条件”。
- 建议在 CI 增设 `govulncheck` 信息态步骤（non-blocking），新公告出现时回到本表登记决策，而非静默忽略。
