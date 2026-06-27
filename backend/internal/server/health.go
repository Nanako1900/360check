package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/httpx"
)

// Pinger is the minimal dependency-health interface (satisfied by db.Pool and
// redis.Client). Defined where it is consumed, per Go idiom.
type Pinger interface {
	Ping(ctx context.Context) error
}

// depPingTimeout bounds a dependency ping so a hung backend cannot wedge a probe.
const depPingTimeout = 3 * time.Second

// pingDeps pings DB and Redis under ctx and reports which (if any) failed. The
// shared core of GET /api/v1/healthz and GET /readyz.
func pingDeps(ctx context.Context, dbPinger, redisPinger Pinger) (dbOK, redisOK bool, failed []string) {
	dbOK = dbPinger.Ping(ctx) == nil
	redisOK = redisPinger.Ping(ctx) == nil
	if !dbOK {
		failed = append(failed, "db")
	}
	if !redisOK {
		failed = append(failed, "redis")
	}
	return dbOK, redisOK, failed
}

// unhealthy503 writes the 503 envelope naming the failed dependencies. The frozen
// 10-code catalog has no health-specific code and the spec maps 503 -> generic
// ErrorResponse without constraining the code, so INTERNAL is the closest
// canonical choice — do not introduce a non-canonical code.
func unhealthy503(c *gin.Context, failed []string) {
	c.JSON(http.StatusServiceUnavailable, httpx.Envelope{
		Success: false,
		Error: &oapi.ErrorObject{
			Code:    oapi.INTERNAL,
			Message: "dependency unhealthy: " + strings.Join(failed, ","),
		},
	})
}

// registerHealth mounts the spec-conformant GET /api/v1/healthz on the group.
// 200 + HealthResponse when DB and Redis ping; 503 + ErrorResponse otherwise,
// naming the failed dependency without leaking internal detail.
func registerHealth(rg gin.IRouter, dbPinger, redisPinger Pinger) {
	rg.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), depPingTimeout)
		defer cancel()

		dbOK, redisOK, failed := pingDeps(ctx, dbPinger, redisPinger)
		if len(failed) == 0 {
			httpx.OK(c, oapi.Health{Status: "ok", Db: &dbOK, Redis: &redisOK})
			return
		}
		unhealthy503(c, failed)
	})
}

// registerProbes mounts the Kubernetes probe endpoints at the ROOT (outside the
// spec's /api/v1 group), matching deploy/k8s/api-deployment.yaml:
//
//	GET /livez   liveness  — always 200; process-only, makes NO dependency calls,
//	                         so a transient DB/Redis blip never restarts the pod.
//	GET /readyz  readiness — 200 when DB+Redis ping, else 503; a failing dependency
//	                         pulls the pod from the Service until it recovers.
func registerProbes(r gin.IRouter, dbPinger, redisPinger Pinger) {
	r.GET("/livez", func(c *gin.Context) {
		httpx.OK(c, map[string]string{"status": "alive"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), depPingTimeout)
		defer cancel()

		dbOK, redisOK, failed := pingDeps(ctx, dbPinger, redisPinger)
		if len(failed) == 0 {
			httpx.OK(c, oapi.Health{Status: "ready", Db: &dbOK, Redis: &redisOK})
			return
		}
		unhealthy503(c, failed)
	})
}
