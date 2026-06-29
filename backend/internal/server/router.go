package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/nnkglobal/c5-backend/internal/config"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
	"github.com/nnkglobal/c5-backend/internal/platform/jwt"
	"github.com/nnkglobal/c5-backend/internal/rbac"
	"github.com/nnkglobal/c5-backend/internal/server/middleware"
)

// APIBasePath is the contract base path (servers.url in the OpenAPI spec).
const APIBasePath = "/api/v1"

// RouteRegistrar mounts a domain's routes on the given router group.
type RouteRegistrar func(gin.IRouter)

// Deps are the constructed dependencies the server needs to build its router.
type Deps struct {
	Cfg      *config.Config
	Logger   *slog.Logger
	DB       Pinger
	Redis    Pinger
	Registry *prometheus.Registry

	// P2+ auth/RBAC. When JWT+Enforcer are set, ProtectedRoutes are mounted
	// behind Authn+Authz. PublicRoutes mount on /api/v1 without auth (login/refresh).
	JWT             *jwt.Manager
	Enforcer        *rbac.Enforcer
	PublicRoutes    []RouteRegistrar
	ProtectedRoutes []RouteRegistrar

	// AuthRateLimiter, when set, is applied to the public auth group (login +
	// refresh) to throttle brute-force / credential-stuffing.
	AuthRateLimiter gin.HandlerFunc
}

// New builds the gin engine with the full middleware chain and mounts the P0
// endpoints: /livez, /readyz and /api/v1/healthz (spec). /metrics is served on a
// separate internal-only port (see cmd/api), not on this public engine. Domain
// handlers implementing oapi.ServerInterface are registered on apiV1 in P2+.
func New(deps Deps) *gin.Engine {
	gin.SetMode(deps.Cfg.Server.Mode)

	r := gin.New()
	// Trust only the configured ingress proxy CIDRs when resolving the client IP
	// from X-Forwarded-For. Empty (the default) => trust no proxy, so c.ClientIP()
	// is the direct peer and a client cannot spoof its IP to evade the per-IP auth
	// rate limiter. CIDRs are validated at config load; on the unreachable error
	// path we fail closed (ignore forwarded headers entirely).
	if err := r.SetTrustedProxies(deps.Cfg.Server.TrustedProxies); err != nil {
		r.ForwardedByClientIP = false
	}
	httpMetrics := middleware.NewHTTPMetrics(deps.Registry)

	// Chain: CORS (first, so preflight short-circuits and Allow-Origin is set in the
	// request phase before any streaming handler flushes) -> request id -> otel trace
	// -> recover(panic->500 envelope) -> slog access log -> prometheus metrics.
	r.Use(
		middleware.CORS(deps.Cfg.Server.AllowedOrigins),
		middleware.RequestID(),
		middleware.SecurityHeaders(),
		otelgin.Middleware(deps.Cfg.Observ.ServiceName),
		middleware.Recovery(deps.Logger),
		middleware.AccessLog(deps.Logger),
		httpMetrics.Middleware(),
	)

	// Unmatched route / method -> NOT_FOUND envelope (consistent with the contract).
	r.NoRoute(func(c *gin.Context) {
		httpx.Fail(c, httpx.NewError(oapi.NOTFOUND, "resource not found"))
	})
	r.NoMethod(func(c *gin.Context) {
		httpx.Fail(c, httpx.NewError(oapi.NOTFOUND, "resource not found"))
	})

	// NOTE: /metrics is intentionally NOT mounted on this public engine. Once api is
	// exposed to the internet (api.x.com), Prometheus exposition (route templates,
	// latency histograms, Go/process internals) must not be public. cmd/api serves
	// /metrics on a separate internal-only port via obs.NewHealthServer.

	// Kubernetes probes at the root: /livez (liveness, process-only) and /readyz
	// (readiness, DB+Redis). Distinct from the spec's /api/v1/healthz.
	registerProbes(r, deps.DB, deps.Redis)

	apiV1 := r.Group(APIBasePath)
	registerHealth(apiV1, deps.DB, deps.Redis)

	// Public routes (login/refresh) get the auth rate limiter when configured.
	publicGroup := apiV1.Group("")
	if deps.AuthRateLimiter != nil {
		publicGroup.Use(deps.AuthRateLimiter)
	}
	for _, reg := range deps.PublicRoutes {
		reg(publicGroup)
	}
	if deps.JWT != nil && deps.Enforcer != nil && len(deps.ProtectedRoutes) > 0 {
		protected := apiV1.Group("", rbac.Authn(deps.JWT), rbac.Authz(deps.Enforcer))
		for _, reg := range deps.ProtectedRoutes {
			reg(protected)
		}
	}

	return r
}
