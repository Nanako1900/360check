// Command c5-api is the gin HTTP server. It loads configuration (fail-fast),
// constructs the DB pool and Redis client, pings them at startup, and serves the
// API with graceful shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"golang.org/x/crypto/bcrypt"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/auth"
	"github.com/nnkglobal/c5-backend/internal/config"
	"github.com/nnkglobal/c5-backend/internal/dict"
	"github.com/nnkglobal/c5-backend/internal/export"
	gendb "github.com/nnkglobal/c5-backend/internal/gen/db"
	"github.com/nnkglobal/c5-backend/internal/inspection"
	"github.com/nnkglobal/c5-backend/internal/media"
	"github.com/nnkglobal/c5-backend/internal/platform/asynqx"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
	"github.com/nnkglobal/c5-backend/internal/platform/cosimpl"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/platform/jwt"
	"github.com/nnkglobal/c5-backend/internal/platform/obs"
	"github.com/nnkglobal/c5-backend/internal/platform/redis"
	"github.com/nnkglobal/c5-backend/internal/platform/sts"
	"github.com/nnkglobal/c5-backend/internal/platform/stsimpl"
	"github.com/nnkglobal/c5-backend/internal/problem"
	"github.com/nnkglobal/c5-backend/internal/project"
	"github.com/nnkglobal/c5-backend/internal/rbac"
	"github.com/nnkglobal/c5-backend/internal/server"
	"github.com/nnkglobal/c5-backend/internal/server/middleware"
	"github.com/nnkglobal/c5-backend/internal/stats"
	syncd "github.com/nnkglobal/c5-backend/internal/sync"
	"github.com/nnkglobal/c5-backend/internal/user"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// `c5-api migrate` applies pending migrations and exits — invoked by the
	// deploy/k8s migrate Job before the api/worker rollout.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := runMigrate(logger); err != nil {
			logger.Error("migrate failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}

	// `c5-api create-admin` sets the bootstrap admin's password from a deploy
	// secret (the seed ships the admin LOCKED — no default credential). One-shot
	// Job after migrate.
	if len(os.Args) > 1 && os.Args[1] == "create-admin" {
		if err := runCreateAdmin(logger); err != nil {
			logger.Error("create-admin failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}

	// `c5-api healthcheck` GETs the local /livez probe and exits 0 on a 2xx, else 1.
	// Lets the distroless runtime image (no curl/wget) self-report health for the
	// Dockerfile HEALTHCHECK / Coolify without shipping an extra tool.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	if err := run(logger); err != nil {
		logger.Error("fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// runMigrate loads config and applies all pending golang-migrate migrations
// (embedded), then returns. Safe to re-run: golang-migrate takes a Postgres
// advisory lock and is idempotent.
func runMigrate(logger *slog.Logger) error {
	// migrate needs ONLY the database URL — not Redis/JWT — so it reads C5_DB_DSN
	// directly instead of the full fail-fast config.Load(). This lets the one-shot
	// compose / k8s migrate container carry just C5_DB_DSN (config.Load would reject
	// it for missing C5_REDIS_ADDR / C5_JWT_SECRET it never uses).
	dsn := os.Getenv("C5_DB_DSN")
	if dsn == "" {
		return fmt.Errorf("C5_DB_DSN is required to run migrations")
	}
	logger.Info("running database migrations")
	if err := db.RunMigrations(dsn, migrations.FS); err != nil {
		return err
	}
	logger.Info("database migrations applied")
	return nil
}

// runCreateAdmin sets the bootstrap admin's password from C5_BOOTSTRAP_ADMIN_PASSWORD
// (+ optional C5_BOOTSTRAP_ADMIN_USERNAME, default "admin"), bcrypt-hashed at cost
// 12. Invoked as `c5-api create-admin` (one-shot Job after migrate) so the admin —
// seeded with a LOCKED password — becomes loginable via a deploy secret rather than
// a baked-in default credential. Idempotent: re-running re-sets the password.
func runCreateAdmin(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	username := os.Getenv("C5_BOOTSTRAP_ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("C5_BOOTSTRAP_ADMIN_PASSWORD")
	if l := len(password); l < 8 || l > 72 {
		return fmt.Errorf("C5_BOOTSTRAP_ADMIN_PASSWORD must be set (8-72 bytes); got %d", l)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.New(ctx, cfg.DB.DSN, cfg.DB.MaxConns, cfg.DB.MinConns, cfg.DB.HealthCheck)
	if err != nil {
		return err
	}
	defer pool.Close()
	queries := gendb.New(pool.Pool)

	row, err := queries.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("admin user %q not found (run migrate first): %w", username, err)
	}
	// bcrypt cost 12 matches the app's password policy (auth.BcryptCost).
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	n, err := queries.UpdateUserPassword(ctx, gendb.UpdateUserPasswordParams{
		ID:           row.ID,
		PasswordHash: string(hash),
		UpdatedBy:    &row.ID,
	})
	if err != nil {
		return fmt.Errorf("update admin password: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("admin user %q not updated", username)
	}
	logger.Info("bootstrap admin password set", slog.String("username", username))
	return nil
}

// runHealthcheck probes http://127.0.0.1:$C5_SERVER_PORT/livez and returns 0 on a
// 2xx response, else 1. Used by the Dockerfile HEALTHCHECK so the distroless image
// needs no curl/wget. It reads the port straight from env (no config.Load) so it
// stays dependency-free and fast.
func runHealthcheck() int {
	port := os.Getenv("C5_SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/livez")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 0
	}
	fmt.Fprintf(os.Stderr, "healthcheck: unexpected status %d\n", resp.StatusCode)
	return 1
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// OpenTelemetry tracing (no-op unless C5_OBSERV_OTLP_ENDPOINT is set). otelgin
	// then exports real spans to the in-cluster collector.
	traceShutdown, err := obs.SetupTracing(context.Background(), cfg.Observ.ServiceName, cfg.Env, cfg.Observ.OTLPEndpoint)
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
	// Each dependency gets its own ping budget so a slow DB cannot starve the
	// Redis check (and vice versa).
	if err := pingWithTimeout(pool.Ping, 5*time.Second); err != nil {
		return err
	}

	rdb := redis.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	defer func() { _ = rdb.Close() }()
	if err := pingWithTimeout(rdb.Ping, 5*time.Second); err != nil {
		return err
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// Auth / RBAC stack.
	enforcer, err := rbac.NewEnforcer(pool, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		return err
	}
	queries := gendb.New(pool.Pool)
	jwtMgr := jwt.NewManager(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.AccessTTL)
	refreshStore := auth.NewRefreshStore(rdb.Client, cfg.JWT.RefreshTTL)
	authHandler := auth.NewHandler(auth.NewService(queries, jwtMgr, refreshStore, enforcer))
	userHandler := user.NewHandler(user.NewService(queries))
	rbacHandler := rbac.NewHandler(rbac.NewService(queries, enforcer))

	// P5/P6: media (COS STS + confirm), offline sync, stats (D2) and async export.
	// Media confirm + export creation enqueue asynq tasks processed by cmd/worker.
	asynqClient := asynqx.NewClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	defer func() { _ = asynqClient.Close() }()

	cosClient, stsIssuer, err := mediaClients(cfg, logger)
	if err != nil {
		return err
	}
	mediaHandler := media.NewHandler(
		media.NewService(pool, cosClient, stsIssuer, media.Config{
			BucketOriginal: cfg.COS.BucketOriginal,
			BucketWeb:      cfg.COS.BucketWeb,
			BucketThumb:    cfg.COS.BucketThumb,
			Region:         cfg.COS.Region,
		}),
		asynqClient,
	)
	syncHandler := syncd.NewHandler(syncd.NewService(pool))
	statsHandler := stats.NewHandler(stats.NewService(pool))
	exportHandler := export.NewHandler(export.NewService(pool, asynqClient), cosClient)

	engine := server.New(server.Deps{
		Cfg:             cfg,
		Logger:          logger,
		DB:              pool,
		Redis:           rdb,
		Registry:        reg,
		JWT:             jwtMgr,
		Enforcer:        enforcer,
		AuthRateLimiter: middleware.AuthRateLimit(rdb.Client, middleware.DefaultAuthRateLimit()),
		PublicRoutes:    []server.RouteRegistrar{authHandler.RegisterPublic},
		ProtectedRoutes: []server.RouteRegistrar{
			authHandler.RegisterProtected,
			userHandler.RegisterRoutes,
			rbacHandler.RegisterRoutes,
			project.NewHandler(project.NewService(pool)).RegisterRoutes,
			inspection.NewHandler(inspection.NewService(pool)).RegisterRoutes,
			dict.NewHandler(dict.NewService(pool)).RegisterRoutes,
			problem.NewHandler(problem.NewService(pool), pool).RegisterRoutes,
			mediaHandler.RegisterRoutes,
			syncHandler.RegisterRoutes,
			statsHandler.RegisterRoutes,
			exportHandler.RegisterRoutes,
		},
	})

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Server.Port),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("c5-api listening",
			slog.Int("port", cfg.Server.Port),
			slog.String("env", cfg.Env),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case <-stop:
		logger.Info("shutdown signal received")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("c5-api stopped cleanly")
	return nil
}

// pingWithTimeout runs a dependency ping under its own bounded context.
func pingWithTimeout(ping func(context.Context) error, d time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return ping(ctx)
}

// mediaClients returns the real Tencent COS client + CAM STS issuer when COS
// credentials are configured, else in-memory mocks so the api boots in dev/test
// without cloud access (media upload-credentials/confirm then exercise the mock
// path). The STS issuer additionally requires the COS APPID (C5_COS_APP_ID).
func mediaClients(cfg *config.Config, logger *slog.Logger) (cos.Client, sts.Issuer, error) {
	if cfg.COS.SecretID == "" || cfg.COS.SecretKey == "" {
		logger.Warn("COS credentials absent — media uses in-memory COS/STS mocks")
		return cos.NewMock(), sts.NewMock(), nil
	}
	cosClient, err := cosimpl.New(cfg.COS.SecretID, cfg.COS.SecretKey, cfg.COS.Region)
	if err != nil {
		return nil, nil, err
	}
	stsIssuer, err := stsimpl.New(cfg.COS.SecretID, cfg.COS.SecretKey, cfg.COS.AppID)
	if err != nil {
		return nil, nil, err
	}
	return cosClient, stsIssuer, nil
}
