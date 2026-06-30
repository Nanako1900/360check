# 前后端分离部署设计（方案 B：真·跨域）

- **日期**：2026-06-29
- **状态**：已确认设计，待 spec 评审 → 实现计划
- **作者**：Claude（与 Nanako 协作）
- **分支**：`feat/frontend-backend-split`
- **关联**：[`docker-compose.yaml`](../../../docker-compose.yaml)、[`web-admin/deploy/`](../../../web-admin/deploy/)、[`backend/internal/server/`](../../../backend/internal/server/)

---

## 1. 背景与动机

### 1.1 现状（单栈同源）
当前前后端在**同一个 Docker Compose 栈**（Coolify / 腾讯云广州），刻意采用「同源」拓扑：

```
浏览器 →(唯一公网域名)→ web(nginx) ┬→ 静态 SPA
                                   └→ /api/* 同源反代 → api:8080(内部)
```

- 仅 `web` 服务暴露公网域名；`api`/`worker`/`postgres`/`redis` 均为内部服务。
- web 容器内 nginx 把 `/api/*` 反代到 `api:8080` → 浏览器单域名 → **免 CORS**，CSP 维持 `connect-src 'self'`。
- `VITE_API_BASE_URL=/api/v1`（相对路径）。

### 1.2 触发问题
Coolify 在腾讯云广州服务器部署时，`web` 服务的 `npm run build` 失败（`exit code: 1`）。诊断结论（见 §8 附录）：

- 本地裸机 + **干净 alpine 容器**（`node:22-alpine` + `npm ci` + `npm run build`，与 Coolify 完全一致）构建**均通过**。
- 退出码 `1`（干净非零退出）反证排除 OOM（OOM 为 134/137）。
- 即：**构建本身没问题，失败是那台服务器环境专有**（疑似磁盘 ENOSPC / Coolify 构建包装 / x64 原生依赖，未最终定位）。

### 1.3 动机（用户确认）
1. **CDN 加速 / 性能**：静态资源边缘加速。
2. **绕过服务器构建失败**：前端构建移出该服务器。
3. **降成本 / 运维**：减少自托管容器与服务器负载。

### 1.4 平台与前置（用户确认）
- 用户主要在**中国大陆** → 前端用**腾讯云 EdgeOne Pages**（类 Cloudflare Pages 的大陆等价物，有大陆节点）。CF Pages 在大陆不可靠，排除。
- **已有 ICP 备案域名** → 可直接用子域 `admin.x.com` / `api.x.com`（下文以此为占位）。

---

## 2. 决策：方案 B（真·跨域）

前端独立部署到 EdgeOne Pages（`admin.x.com`），后端在腾讯云 Coolify 暴露公网 API（`api.x.com`），浏览器**跨域**调用，后端开启 **CORS 白名单**。

### 2.1 为什么是 B 而非"边缘同源代理"（方案 A）
- **Android APP 是三端架构的一端**，它无法用"边缘同源代理"技巧，**必须直连一个公网 API 端点**。即无论如何 `api.x.com` 都要存在。既然如此，web 直接跨域用它最干净。
- 代码已为跨域就绪，新增成本仅 CORS：
  - **鉴权无 cookie**：access 走 `Authorization` 头、refresh 走 JSON body → 跨域零 SameSite 坑。
  - **SSE 是手写 `fetch + ReadableStream`**（非原生 EventSource），已带 `Authorization` 头，且自带 ~2s **轮询回退**（抗 CDN/CLB 缓冲）→ 跨域可用且鲁棒。
    - ⚠️ 注意：SSE 是带 `Authorization` 头的 GET（非安全字段）→ 跨域后**会触发 CORS 预检**（同源时没有）。且流式响应在首字节即 flush 头 → CORS 头必须在**请求阶段**写入（见 §4.1）。
  - **后端已自设安全响应头**（`secheaders.go`）→ 直接暴露 api 不丢安全头。
- CDN 收益落在**静态资源**（大头），API 直连后端（SSE 最稳、无多余一跳、无边缘缓冲风险）。

### 2.2 非目标（Non-goals）
- 不迁移后端出腾讯云（postgres/redis/api/worker 继续在 Coolify 栈）。
- 不最终定位 §1.2 的服务器构建失败根因（分离后对前端 moot；若后端 Docker 构建将来也失败，再单独查服务器磁盘/资源）。
- 不改动业务逻辑、API 契约、DB schema。
- 不引入 cookie 鉴权。

---

