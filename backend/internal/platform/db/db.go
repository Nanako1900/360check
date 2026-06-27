// Package db provides the pgx connection pool and transaction helpers used by
// every repository. Geometry columns are read/written as EWKB bytea at the SQL
// boundary (see internal/platform/geo); pgx never sees PostGIS types directly.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps a pgxpool.Pool with C5 defaults and helpers.
type Pool struct {
	*pgxpool.Pool
}

// New constructs a pgxpool from a DSN with bounded connections and health checks.
// It does not verify connectivity; callers should Ping at startup to fail fast.
func New(ctx context.Context, dsn string, maxConns, minConns int32, healthCheck time.Duration) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse db dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	if minConns > 0 {
		cfg.MinConns = minConns
	}
	if healthCheck > 0 {
		cfg.HealthCheckPeriod = healthCheck
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create db pool: %w", err)
	}
	return &Pool{Pool: pool}, nil
}

// Ping verifies database connectivity (used by /healthz and /readyz).
func (p *Pool) Ping(ctx context.Context) error {
	if err := p.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}
	return nil
}
