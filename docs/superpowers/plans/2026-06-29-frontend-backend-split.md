# 前后端分离部署 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:executing-plans` / `subagent-driven-development`. Steps use `- [ ]`。

**Goal:** 后端开启 CORS + 安全暴露公网 API（`api.x.com`），前端可独立部署到 EdgeOne Pages（`admin.x.com`）跨域消费。

**Architecture:** 见 spec `docs/superpowers/specs/2026-06-29-frontend-backend-split-deploy-design.md`。方案 B（真·跨域）。决策锁定 **A/A**：`/metrics` 移内网端口；CORP 放宽 `cross-origin`。

**Tech Stack:** Go 1.24 / Gin / viper / prometheus；React/Vite（EdgeOne Pages）；Docker Compose（Coolify）。

**验证基线（每个后端 Task 后）：** `cd backend && go test ./... && golangci-lint run`。

---

### Task 1: config — `Server.AllowedOrigins` + prod 校验

**Files:**
- Modify: `backend/internal/config/config.go`
- Test: `backend/internal/config/config_test.go`

- [ ] **Step 1（RED 测试）**：在 `config_test.go` 加：prod + 空 origins → err 含 `C5_CORS_ALLOWED_ORIGINS`；prod + `*` → err；prod + `https://admin.x.com` → ok；prod + 带路径 `https://a.com/x` → err；非 prod 空 → ok。
- [ ] **Step 2**：`go test ./internal/config/ -run CORS -v` → FAIL（字段/校验不存在）。
- [ ] **Step 3（实现）**：
  - `ServerConfig` 加 `AllowedOrigins []string`。
  - `Load()`：BindEnv 列表加 `"cors.allowed_origins"`；`Server{... AllowedOrigins: splitCSV(v.GetString("cors.allowed_origins"))}`（env = `C5_CORS_ALLOWED_ORIGINS`）。
  - `Validate()` 末尾加：`if c.Env == "prod"` 时 origins 非空且逐项 `validateOrigin`。
  - 新增 helper（import `net/url`）：
    ```go
    func validateOrigin(o string) error {
        if o == "" || strings.Contains(o, "*") {
            return fmt.Errorf("invalid origin %q (no wildcard/empty)", o)
        }
        u, err := url.Parse(o)
        if err != nil || u.Scheme != "https" || u.Host == "" ||
            u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
            return fmt.Errorf("invalid origin %q (want https://host, no path)", o)
        }
        return nil
    }
    ```
- [ ] **Step 4**：`go test ./internal/config/ -v` → PASS。
- [ ] **Step 5**：commit `feat(config): C5_CORS_ALLOWED_ORIGINS + prod fail-fast`。

### Task 2: CORS 中间件

**Files:**
- Create: `backend/internal/server/middleware/cors.go`
- Test: `backend/internal/server/middleware/cors_test.go`

- [ ] **Step 1（RED）**：`cors_test.go`（仿 `secheaders_test.go` 风格）：
  - 命中白名单 GET → `Access-Control-Allow-Origin` 回显该 origin、`Vary` 含 `Origin`、`Expose-Headers` 含 `ETag`。
  - 预检 `OPTIONS`（命中）→ 204 + `Allow-Headers` 含 `Authorization`/`If-None-Match`/`X-Skip-Refresh`。
  - 未命中 origin → 不设 `Allow-Origin`。
  - 流式断言：handler 调用前头已写（用普通 GET 即可证明请求阶段写头）。
- [ ] **Step 2**：`go test ./internal/server/middleware/ -run CORS -v` → FAIL。
- [ ] **Step 3（实现）** `cors.go`：
    ```go
    package middleware

    import (
        "net/http"
        "github.com/gin-gonic/gin"
    )

    // CORS 允许精确白名单内的跨域请求（回显匹配 Origin，绝不 "*"）。鉴权走 Authorization
    // 头（无 cookie）→ 不启用 credentials。预检 OPTIONS 显式 204 短路：本路由未注册任何
    // OPTIONS handler，未 Abort 会落到 NoRoute 返 404。头在请求阶段写入（流式 SSE 首字节前）。
    func CORS(allowedOrigins []string) gin.HandlerFunc {
        allowed := make(map[string]struct{}, len(allowedOrigins))
        for _, o := range allowedOrigins {
            allowed[o] = struct{}{}
        }
        return func(c *gin.Context) {
            h := c.Writer.Header()
            h.Add("Vary", "Origin")
            origin := c.GetHeader("Origin")
            if _, ok := allowed[origin]; ok && origin != "" {
                h.Set("Access-Control-Allow-Origin", origin)
                h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
                h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Skip-Refresh, If-None-Match")
                h.Set("Access-Control-Expose-Headers", "ETag")
                h.Set("Access-Control-Max-Age", "600")
            }
            if c.Request.Method == http.MethodOptions {
                c.AbortWithStatus(http.StatusNoContent)
                return
            }
            c.Next()
        }
    }
    ```
- [ ] **Step 4**：`go test ./internal/server/middleware/ -v` → PASS。
- [ ] **Step 5**：commit `feat(api): CORS middleware (exact allowlist, no credentials)`。

### Task 3: 接入 router

**Files:** Modify `backend/internal/server/router.go`；Test `backend/internal/server/router_test.go`

- [ ] **Step 1**：`router.go` 的 `r.Use(...)` 把 `middleware.CORS(deps.Cfg.Server.AllowedOrigins)` 放**第一**（在 `RequestID` 之前）。
- [ ] **Step 2**：`go test ./internal/server/ -v` → 现有用例仍 PASS（CORS 对无 Origin 请求透明）。
- [ ] **Step 3**：commit `feat(api): wire CORS first in middleware chain`。