## 3. 目标架构

```
                      ┌─ https://admin.x.com → EdgeOne Pages（静态 SPA，CDN 边缘加速，注入 CSP/安全头/SPA 回退）
   浏览器(大陆) ───────┤
                      └─ https://api.x.com   → 腾讯云 Coolify ▶ Traefik ▶ api:8080（开启 CORS）
                                                      │
   Android APP ───────────────────────────────────────┘  （同一 api.x.com；APP 无 CORS 概念）

   后端栈(Coolify,腾讯云): postgres + redis + migrate + api(公网) + worker   ← web 服务移除
```

---

## 4. 详细设计

### 4.1 后端：新增 CORS 中间件（核心后端改动）

> 注：跨域本身只需 CORS；但**公网暴露 api** 另带出 §4.2 的三个 MUST-FIX，其中 metrics 隔离(1-A)、CORP(3-A) 若取推荐的 A 方案也是后端代码改动。

**新增** `backend/internal/server/middleware/cors.go`：

- 挂在 `backend/internal/server/router.go` 的全局 `r.Use(...)` 链**最前**（在 auth / rate-limit 之前）。
- **CORS 头必须在「请求阶段」写入**（`c.Next()` 之前），不能用 deferred/after-Next：流式 SSE 处理器在首字节即 flush 响应头，晚写的 `Allow-Origin` 会丢失。
- `OPTIONS` 预检：设好 CORS 头后**显式 `c.AbortWithStatus(http.StatusNoContent)`（204）**。⚠️ 不能依赖 gin 路由兜底——本仓未注册任何 OPTIONS 路由、`HandleMethodNotAllowed` 默认未开（`router.go:74`），未 Abort 的 OPTIONS 会落到 `r.NoRoute` 返回 **404 信封**而非 204。
- 仅当请求 `Origin` 命中白名单时设置 CORS 头；未命中不设（浏览器据缺失头拦截）。回显 Origin 必与白名单**精确串比**（禁止前缀/子串匹配），并设 `Vary: Origin`。

**响应头**（按已核实的客户端实际行为精确给）：

| 头 | 值 | 依据 |
|---|---|---|
| `Access-Control-Allow-Origin` | 回显命中白名单的 Origin（**非 `*`**） | 精确白名单 |
| `Access-Control-Allow-Methods` | `GET, POST, PUT, DELETE, OPTIONS` | 契约用到的方法 |
| `Access-Control-Allow-Headers` | `Authorization, Content-Type, X-Skip-Refresh, If-None-Match` | `X-Skip-Refresh` 实测会上线（请求拦截器只删 `X-Skip-Auth`）；**`If-None-Match` 是生产代码 `useDict.ts:19` 实发的硬需求**（`useDict.test.ts` 覆盖），缺它字典预检会失败。`Accept`（SSE 用 `text/event-stream`）属安全字段一般无需列，保险可加 |
| `Access-Control-Expose-Headers` | `ETag` | **仅前瞻**：当前字典缓存从 JSON body 的 `content_hash` 钉版、按响应**状态 304** 判定（`useDict.ts:23,34`），并不读 `ETag` 响应头；故 expose 非"必须"，留作未来用 |
| `Access-Control-Max-Age` | `600` | 缓存预检，减少 OPTIONS |
| `Vary` | `Origin` | 多源白名单下正确缓存 |
| ~~`Access-Control-Allow-Credentials`~~ | **不设** | 无 cookie 鉴权 |

**配置**（`backend/internal/config/config.go`）：

- 解析**复用** `splitCSV`（`config.go:218`，既有）：新增 `Server.AllowedOrigins []string` ← `C5_CORS_ALLOWED_ORIGINS`（逗号分隔）。
- ⚠️ **校验逻辑是全新的**：现 `Validate()` 是 env 无关的（仅查 DSN/Redis/JWT 存在性 + CIDR 格式，`TrustedProxies` 校验为无条件格式检查、允许空、无 prod 分支）。本次须**新增** `C5_ENV=prod` 条件分支：prod 时 `AllowedOrigins` **必须非空**，每项规范化为合法 `https://` Origin（require https scheme、reject 路径、reject 尾斜杠、reject 含 `*`），否则启动失败（防误部署成无 CORS 或全开）。
- 非 prod（本地/dev）可空（或允许 `http://localhost:5173` 等开发源）。

**示例值**：`C5_CORS_ALLOWED_ORIGINS=https://admin.x.com`

