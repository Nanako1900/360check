//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/ewkb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	migrations "github.com/nnkglobal/c5-backend/db/migrations"
	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

// newPostGISDSN spins a fresh PostGIS container (no migrations) and returns its DSN.
func newPostGISDSN(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("c5"),
		tcpostgres.WithUsername("c5"),
		tcpostgres.WithPassword("c5"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

// newMigratedPool spins a PostGIS container, runs all migrations, and returns a
// connected pool plus the DSN.
func newMigratedPool(t *testing.T) (*Pool, string) {
	t.Helper()
	ctx := context.Background()

	dsn := newPostGISDSN(t)
	require.NoError(t, RunMigrations(dsn, migrations.FS))

	pool, err := New(ctx, dsn, 5, 1, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))
	return pool, dsn
}

// regclassExists reports whether a table exists, via a short-lived connection.
func regclassExists(t *testing.T, dsn, table string) bool {
	t.Helper()
	ctx := context.Background()
	pool, err := New(ctx, dsn, 2, 1, 30*time.Second)
	require.NoError(t, err)
	defer pool.Close()
	var reg *string
	require.NoError(t, pool.QueryRow(ctx, "SELECT to_regclass($1)::text", table).Scan(&reg))
	return reg != nil
}

func TestMigrations_DownThenUp(t *testing.T) {
	dsn := newPostGISDSN(t)

	m, err := newMigrator(dsn, migrations.FS)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = m.Close() })

	require.NoError(t, m.Up())
	assert.True(t, regclassExists(t, dsn, "users"), "users present after up")

	require.NoError(t, m.Down())
	assert.False(t, regclassExists(t, dsn, "users"), "users dropped after clean down")

	require.NoError(t, m.Up())
	assert.True(t, regclassExists(t, dsn, "users"), "users restored after re-up")
}

func TestMigrations_TablesAndIdempotentUp(t *testing.T) {
	pool, dsn := newMigratedPool(t)
	ctx := context.Background()

	// All 16 master tables present.
	for _, tbl := range []string{
		"users", "roles", "permissions", "user_roles", "casbin_rule",
		"dict_type", "dict_item", "app_config", "projects", "inspection_tasks",
		"inspections", "trajectory_points", "problems", "problem_processing_log",
		"media_assets", "export_jobs",
	} {
		var reg *string
		require.NoError(t, pool.QueryRow(ctx, "SELECT to_regclass($1)::text", tbl).Scan(&reg))
		require.NotNilf(t, reg, "table %s should exist", tbl)
	}

	// Seed (000002) applied: admin user + casbin policies present.
	var adminID int64
	require.NoError(t, pool.QueryRow(ctx, "SELECT id FROM users WHERE username='admin'").Scan(&adminID))
	assert.Positive(t, adminID)
	var pcount int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM casbin_rule WHERE ptype='p'").Scan(&pcount))
	assert.Positive(t, pcount)

	// Up again is idempotent (ErrNoChange swallowed).
	require.NoError(t, RunMigrations(dsn, migrations.FS))
}