### Task 4: CORP `same-site` → `cross-origin`（MUST-FIX 3-A）

**Files:** Modify `backend/internal/server/middleware/secheaders.go`、`secheaders_test.go`

- [ ] **Step 1（RED）**：`secheaders_test.go` 加断言 `Cross-Origin-Resource-Policy == "cross-origin"`。
- [ ] **Step 2**：FAIL（当前 `same-site`）。
- [ ] **Step 3**：`secheaders.go` `crossOriginResource = "cross-origin"`；更新行内注释说明（JSON API 跨站前端需放行）。
- [ ] **Step 4**：PASS。
- [ ] **Step 5**：commit `fix(api): CORP cross-origin so cross-site SPA can read API`。

### Task 5: `/metrics` 移内网端口（MUST-FIX 1-A）

**Files:** Modify `backend/internal/server/router.go`、`router_test.go`、`backend/cmd/api/main.go`

- [ ] **Step 1（RED）**：`router_test.go:135` 改为断言公网引擎 `GET /metrics` → **404**（不再公网）。
- [ ] **Step 2**：`go test ./internal/server/ -run Metrics -v` → 当前 PASS(200) → 改断言后 FAIL。
- [ ] **Step 3（实现）**：
  - `router.go`：删 `r.GET("/metrics", ...)`（行 81-82）+ 删 `promhttp` import（`prometheus` 仍用）。`/livez`/`/readyz` 保留（Dockerfile healthcheck 探 :8080/livez）。
  - `cmd/api/main.go`：加 `const apiMetricsAddr = ":9090"`；构建 engine 后起内网 health/metrics 服务（复用 `obs.NewHealthServer`，与 worker 同款）：
    ```go
    metricsSrv := obs.NewHealthServer(apiMetricsAddr, reg,
        obs.ReadyCheck{Name: "postgres", Check: pool.Ping},
        obs.ReadyCheck{Name: "redis", Check: rdb.Ping},
    )
    go func() {
        if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            serverErr <- err
        }
    }()
    ```
    优雅关闭处加 `_ = metricsSrv.Shutdown(shutdownCtx)`。日志加 `metrics_addr`。
- [ ] **Step 4**：`go test ./... ` → PASS。
- [ ] **Step 5**：commit `fix(api): serve /metrics on internal :9090, off public 8080`。

### Task 6: Compose + .env.example

**Files:** Modify `docker-compose.yaml`、`.env.example`

- [ ] **Step 1**：`api` 服务 environment 加 `SERVICE_FQDN_API_8080: ${SERVICE_FQDN_API_8080:-}`（字典式）、`C5_CORS_ALLOWED_ORIGINS: ${C5_CORS_ALLOWED_ORIGINS:-}`；把 `C5_SERVER_TRUSTED_PROXIES` 改为 `${C5_SERVER_TRUSTED_PROXIES:-}`（默认空=不信任代理=防伪造；部署时设为 Coolify 代理子网，见 MUST-FIX 2 注释）。
- [ ] **Step 2**：**移除 `web` 服务**整块；改顶部注释块（1-21 行）反映"api 直接公网 + 前端在 EdgeOne"。
- [ ] **Step 3**：`.env.example` 加 `SERVICE_FQDN_API_8080=`、`C5_CORS_ALLOWED_ORIGINS=`、`C5_SERVER_TRUSTED_PROXIES=`（带 MUST-FIX 2 警示注释）。
- [ ] **Step 4（验证）**：`docker compose config -q`（语法校验，不起栈）。
- [ ] **Step 5**：commit `feat(deploy): expose api publicly + CORS env; drop web service`。

### Task 7: EdgeOne 静态配置（CF/EdgeOne-Pages 兼容）

**Files:** Create `web-admin/public/_redirects`、`web-admin/public/_headers`

- [ ] **Step 1**：`_redirects`：`/*  /index.html  200`（SPA 回退）。
- [ ] **Step 2**：`_headers`：`/*` 段照搬安全头 + CSP（`connect-src` 含 `https://api.example.com` 占位，`# 替换为真实 API 源`）；`/assets/*` 段 `Cache-Control: public, max-age=31536000, immutable`。
- [ ] **Step 3（验证）**：`cd web-admin && npm run build` → PASS，且 `dist/_headers`、`dist/_redirects` 存在（Vite 复制 public/）。
- [ ] **Step 4**：commit `feat(web): EdgeOne Pages _headers/_redirects (CSP/security/SPA)`。

### Task 8: README 部署章节

**Files:** Modify `README.md`

- [ ] **Step 1**：部署章节加「EdgeOne Pages + 公网 api（主路径）」小节：前端 EdgeOne（构建变量、_headers/_redirects、地图白名单）、后端 api 暴露（CORS、收窄 TrustedProxies、/metrics 内网、CORP）。自托管整栈降为备选。
- [ ] **Step 2**：commit `docs: deploy via EdgeOne Pages + public api (primary path)`。

---

## 验证（全部 Task 后）
- [ ] `cd backend && go test ./...` 全绿。
- [ ] `cd backend && golangci-lint run` 干净。
- [ ] `cd web-admin && npm run build` 通过；`dist/_headers`/`dist/_redirects` 就位。
- [ ] `docker compose config -q` 通过。
- [ ] 独立 code-review（`code-reviewer`）后再推送。

## 自查
- spec 覆盖：CORS(T1-3)、CORP(T4)、/metrics(T5)、暴露 api+收窄代理+删 web(T6)、EdgeOne(T7)、README(T8) ✓。
- 类型一致：`Server.AllowedOrigins` 在 T1 定义、T3 使用；`apiMetricsAddr` 仅 T5。
- 占位：`api.example.com` 为 _headers/CSP 占位（已注释提示替换）。
