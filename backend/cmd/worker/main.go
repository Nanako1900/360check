// Command c5-worker runs the asynq background processor for C5: it derives media
// tiers (web/thumb) from confirmed originals, runs Excel exports, and runs a
// periodic reaper that flags stale UPLOADING media rows. asynq exposes no HTTP of
// its own, so the worker also serves a small :9091 surface for
// liveness/readiness/metrics (probed by Kubernetes — see deploy/k8s).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/nnkglobal/c5-backend/internal/config"
	"github.com/nnkglobal/c5-backend/internal/export"
	"github.com/nnkglobal/c5-backend/internal/media"
	"github.com/nnkglobal/c5-backend/internal/platform/asynqx"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
	"github.com/nnkglobal/c5-backend/internal/platform/cosimpl"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/platform/obs"
	"github.com/nnkglobal/c5-backend/internal/platform/redis"
	"github.com/nnkglobal/c5-backend/internal/stats"
)

const (
	// reapInterval is how often the UPLOADING reaper tick is enqueued.
	reapInterval = "@every 1h"
	// uploadingTTL is how long a media row may sit in UPLOADING before the reaper
	// flags it stale (matches media.NewReaper's default; no config knob yet).
	uploadingTTL = 24 * time.Hour
	// workerMetricsAddr is the worker's health/metrics HTTP surface.
	workerMetricsAddr = ":9091"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// `c5-worker healthcheck` GETs the local :9091/livez and exits 0 on 2xx, else 1.
	// Used by the Dockerfile HEALTHCHECK so Coolify tracks worker health like every
	// other stack service (parity with c5-api; no curl/wget needed).
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	if err := run(logger); err != nil {
		logger.Error("fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// OpenTelemetry tracing (no-op unless C5_OBSERV_OTLP_ENDPOINT is set).
	traceShutdown, err := obs.SetupTracing(context.Background(), "c5-worker", cfg.Env, cfg.Observ.OTLPEndpoint)
	if err != nil {
		return err
	}
	defer func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()
		_ = traceShutdown(flushCtx)
	}()

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.New(startupCtx, cfg.DB.DSN, cfg.DB.MaxConns, cfg.DB.MinConns, cfg.DB.HealthCheck)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pingWithTimeout(pool.Ping, 5*time.Second); err != nil {
		return err
	}

	rdb := redis.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	defer func() { _ = rdb.Close() }()
	if err := pingWithTimeout(rdb.Ping, 5*time.Second); err != nil {
		return err
	}

	cosClient, err := buildCOS(cfg, logger)
	if err != nil {
		return err
	}

	mediaCfg := media.Config{
		BucketOriginal: cfg.COS.BucketOriginal,
		BucketWeb:      cfg.COS.BucketWeb,
		BucketThumb:    cfg.COS.BucketThumb,
		Region:         cfg.COS.Region,
		UploadingTTL:   uploadingTTL,
	}

	// One ServeMux aggregates every domain's asynq handlers.
	mux := asynq.NewServeMux()
	media.RegisterWorkers(mux, pool, cosClient, mediaCfg)
	media.RegisterReaperWorker(mux, pool, uploadingTTL)
	// Exported .xlsx artifacts land in the (private) original bucket under the
	// export key prefix; the api signs result_url from the recorded result_bucket.
	export.RegisterWorkersWithBucket(mux, cosClient, pool, stats.NewService(pool), cfg.COS.BucketOriginal)

	srv := asynqx.NewServer(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, 0)

	// Periodic scheduler enqueues the reaper tick; the handler above processes it.
	scheduler := asynqx.NewScheduler(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if _, err := scheduler.Register(reapInterval, media.NewReapTask()); err != nil {
		return err
	}

	// Prometheus registry + health/metrics HTTP surface (asynq has no HTTP).
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	health := obs.NewHealthServer(workerMetricsAddr, reg,
		obs.ReadyCheck{Name: "postgres", Check: pool.Ping},
		obs.ReadyCheck{Name: "redis", Check: rdb.Ping},
	)

	runErr := make(chan error, 1)
	go func() {
		if err := health.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr <- err
		}
	}()
	if err := srv.Start(mux); err != nil {
		return err
	}
	if err := scheduler.Start(); err != nil {
		return err
	}
	logger.Info("c5-worker started",
		slog.String("env", cfg.Env),
		slog.String("metrics_addr", workerMetricsAddr),
	)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-runErr:
		return err
	case <-stop:
		logger.Info("shutdown signal received")
	}

	// Graceful: stop scheduling, drain in-flight tasks, then close the http surface.
	scheduler.Shutdown()
	srv.Shutdown()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()
	if err := health.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("c5-worker stopped cleanly")
	return nil
}

// buildCOS returns the real Tencent COS client when credentials are configured,
// else an in-memory mock so the worker boots in dev without cloud access.
func buildCOS(cfg *config.Config, logger *slog.Logger) (cos.Client, error) {
	if cfg.COS.SecretID == "" || cfg.COS.SecretKey == "" {
		logger.Warn("COS credentials absent — using in-memory mock (no real uploads)")
		return cos.NewMock(), nil
	}
	return cosimpl.New(cfg.COS.SecretID, cfg.COS.SecretKey, cfg.COS.Region)
}

// pingWithTimeout runs a dependency ping under its own bounded context.
func pingWithTimeout(ping func(context.Context) error, d time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return ping(ctx)
}

// runHealthcheck probes the worker's internal liveness endpoint (workerMetricsAddr
// /livez) and returns 0 on a 2xx, else 1. Invoked as `c5-worker healthcheck` by the
// Dockerfile HEALTHCHECK — self-contained so the image needs no curl/wget, mirroring
// c5-api so Coolify reports worker health consistently across the stack.
func runHealthcheck() int {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + workerMetricsAddr + "/livez")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 0
	}
	_, _ = fmt.Fprintf(os.Stderr, "healthcheck: unexpected status %d\n", resp.StatusCode)
	return 1
}