**单元测试** `cors_test.go`：
- 预检 `OPTIONS` 命中白名单 → 204 + 正确 CORS 头。
- 实际请求命中白名单 → 业务响应带 `Allow-Origin` + `Expose-Headers: ETag`。
- 未命中 Origin → 不设 CORS 头。
- prod 下 `C5_CORS_ALLOWED_ORIGINS` 为空 / 含 `*` → `config.Load` fail-fast（加进 `config_test.go`）。

### 4.2 后端：把 api 暴露公网（Coolify / Compose）

`docker-compose.yaml`：

- `api` 服务新增 `environment: SERVICE_FQDN_API_8080: ${SERVICE_FQDN_API_8080:-}`（Coolify 魔法变量 → Traefik 路由 `api.x.com` → `api:8080`，自动签发 TLS）。
  - 写法用**字典式**（与既有 `SERVICE_FQDN_WEB_8080` 教训一致，列表写法会在 Coolify 合并时报 non-string key）。
- `api` 新增 `C5_CORS_ALLOWED_ORIGINS: ${C5_CORS_ALLOWED_ORIGINS:-}`。
- `api` 已内置 Dockerfile `HEALTHCHECK` + `secheaders.go`（自设安全头）→ 直接暴露基本安全；但有两处**必须处理**（下面 🔴）。

**🔴 MUST-FIX 1 — `/metrics` 会随 api 公网暴露（高敏）**：
- `/metrics`（Prometheus 暴露：路由模板、时延直方图、请求计数、Go runtime/process 指标）挂在**同一 gin 引擎、同一端口 8080**（`router.go:82`），api 进程单端口 8080 对外（`cmd/api/main.go`，**没有**像 worker 那样的独立 `:9091`）。一旦 `SERVICE_FQDN_API_8080` 把 Traefik → api:8080，`https://api.x.com/metrics` 即公开。
- 修复（二选一，推荐 A）：
  - **A（推荐，纵深防御）**：把 `/metrics`（及可选 `/livez`/`/readyz`）移到 **api 独立的内网监听端口**（仿 `cmd/worker/main.go` 的 `:9091`），公网 8080 不再挂 `/metrics`。不依赖 ingress 规则正确性。
  - **B（快速缓解）**：在 Coolify/Traefik 对 `api.x.com` 的 `/metrics` 路径做 **deny**（path 中间件），或仅允许内网来源。

**🔴 MUST-FIX 2 — XFF 伪造 → 登录限流绕过（严重安全）**：
- 登录限流按 `c.ClientIP()` 计键（`ratelimit.go:39` `rl:ip:<path>:<ClientIP>`），`ClientIP` 经 `r.SetTrustedProxies(TrustedProxies)`（`router.go:57`）从 `X-Forwarded-For` 右→左、跳过受信 IP 解析。
- 现 `C5_SERVER_TRUSTED_PROXIES=10.0.0.0/8,172.16.0.0/12` **过宽**：若 Traefik 追加（非覆盖）客户端自带的 XFF，攻击者发 `X-Forwarded-For: 9.9.9.9, 10.1.2.3`——gin 右→左遇到受信私网 `10.1.2.3` 跳过，返回攻击者左端选定值；**每请求轮换该值 → 登录限流形同虚设**（暴力破解/撞库不再受限）。
- 修复（两条都做）：
  1. **收窄 `C5_SERVER_TRUSTED_PROXIES` 到 Coolify Traefik 的确切网络子网/IP**（部署时从 Coolify 代理网络确定，**不要** 10/8 + 172.16/12）。这样 gin 只信任唯一上游 Traefik，返回 Traefik 实测的真实客户端 IP，忽略攻击者左端注入。
  2. **确认 Traefik 对不受信来源覆盖/清洗 `X-Forwarded-For`**（Traefik `forwardedHeaders` 不信任任意客户端）。
- 验证：部署后构造伪造 XFF 请求，确认 `c.ClientIP()`（可临时打点 / 看限流键）取到真实边缘 IP 而非注入值。

**🔴 MUST-FIX 3 — CORP `same-site` 会挡跨站前端**：
- `secheaders.go:18,32` 对**每个 api 响应**设 `Cross-Origin-Resource-Policy: same-site`。`admin.x.com` 与 `api.x.com` 同站（共注册域 `x.com`）→ 放行；**但灰度用的 EdgeOne 临时域名（如 `*.edgeone.app`）与 `api.x.com` 跨站 → 即便 CORS 正确，CORP `same-site` 也会拦截跨域读取**（§7.2 步骤 2 会直接撞墙）。
- 修复（二选一）：
  - **A（推荐）**：api 是 JSON API，CORP `same-site` 收益很小 → 放宽为 `cross-origin`（`secheaders.go` 改 `crossOriginResource`，含其单测）。
  - **B**：强制所有前端来源（含灰度域名）与 `api.x.com` **同站**，不用任意临时域名做跨站验证。

