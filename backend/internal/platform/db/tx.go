package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// WithTx runs fn inside a transaction, committing on success and rolling back on
// error or panic. fn must perform all queries via the provided pgx.Tx.
func (p *Pool) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := p.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Rollback is a no-op after a successful Commit; this also unwinds on panic.
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// WithSavepoint runs fn inside a nested transaction (SAVEPOINT) on an existing
// tx. On error it rolls back TO the savepoint and returns the error, leaving the
// outer tx usable — the per-item isolation /sync/batch needs so one bad item
// never aborts the whole batch. pgx issues SAVEPOINT / RELEASE / ROLLBACK TO
// automatically for a nested Begin/Commit/Rollback.
//
// Panics are intentionally NOT recovered here: a panic propagates up to WithTx,
// whose deferred Rollback collapses the whole outer transaction (no half-applied
// batch). The per-item isolation contract is about returned errors, not panics.
func WithSavepoint(ctx context.Context, tx pgx.Tx, fn func(pgx.Tx) error) error {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin savepoint: %w", err)
	}
	if err := fn(sp); err != nil {
		_ = sp.Rollback(ctx) // ROLLBACK TO SAVEPOINT — outer tx stays alive
		return err
	}
	if err := sp.Commit(ctx); err != nil { // RELEASE SAVEPOINT
		return fmt.Errorf("release savepoint: %w", err)
	}
	return nil
}