func TestSRIDRejectAndMileageGeography(t *testing.T) {
	pool, _ := newMigratedPool(t)
	ctx := context.Background()

	var adminID int64
	require.NoError(t, pool.QueryRow(ctx, "SELECT id FROM users WHERE username='admin'").Scan(&adminID))

	var projectID int64
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO projects (code, name) VALUES ('P-GEO','geo test') RETURNING id`).Scan(&projectID))

	var inspID int64
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO inspections (project_id, inspector_id, started_at) VALUES ($1,$2, now()) RETURNING id`,
		projectID, adminID).Scan(&inspID))

	// Valid 4326 point inserts; round-trips through PostGIS via EWKB.
	p := orb.Point{113.0, 23.0}
	ewkb4326, err := geo.EncodePointWGS84(p)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO trajectory_points (inspection_id, seq, geom, recorded_at)
		 VALUES ($1, 1, ST_GeomFromEWKB($2), now())`, inspID, ewkb4326)
	require.NoError(t, err)

	var gotEWKB []byte
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT ST_AsEWKB(geom) FROM trajectory_points WHERE inspection_id=$1 AND seq=1`, inspID).Scan(&gotEWKB))
	gotPt, err := geo.DecodePoint(gotEWKB)
	require.NoError(t, err)
	assert.InDelta(t, p.Lon(), gotPt.Lon(), 1e-9)
	assert.InDelta(t, p.Lat(), gotPt.Lat(), 1e-9)

	// SRID != 4326 is rejected by the column typmod / CHECK.
	ewkb3857, err := ewkb.Marshal(orb.Point{12_900_000, 2_600_000}, 3857)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO trajectory_points (inspection_id, seq, geom, recorded_at)
		 VALUES ($1, 99, ST_GeomFromEWKB($2), now())`, inspID, ewkb3857)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SRID")

	// Second valid point 0.01deg east, same latitude.
	ewkb2, err := geo.EncodePointWGS84(orb.Point{113.01, 23.0})
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO trajectory_points (inspection_id, seq, geom, recorded_at)
		 VALUES ($1, 2, ST_GeomFromEWKB($2), now())`, inspID, ewkb2)
	require.NoError(t, err)

	// Mileage MUST use geography (meters); plain geometry length is degrees.
	var meters, degrees float64
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT ST_Length(ST_MakeLine(geom ORDER BY seq)::geography),
		       ST_Length(ST_MakeLine(geom ORDER BY seq))
		FROM trajectory_points WHERE inspection_id=$1 AND seq IN (1,2)`, inspID).Scan(&meters, &degrees))

	// Golden: ~1025.2 m (contract-validated) within 1%; degrees ~0.01 (NOT meters).
	assert.InEpsilon(t, 1025.2, meters, 0.01, "mileage via ::geography must be meters")
	assert.InDelta(t, 0.01, degrees, 1e-4, "plain geometry length is degrees, proving the geography cast is required")
}

func TestWithSavepoint_PerItemIsolation(t *testing.T) {
	pool, _ := newMigratedPool(t)
	ctx := context.Background()

	err := pool.WithTx(ctx, func(tx pgx.Tx) error {
		// Good item A.
		require.NoError(t, WithSavepoint(ctx, tx, func(sp pgx.Tx) error {
			_, e := sp.Exec(ctx, `INSERT INTO projects (code, name) VALUES ('SP-A','a')`)
			return e
		}))
		// Bad item: duplicate code -> CONFLICT, savepoint rolls back, outer tx survives.
		bad := WithSavepoint(ctx, tx, func(sp pgx.Tx) error {
			_, e := sp.Exec(ctx, `INSERT INTO projects (code, name) VALUES ('SP-A','dup')`)
			return e
		})
		require.Error(t, bad)
		// Good item B still commits on the same (still-alive) outer tx.
		require.NoError(t, WithSavepoint(ctx, tx, func(sp pgx.Tx) error {
			_, e := sp.Exec(ctx, `INSERT INTO projects (code, name) VALUES ('SP-B','b')`)
			return e
		}))
		return nil
	})
	require.NoError(t, err)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM projects WHERE code IN ('SP-A','SP-B')`).Scan(&count))
	assert.Equal(t, 2, count, "A and B persist; the duplicate was isolated to its savepoint")
}

func TestClientUUIDIdempotency(t *testing.T) {
	pool, _ := newMigratedPool(t)
	ctx := context.Background()

	var adminID, projectID, inspID int64
	require.NoError(t, pool.QueryRow(ctx, "SELECT id FROM users WHERE username='admin'").Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO projects (code, name) VALUES ('P-IDEM','idem') RETURNING id`).Scan(&projectID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO inspections (project_id, inspector_id, started_at) VALUES ($1,$2, now()) RETURNING id`,
		projectID, adminID).Scan(&inspID))

	pt, err := geo.EncodePointWGS84(orb.Point{113.0, 23.0})
	require.NoError(t, err)
	const cu = "11111111-1111-1111-1111-111111111111"

	ins := `INSERT INTO trajectory_points (client_uuid, inspection_id, seq, geom, recorded_at)
	        VALUES ($1, $2, 1, ST_GeomFromEWKB($3), now())
	        ON CONFLICT (client_uuid) DO NOTHING`

	tag1, err := pool.Exec(ctx, ins, cu, inspID, pt)
	require.NoError(t, err)
	assert.EqualValues(t, 1, tag1.RowsAffected())

	// Replay: same client_uuid -> 0 rows (idempotent).
	tag2, err := pool.Exec(ctx, ins, cu, inspID, pt)
	require.NoError(t, err)
	assert.EqualValues(t, 0, tag2.RowsAffected())
}

func TestValidateGeomWGS84(t *testing.T) {
	pool, _ := newMigratedPool(t)
	ctx := context.Background()

	// Valid 4326 point passes.
	ptEWKB, err := geo.EncodePointWGS84(orb.Point{113.0, 23.0})
	require.NoError(t, err)
	ok, reason, err := ValidateGeomWGS84(ctx, pool, ptEWKB)
	require.NoError(t, err)
	assert.True(t, ok, "valid 4326 point should pass; reason=%s", reason)

	// Self-intersecting ("bowtie") polygon is 4326 but topologically invalid.
	bowtie := orb.MultiPolygon{{orb.Ring{{0, 0}, {1, 1}, {1, 0}, {0, 1}, {0, 0}}}}
	bowtieEWKB, err := geo.EncodeMultiPolygonWGS84(bowtie)
	require.NoError(t, err)
	ok, reason, err = ValidateGeomWGS84(ctx, pool, bowtieEWKB)
	require.NoError(t, err)
	assert.False(t, ok, "self-intersecting polygon must be rejected by ST_IsValid")
	assert.NotEmpty(t, reason)

	// Wrong SRID is rejected.
	merc, err := ewkb.Marshal(orb.Point{12_900_000, 2_600_000}, 3857)
	require.NoError(t, err)
	ok, reason, err = ValidateGeomWGS84(ctx, pool, merc)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, reason, "SRID")
}