### 4.3 前端：迁移到 EdgeOne Pages

**构建**：
- EdgeOne Pages 绑定 Git 仓库，构建命令 `npm run build`，产物目录 `dist/`，Node 22（`package.json` engines `>=22.12 <23` 已声明，且 **`web-admin/.nvmrc` 已存在=`22`**；只需确认 EdgeOne 构建镜像尊重 `.nvmrc`）。
- **前端构建从此在 EdgeOne CI 跑，腾讯云服务器不再构建前端** → §1.2 的构建失败彻底绕开。

**构建期环境变量**（EdgeOne Pages 后台配置 → Vite build 期内联）：
- `VITE_API_BASE_URL=https://api.x.com/api/v1`
- `VITE_MAP_KEY=<腾讯地图 GL key>`
- `VITE_CDN_BASE=<如使用；当前媒体 URL 由 GET /api/v1/media/{id} 返回完整签名>`
- `VITE_ENABLE_MSW=false`

**安全头 / CSP / SPA 回退 / 缓存**：从 `web-admin/deploy/nginx.conf` 搬到 **EdgeOne 专有 `web-admin/edgeone.json`** 的 `headers` / `rewrites` 字段（**非** Cloudflare 的 `_headers`/`_redirects`，见 §6 已确认）：

- **SPA 回退**：未命中路由 → `/index.html`（状态 200）。
- **安全头**（照搬 nginx 现值）：
  - `Strict-Transport-Security: max-age=31536000; includeSubDomains`
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`
  - `Referrer-Policy: strict-origin-when-cross-origin`
  - `Permissions-Policy: camera=(), microphone=(), geolocation=()`
- **CSP 关键改动**：`connect-src` 在 `'self'` 基础上**新增 `https://api.x.com`**（跨域 API + SSE）。其余照旧：
  ```
  default-src 'self';
  script-src 'self' https://map.qq.com https://*.map.qq.com;
  style-src 'self' 'unsafe-inline';
  img-src 'self' data: blob: https://*.map.qq.com https://*.gtimg.com https://*.myqcloud.com;
  font-src 'self' data:;
  connect-src 'self' https://api.x.com https://*.map.qq.com https://*.gtimg.com https://*.myqcloud.com;
  worker-src 'self' blob:; frame-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'
  ```
- **缓存**：`/assets/*` → `Cache-Control: public, max-age=31536000, immutable`（内容哈希文件名）；`/` 与 `index.html` → 不缓存（新部署即时生效）。EdgeOne CDN 边缘缓存自动叠加。

**腾讯地图控制台**：把 `admin.x.com` 加入该 key 的域名白名单（否则地图 GL 拒绝加载）。

### 4.4 前端代码改动
- **几乎为零**：`VITE_API_BASE_URL` 由相对变绝对，`src/env.ts`（zod 校验，默认 `/api/v1`）与 `src/shared/api/http.ts`、`sse.ts` 均已通过该变量取值，无需改代码。
- axios `withCredentials` 保持默认 `false`（无 cookie）。
- 核对：全仓无"假设同源"的硬编码（已确认 http/sse 均走 `env.VITE_API_BASE_URL`）。

### 4.5 Compose 收尾
- 从活动 `docker-compose.yaml` **移除 `web` 服务**（少一容器，契合"降成本/运维"）。
  - **依赖方向**：无任何服务 `depends_on: web` → 删 web 不破坏启动顺序；`web` 自身的 `depends_on: - api`（`docker-compose.yaml:159-160`）随之删除。
  - ⚠️ **同步改顶部注释块（1-21 行）**：现注释声明"仅 web 服务分配公网域名 / nginx `/api`→`api:8080` 反代"，拆分后该拓扑不再成立（api 直接公网 + web 移除），否则注释与实际矛盾、误导后人。
- **保留** `web-admin/deploy/*`（Dockerfile / nginx.conf / compose.web.yaml）作为**自托管回退路径**（README 的 Coolify 整栈方案）。
- 更新 `README.md` 部署章节：把 EdgeOne + api 分离列为**主路径**，自托管整栈为备选。

