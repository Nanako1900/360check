package db

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the "pgx5" scheme
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// newMigrator builds a golang-migrate instance over the embedded SQL source and
// the pgx/v5 database driver. Callers must Close() the returned instance.
func newMigrator(dsn string, migrationsFS fs.FS) (*migrate.Migrate, error) {
	src, err := iofs.New(migrationsFS, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: open iofs source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toMigrateDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("migrate: init: %w", err)
	}
	return m, nil
}

// RunMigrations applies all up migrations from migrationsFS against the DSN.
// It is idempotent: an already-migrated database returns nil (ErrNoChange).
//
// Production runs this as a dedicated serial Job (initContainer / K8s Job); the
// application processes never auto-migrate, to avoid multi-replica races.
func RunMigrations(dsn string, migrationsFS fs.FS) error {
	m, err := newMigrator(dsn, migrationsFS)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// toMigrateDSN normalizes a libpq/pgx DSN to the golang-migrate pgx/v5 scheme
// ("pgx5://"), under which the pgx/v5 database driver is registered.
func toMigrateDSN(dsn string) string {
	switch {
	case strings.HasPrefix(dsn, "postgresql://"):
		return "pgx5://" + strings.TrimPrefix(dsn, "postgresql://")
	case strings.HasPrefix(dsn, "postgres://"):
		return "pgx5://" + strings.TrimPrefix(dsn, "postgres://")
	default:
		return dsn
	}
}
