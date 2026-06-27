package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nnkglobal/c5-backend/internal/platform/geo"
)

// rowQuerier is the minimal query surface shared by *Pool and pgx.Tx, so geometry
// validation works inside or outside a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ValidateGeomWGS84 checks, via PostGIS, that an EWKB blob decodes to a geometry
// that is both SRID 4326 and topologically valid (ST_IsValid — catches e.g.
// self-intersecting polygon rings that the column SRID CHECK alone would accept).
// Service write paths call this before insert and return VALIDATION_FAILED when
// it reports invalid. It returns (valid, reason); reason is a human-readable
// explanation when valid is false.
func ValidateGeomWGS84(ctx context.Context, q rowQuerier, ewkb []byte) (bool, string, error) {
	var (
		valid  bool
		srid   int
		reason *string
	)
	err := q.QueryRow(ctx, `
		SELECT ST_IsValid(g), ST_SRID(g), ST_IsValidReason(g)
		FROM (SELECT ST_GeomFromEWKB($1) AS g) t`, ewkb).Scan(&valid, &srid, &reason)
	if err != nil {
		return false, "", fmt.Errorf("validate geom: %w", err)
	}
	if srid != geo.SRID4326 {
		return false, fmt.Sprintf("SRID %d != 4326", srid), nil
	}
	if !valid {
		r := "invalid geometry"
		if reason != nil && *reason != "" {
			r = *reason
		}
		return false, r, nil
	}
	return true, "", nil
}