### 4.6 安全再加固（因反转了"同源"决定）
上线前 checklist：
- [ ] CORS 白名单精确到 `https://admin.x.com`，**永不 `*`**；回显 Origin 精确串比 + `Vary: Origin`。
- [ ] CSP `connect-src` / `img-src` 收敛到确切 api / 地图 / COS 域（去宽松通配 `*.myqcloud.com` → 确切 bucket/CDN 域）。
- [ ] 两个域名都启用 HSTS（EdgeOne 证书 + Traefik 证书）。
- [ ] **`/metrics` 不公网可达**（§4.2 MUST-FIX 1：独立内网端口 或 ingress deny）。
- [ ] **`C5_SERVER_TRUSTED_PROXIES` 收窄到 Traefik 确切子网** + Traefik 清洗 XFF（§4.2 MUST-FIX 2），并实测无法伪造 ClientIP。
- [ ] **CORP 决策落地**（§4.2 MUST-FIX 3：放宽 `cross-origin` 或灰度域名同站）。
- [ ] **COS 桶 CORS**：全景图经 Photo Sphere Viewer 以 WebGL 纹理（`crossOrigin`）从 COS/CDN 签名 URL 加载，确认 COS 桶/CDN 对 `admin.x.com` 返回 `Access-Control-Allow-Origin`（媒体本就跨域，与本次拆分无关，但须确认已配）。
- [ ] `api.x.com` 仅暴露必要端口；`/livez`/`/readyz` 不泄敏。

---

## 5. 影响文件清单

| 文件 | 改动 |
|---|---|
| `backend/internal/server/middleware/cors.go` | **新增** CORS 中间件（请求阶段写头 + OPTIONS `AbortWithStatus(204)` + 精确白名单 + `Vary`） |
| `backend/internal/server/middleware/cors_test.go` | **新增** 单测（预检/白名单/expose/SSE 响应带 Allow-Origin） |
| `backend/internal/server/router.go` | 全局 `r.Use(...)` 链最前挂 CORS；MUST-FIX 1-A 时把 `/metrics` 从公网引擎移除 |
| `backend/internal/server/middleware/secheaders.go` + `_test.go` | **MUST-FIX 3**：CORP `same-site` → `cross-origin`（若选方案 A） |
| `backend/cmd/api/main.go` | **MUST-FIX 1-A**：新增 api 独立内网端口（仿 worker `:9091`）承载 `/metrics`（+可选探针） |
| `backend/internal/config/config.go` | 新增 `Server.AllowedOrigins`：复用 `splitCSV`；**新增** prod 条件校验（https/无路径/无尾斜杠/非 `*`/非空）+ Validate 感知 `c.Env` |
| `backend/internal/config/config_test.go` | 新增 CORS 配置校验测试（含 prod fail-fast） |
| `docker-compose.yaml` | api 加 `SERVICE_FQDN_API_8080` + `C5_CORS_ALLOWED_ORIGINS`；收窄 `C5_SERVER_TRUSTED_PROXIES`；**移除 `web` 服务**；**改顶部注释块（1-21 行）**——"仅 web 暴露域名 + nginx /api 反代"已过时 |
| `.env.example` | 新增 `SERVICE_FQDN_API_8080`、`C5_CORS_ALLOWED_ORIGINS`；更新 `C5_SERVER_TRUSTED_PROXIES` 注释 |
| Coolify/Traefik 配置 | api.x.com TLS；MUST-FIX 1-B 时对 `/metrics` 做 path deny；确认 Traefik 清洗 XFF |
| `web-admin/`（EdgeOne 配置） | 新增 `_headers` / `_redirects`（或 EdgeOne 规则）。`web-admin/.nvmrc` 已存在(22) |
| `README.md` | 部署章节：EdgeOne + api 分离为主路径 |
| `web-admin/deploy/*` | 保留（回退路径），不删 |

---

## 6. 待实现时确认（Open Items）
1. ~~EdgeOne Pages 头/重定向确切机制~~ **已确认（官方文档）**：EdgeOne Pages **不读** Cloudflare 的 `_headers`/`_redirects`，改用根目录 `edgeone.json`（字段 `buildCommand`/`outputDirectory`/`nodeVersion`/`rewrites`/`headers`/`redirects`）。已落地为 [`web-admin/edgeone.json`](../../../web-admin/edgeone.json)。SPA 回退用 `rewrites:[{source:"/*",destination:"/index.html"}]`（部署后须实测深链刷新）。env 变量（`VITE_*`）在 EdgeOne 控制台配置，不在 edgeone.json。
2. ~~真实 dict 客户端是否已发 `If-None-Match`~~ **已核实（非待定）**：生产代码 `web-admin/src/shared/dict/useDict.ts:19` 确实发 `If-None-Match`（`useDict.test.ts` 覆盖）→ `Access-Control-Allow-Headers` **必须**含它。`ETag` expose 仅前瞻（dict 读 body `content_hash` + 304 状态，不读 ETag 头）。
3. **MUST-FIX 决策三连**（实现时拍板，见 §4.2）：`/metrics` 隔离取 A（独立端口）还是 B（ingress deny）；CORP 取 A（放宽 `cross-origin`）还是 B（灰度域名同站）。
4. **COS 桶 CORS**：确认全景媒体（COS/CDN 签名 URL）对 `admin.x.com` 已配 `Access-Control-Allow-Origin`（WebGL 纹理跨域加载）。
5. **EdgeOne Pages 是否需独立 ICP / 加速区选择**：大陆加速区需确认域名 ICP 绑定与 EdgeOne 站点关系（用户已有备案域名）。

---

## 7. 测试与灰度

### 7.1 测试
- **后端**：CORS 中间件单测（预检 / 白名单 / expose ETag / prod fail-fast）。现有集成测试不受影响（CORS 只增不改路由与鉴权）。`go test ./...` 全绿 + `golangci-lint` 通过。
- **前端**：用绝对 `VITE_API_BASE_URL` 本地 `npm run build` 通过；浏览器实测跨域 **登录 → 列表 → 地图 → 导出(SSE)**，DevTools console 无 CSP / CORS 拦截，Network 预检 200。

### 7.2 灰度步骤
1. 后端先上：api 暴露 `api.x.com` + CORS（先放宽白名单含临时前端源），`/metrics` 隔离（MUST-FIX 1），收窄 TrustedProxies（MUST-FIX 2），`go test` 全绿后部署。
2. 前端在 EdgeOne Pages 用**临时分配域名**构建，`VITE_API_BASE_URL` 指向 `api.x.com`，验证跨域形态。
   - ⚠️ 临时域名（如 `*.edgeone.app`）与 `api.x.com` **跨站** → 若 CORP 仍是 `same-site` 会被拦。**必须先做 MUST-FIX 3（CORP→`cross-origin`）**，否则此步无法验证（或直接跳到同站 `admin.x.com` 验证）。
3. 绑定自定义域 `admin.x.com`（ICP），收敛 CORS 白名单到 `https://admin.x.com`，收敛 CSP。
4. 腾讯地图 key 加入 `admin.x.com` 白名单；确认 COS 桶 CORS 含 `admin.x.com`。
5. 全链路回归（登录/列表/地图/全景/导出 SSE，构造伪造 XFF 验限流）→ 切流。

### 7.3 回滚
- 前端回滚：EdgeOne Pages 回滚到上一构建；或临时启用保留的自托管 `web` 服务（compose 重新加入 `web` + `SERVICE_FQDN_WEB_8080`）。
- 后端回滚：CORS 中间件与 api 暴露是增量、可独立回退（移除 `SERVICE_FQDN_API_8080` 即收回公网）。

---

## 8. 附录：服务器构建失败诊断（已闭环到"非代码"）

| 实验 | 结果 | 排除 |
|---|---|---|
| 本地裸机 `npm run build` (node20) | ✅ exit 0 | 代码 / 类型错误 |
| 558 JS/TS 导入 + CSS `@import` + git 大小写审计 | ✅ 一致 | 大小写敏感 |
| `npm ci --dry-run` + lock 含全平台 musl 原生包 | ✅ 同步/齐全 | 依赖漂移 / 原生缺失 |
| 干净 alpine 容器构建 (node22/musl/arm64，同 Coolify) | ✅ **exit 0** | alpine/node22/Linux大小写/arm64-musl |
| 堆限 512MB | exit **134** | **OOM**（≠服务器的 exit 1） |

**结论**：构建本身正确，失败为腾讯云服务器环境专有（疑似磁盘 ENOSPC / Coolify 构建包装 / x64 原生依赖，未最终定位）。分离方案把前端构建移出该服务器 → 该问题对前端 moot。若需根因，取服务器 `df -h` / `docker system df` 与 Coolify 构建步骤真实日志即可定位。
