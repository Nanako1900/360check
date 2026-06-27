package dict

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// repo is the hand-written pgx data-access layer for dict_type, dict_item and
// app_config. Every read filters deleted_at IS NULL where the column exists
// (dict_type / dict_item; app_config has no soft delete — its history is the
// inactive rows). All item mutations recompute the owning type's content_hash in
// the same transaction.
type repo struct {
	pool *db.Pool
}

func newRepo(pool *db.Pool) *repo { return &repo{pool: pool} }

// --- dict_type --------------------------------------------------------------

// dictTypeColumns is the canonical SELECT projection for a dict_type row.
const dictTypeColumns = `
	id, code, name, scope, description, version, content_hash, is_active,
	created_at, updated_at`

// dictTypeRow is the raw scan target for a dict_type SELECT.
type dictTypeRow struct {
	id          int64
	code        string
	name        string
	scope       string
	description string
	version     int32
	contentHash string
	isActive    bool
	createdAt   pgtype.Timestamptz
	updatedAt   pgtype.Timestamptz
}

// scanDictType reads one dictTypeRow from a pgx.Row in dictTypeColumns order.
func scanDictType(row pgx.Row) (*dictTypeRow, error) {
	var r dictTypeRow
	err := row.Scan(
		&r.id, &r.code, &r.name, &r.scope, &r.description,
		&r.version, &r.contentHash, &r.isActive,
		&r.createdAt, &r.updatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// toDictType converts a scanned row into the API model.
func (r *dictTypeRow) toDictType() oapi.DictType {
	desc := r.description
	created := r.createdAt.Time
	updated := r.updatedAt.Time
	return oapi.DictType{
		Id:          r.id,
		Code:        r.code,
		Name:        r.name,
		Scope:       oapi.DictScope(r.scope),
		Description: &desc,
		Version:     int(r.version),
		ContentHash: r.contentHash,
		IsActive:    r.isActive,
		CreatedAt:   &created,
		UpdatedAt:   &updated,
	}
}

// createDictTypeArgs carries the validated inputs for a dict_type insert.
type createDictTypeArgs struct {
	code        string
	name        string
	scope       string
	description string
	createdBy   *int64
}

// createDictType inserts a dict_type (empty item set => content_hash over no
// items) and returns the created row.
func (rp *repo) createDictType(ctx context.Context, a createDictTypeArgs) (*oapi.DictType, error) {
	const sql = `
		INSERT INTO dict_type
			(code, name, scope, description, version, content_hash, is_active, created_by, updated_by)
		VALUES
			($1, $2, $3, $4, 1, $5, TRUE, $6, $6)
		RETURNING` + dictTypeColumns
	// A brand-new type has no items; its hash is the empty-set hash.
	emptyHash := hashItems(nil)
	row := rp.pool.QueryRow(ctx, sql, a.code, a.name, a.scope, a.description, emptyHash, a.createdBy)
	tr, err := scanDictType(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCodeTaken
		}
		return nil, fmt.Errorf("dict: insert type: %w", err)
	}
	dt := tr.toDictType()
	return &dt, nil
}

// getDictTypeByCode returns a single non-deleted dict_type by its machine code.
func (rp *repo) getDictTypeByCode(ctx context.Context, code string) (*oapi.DictType, error) {
	const sql = `SELECT` + dictTypeColumns + `
		FROM dict_type WHERE code = $1 AND deleted_at IS NULL`
	tr, err := scanDictType(rp.pool.QueryRow(ctx, sql, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("dict: get type by code: %w", err)
	}
	dt := tr.toDictType()
	return &dt, nil
}

// listDictTypesFilter holds the normalized dict_type list filters.
type listDictTypesFilter struct {
	scope    *string
	isActive *bool
}

// listDictTypes returns all non-deleted dict_types matching the filter, ordered
// by code (this catalog is small and admin-facing, so it is unpaginated).
func (rp *repo) listDictTypes(ctx context.Context, f listDictTypesFilter) ([]oapi.DictType, error) {
	conds := []string{"deleted_at IS NULL"}
	var args []any
	if f.scope != nil && *f.scope != "" {
		args = append(args, *f.scope)
		conds = append(conds, fmt.Sprintf("scope = $%d", len(args)))
	}
	if f.isActive != nil {
		args = append(args, *f.isActive)
		conds = append(conds, fmt.Sprintf("is_active = $%d", len(args)))
	}
	sql := `SELECT` + dictTypeColumns + ` FROM dict_type WHERE ` +
		strings.Join(conds, " AND ") + ` ORDER BY code ASC`

	rows, err := rp.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("dict: list types: %w", err)
	}
	defer rows.Close()

	out := make([]oapi.DictType, 0)
	for rows.Next() {
		tr, serr := scanDictType(rows)
		if serr != nil {
			return nil, fmt.Errorf("dict: list types scan: %w", serr)
		}
		out = append(out, tr.toDictType())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dict: list types rows: %w", err)
	}
	return out, nil
}

// updateDictTypeArgs carries a partial dict_type update; nil fields are unchanged.
type updateDictTypeArgs struct {
	id          int64
	name        *string
	description *string
	isActive    *bool
	updatedBy   *int64
}

// updateDictType applies a partial update (name/description/is_active) and
// returns the updated row. The item set is untouched, so content_hash/version do
// not move here. A missing/deleted target yields ErrNotFound.
func (rp *repo) updateDictType(ctx context.Context, a updateDictTypeArgs) (*oapi.DictType, error) {
	const sql = `
		UPDATE dict_type SET
			name        = COALESCE($2, name),
			description = COALESCE($3, description),
			is_active   = COALESCE($4, is_active),
			updated_by  = $5
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING` + dictTypeColumns
	row := rp.pool.QueryRow(ctx, sql, a.id, a.name, a.description, a.isActive, a.updatedBy)
	tr, err := scanDictType(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("dict: update type: %w", err)
	}
	dt := tr.toDictType()
	return &dt, nil
}

// softDeleteDictType marks a dict_type deleted. Returns ErrNotFound when no live
// row matched.
func (rp *repo) softDeleteDictType(ctx context.Context, id int64, by *int64) error {
	const sql = `
		UPDATE dict_type SET deleted_at = now(), is_active = FALSE, updated_by = $2
		WHERE id = $1 AND deleted_at IS NULL`
	tag, err := rp.pool.Exec(ctx, sql, id, by)
	if err != nil {
		return fmt.Errorf("dict: soft delete type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- dict_item --------------------------------------------------------------

// dictItemColumns is the canonical SELECT projection for a dict_item row.
const dictItemColumns = `
	id, dict_type_id, code, label, color, extra, sort_order, is_active,
	created_at, updated_at`

// dictItemRow is the raw scan target for a dict_item SELECT.
type dictItemRow struct {
	id         int64
	dictTypeID int64
	code       string
	label      string
	color      *string
	extra      []byte
	sortOrder  int32
	isActive   bool
	createdAt  pgtype.Timestamptz
	updatedAt  pgtype.Timestamptz
}

// scanDictItem reads one dictItemRow from a pgx.Row in dictItemColumns order.
func scanDictItem(row pgx.Row) (*dictItemRow, error) {
	var r dictItemRow
	err := row.Scan(
		&r.id, &r.dictTypeID, &r.code, &r.label, &r.color, &r.extra,
		&r.sortOrder, &r.isActive, &r.createdAt, &r.updatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// toDictItem converts a scanned row into the API model, decoding extra JSON.
func (r *dictItemRow) toDictItem() (*oapi.DictItem, error) {
	extra, err := decodeExtra(r.extra)
	if err != nil {
		return nil, err
	}
	created := r.createdAt.Time
	updated := r.updatedAt.Time
	var color *string
	if r.color != nil {
		color = colorPtr(*r.color)
	}
	return &oapi.DictItem{
		Id:         r.id,
		DictTypeId: r.dictTypeID,
		Code:       r.code,
		Label:      r.label,
		Color:      color,
		Extra:      extra,
		SortOrder:  int(r.sortOrder),
		IsActive:   r.isActive,
		CreatedAt:  &created,
		UpdatedAt:  &updated,
	}, nil
}

// listItemsByTypeID returns a type's items ordered by (sort_order, id). When
// includeInactive is false, retired (is_active=false) items are excluded.
// Soft-deleted items are always excluded.
func (rp *repo) listItemsByTypeID(ctx context.Context, q queryer, typeID int64, includeInactive bool) ([]oapi.DictItem, error) {
	sql := `SELECT` + dictItemColumns + `
		FROM dict_item
		WHERE dict_type_id = $1 AND deleted_at IS NULL`
	if !includeInactive {
		sql += ` AND is_active = TRUE`
	}
	sql += ` ORDER BY sort_order ASC, id ASC`

	rows, err := q.Query(ctx, sql, typeID)
	if err != nil {
		return nil, fmt.Errorf("dict: list items: %w", err)
	}
	defer rows.Close()

	out := make([]oapi.DictItem, 0)
	for rows.Next() {
		ir, serr := scanDictItem(rows)
		if serr != nil {
			return nil, fmt.Errorf("dict: list items scan: %w", serr)
		}
		item, cerr := ir.toDictItem()
		if cerr != nil {
			return nil, cerr
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dict: list items rows: %w", err)
	}
	return out, nil
}

// getItemTypeID returns the owning dict_type_id of a live dict_item, mapping a
// missing/deleted item to ErrNotFound. Used to locate the type whose hash must
// be recomputed after an item mutation.
func (rp *repo) getItemTypeID(ctx context.Context, q queryer, itemID int64) (int64, error) {
	var typeID int64
	err := q.QueryRow(ctx,
		`SELECT dict_type_id FROM dict_item WHERE id = $1 AND deleted_at IS NULL`, itemID).Scan(&typeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("dict: get item type id: %w", err)
	}
	return typeID, nil
}

// createItemArgs carries the validated, encoded inputs for a dict_item insert.
type createItemArgs struct {
	dictTypeID int64
	code       string
	label      string
	color      *string // nil => SQL NULL
	extra      []byte
	sortOrder  int
	createdBy  *int64
}

// createItem inserts a dict_item on the given queryer (a tx) and returns it.
func (rp *repo) createItem(ctx context.Context, q queryer, a createItemArgs) (*oapi.DictItem, error) {
	const sql = `
		INSERT INTO dict_item
			(dict_type_id, code, label, color, extra, sort_order, is_active, created_by, updated_by)
		VALUES
			($1, $2, $3, $4, $5, $6, TRUE, $7, $7)
		RETURNING` + dictItemColumns
	row := q.QueryRow(ctx, sql,
		a.dictTypeID, a.code, a.label, a.color, a.extra, a.sortOrder, a.createdBy)
	ir, err := scanDictItem(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrItemCodeTaken
		}
		if isForeignKeyViolation(err) {
			return nil, ErrNotFound // unknown dict_type_id
		}
		return nil, fmt.Errorf("dict: insert item: %w", err)
	}
	return ir.toDictItem()
}

// updateItemArgs carries a partial dict_item update; nil flags leave the column
// unchanged. setColor distinguishes "set color to NULL" from "leave color".
type updateItemArgs struct {
	id        int64
	label     *string
	setColor  bool
	color     *string
	extra     []byte // nil => leave unchanged
	sortOrder *int
	isActive  *bool // false retires the item (never hard-deletes)
	updatedBy *int64
}

// updateItem applies a partial update on the given queryer (a tx) and returns
// the updated row. is_active=false retires the item; the row stays referencable.
func (rp *repo) updateItem(ctx context.Context, q queryer, a updateItemArgs) (*oapi.DictItem, error) {
	const sql = `
		UPDATE dict_item SET
			label      = COALESCE($2, label),
			color      = CASE WHEN $3 THEN $4 ELSE color END,
			extra      = COALESCE($5, extra),
			sort_order = COALESCE($6, sort_order),
			is_active  = COALESCE($7, is_active),
			updated_by = $8
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING` + dictItemColumns
	var sortOrder *int32
	if a.sortOrder != nil {
		v := int32(*a.sortOrder)
		sortOrder = &v
	}
	row := q.QueryRow(ctx, sql,
		a.id, a.label, a.setColor, a.color, a.extra, sortOrder, a.isActive, a.updatedBy)
	ir, err := scanDictItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrItemCodeTaken
		}
		return nil, fmt.Errorf("dict: update item: %w", err)
	}
	return ir.toDictItem()
}

// softDeleteItem soft-deletes a dict_item (deleted_at) on the given queryer. A
// referenced item cannot be hard-deleted, so callers must guard with
// itemIsReferenced first and retire (is_active=false) instead. Returns
// ErrNotFound when no live row matched.
func (rp *repo) softDeleteItem(ctx context.Context, q queryer, id int64, by *int64) error {
	const sql = `
		UPDATE dict_item SET deleted_at = now(), is_active = FALSE, updated_by = $2
		WHERE id = $1 AND deleted_at IS NULL`
	tag, err := q.Exec(ctx, sql, id, by)
	if err != nil {
		return fmt.Errorf("dict: soft delete item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// itemIsReferenced reports whether any problems row still points at the item via
// type/status/category soft FK (ON DELETE RESTRICT). A referenced item must be
// retired, never deleted.
func (rp *repo) itemIsReferenced(ctx context.Context, q queryer, itemID int64) (bool, error) {
	const sql = `
		SELECT EXISTS (
			SELECT 1 FROM problems
			WHERE deleted_at IS NULL
			  AND (type_item_id = $1 OR status_item_id = $1 OR category_item_id = $1)
		)`
	var referenced bool
	if err := q.QueryRow(ctx, sql, itemID).Scan(&referenced); err != nil {
		return false, fmt.Errorf("dict: item reference check: %w", err)
	}
	return referenced, nil
}

// recomputeTypeHash recomputes a dict_type's content_hash over its full
// (id-ordered) item set and bumps its version, on the given queryer (a tx). It
// runs in the SAME transaction as the item mutation so the stored hash and the
// item set never diverge. Returns the new version + hash.
func (rp *repo) recomputeTypeHash(ctx context.Context, q queryer, typeID int64) (version int, hash string, err error) {
	const sql = `SELECT id, code, label, color, sort_order, is_active
		FROM dict_item
		WHERE dict_type_id = $1 AND deleted_at IS NULL
		ORDER BY id ASC`
	rows, qerr := q.Query(ctx, sql, typeID)
	if qerr != nil {
		return 0, "", fmt.Errorf("dict: load items for hash: %w", qerr)
	}
	defer rows.Close()

	var items []hashItemRow
	for rows.Next() {
		var r hashItemRow
		var color *string
		if serr := rows.Scan(&r.id, &r.code, &r.label, &color, &r.sortOrder, &r.isActive); serr != nil {
			return 0, "", fmt.Errorf("dict: scan item for hash: %w", serr)
		}
		if color != nil {
			r.color = *color
		}
		items = append(items, r)
	}
	if rerr := rows.Err(); rerr != nil {
		return 0, "", fmt.Errorf("dict: hash rows: %w", rerr)
	}
	rows.Close()

	newHash := hashItems(items)
	const upd = `
		UPDATE dict_type
		SET version = version + 1, content_hash = $2
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING version, content_hash`
	if err := q.QueryRow(ctx, upd, typeID, newHash).Scan(&version, &hash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", ErrNotFound
		}
		return 0, "", fmt.Errorf("dict: bump type hash: %w", err)
	}
	return version, hash, nil
}

// --- app_config -------------------------------------------------------------

// appConfigColumns is the canonical SELECT projection for an app_config row.
const appConfigColumns = `
	id, config_key, value, version, content_hash, is_active,
	effective_from, effective_to, description, created_at, updated_at`

// appConfigRow is the raw scan target for an app_config SELECT.
type appConfigRow struct {
	id            int64
	configKey     string
	value         []byte
	version       int32
	contentHash   string
	isActive      bool
	effectiveFrom pgtype.Timestamptz
	effectiveTo   pgtype.Timestamptz
	description   string
	createdAt     pgtype.Timestamptz
	updatedAt     pgtype.Timestamptz
}

// scanAppConfig reads one appConfigRow from a pgx.Row in appConfigColumns order.
func scanAppConfig(row pgx.Row) (*appConfigRow, error) {
	var r appConfigRow
	err := row.Scan(
		&r.id, &r.configKey, &r.value, &r.version, &r.contentHash, &r.isActive,
		&r.effectiveFrom, &r.effectiveTo, &r.description, &r.createdAt, &r.updatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// toAppConfig converts a scanned row into the API model, decoding the value.
func (r *appConfigRow) toAppConfig() (*oapi.AppConfig, error) {
	value, err := decodeValue(r.value)
	if err != nil {
		return nil, err
	}
	desc := r.description
	created := r.createdAt.Time
	updated := r.updatedAt.Time
	return &oapi.AppConfig{
		Id:            r.id,
		ConfigKey:     r.configKey,
		Value:         value,
		Version:       int(r.version),
		ContentHash:   r.contentHash,
		IsActive:      r.isActive,
		EffectiveFrom: pgToTimePtr(r.effectiveFrom),
		EffectiveTo:   pgToTimePtr(r.effectiveTo),
		Description:   &desc,
		CreatedAt:     &created,
		UpdatedAt:     &updated,
	}, nil
}

// getActiveConfig returns the current active row for a key. Missing key (no
// active row) yields ErrNotFound.
func (rp *repo) getActiveConfig(ctx context.Context, key string) (*oapi.AppConfig, error) {
	const sql = `SELECT` + appConfigColumns + `
		FROM app_config WHERE config_key = $1 AND is_active = TRUE`
	cr, err := scanAppConfig(rp.pool.QueryRow(ctx, sql, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("dict: get active config: %w", err)
	}
	return cr.toAppConfig()
}

// listConfigHistory returns all versions for a key, newest first. An empty slice
// (no rows at all) is returned as-is; callers decide whether that is a 404.
func (rp *repo) listConfigHistory(ctx context.Context, key string) ([]oapi.AppConfig, error) {
	const sql = `SELECT` + appConfigColumns + `
		FROM app_config WHERE config_key = $1 ORDER BY version DESC`
	rows, err := rp.pool.Query(ctx, sql, key)
	if err != nil {
		return nil, fmt.Errorf("dict: list config history: %w", err)
	}
	defer rows.Close()

	out := make([]oapi.AppConfig, 0)
	for rows.Next() {
		cr, serr := scanAppConfig(rows)
		if serr != nil {
			return nil, fmt.Errorf("dict: history scan: %w", serr)
		}
		cfg, cerr := cr.toAppConfig()
		if cerr != nil {
			return nil, cerr
		}
		out = append(out, *cfg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dict: history rows: %w", err)
	}
	return out, nil
}

// swapConfigArgs carries the validated inputs for a versioned config write.
type swapConfigArgs struct {
	key           string
	valueJSON     []byte
	contentHash   string
	description   *string
	effectiveFrom pgtype.Timestamptz
	updatedBy     *int64
}

// swapConfigVersion performs the versioned write inside a transaction: the
// current active row (if any) is retired (is_active=false, effective_to=now) and
// a new version+1 (or 1) is_active=true row is inserted with the recomputed
// content_hash. The partial unique index uq_app_config_active guarantees one
// active row per key; a lost race surfaces as ErrConfigConflict for the caller
// to retry.
func (rp *repo) swapConfigVersion(ctx context.Context, a swapConfigArgs) (*oapi.AppConfig, error) {
	var out *oapi.AppConfig
	err := rp.pool.WithTx(ctx, func(tx pgx.Tx) error {
		// Retire the currently-active row (if any) and learn its version.
		var prevVersion int32
		retErr := tx.QueryRow(ctx, `
			UPDATE app_config
			SET is_active = FALSE, effective_to = now(), updated_by = $2
			WHERE config_key = $1 AND is_active = TRUE
			RETURNING version`, a.key, a.updatedBy).Scan(&prevVersion)
		if retErr != nil && !errors.Is(retErr, pgx.ErrNoRows) {
			return fmt.Errorf("dict: retire active config: %w", retErr)
		}
		newVersion := prevVersion + 1 // 0+1 == 1 for the first version

		const ins = `
			INSERT INTO app_config
				(config_key, value, version, content_hash, is_active,
				 effective_from, description, created_by, updated_by)
			VALUES
				($1, $2, $3, $4, TRUE, COALESCE($5, now()), COALESCE($6, ''), $7, $7)
			RETURNING` + appConfigColumns
		cr, insErr := scanAppConfig(tx.QueryRow(ctx, ins,
			a.key, a.valueJSON, newVersion, a.contentHash,
			a.effectiveFrom, a.description, a.updatedBy))
		if insErr != nil {
			if isUniqueViolation(insErr) {
				// Either a concurrent active-row swap (uq_app_config_active) or a
				// version collision (uq_app_config_key_version) — both are races.
				return ErrConfigConflict
			}
			return fmt.Errorf("dict: insert config version: %w", insErr)
		}
		cfg, cerr := cr.toAppConfig()
		if cerr != nil {
			return cerr
		}
		out = cfg
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// withItemTx runs fn inside a transaction so the item mutation and the owning
// type's hash recompute commit atomically.
func (rp *repo) withItemTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return rp.pool.WithTx(ctx, fn)
}

// queryer is the read/write surface shared by *db.Pool and pgx.Tx, letting the
// item helpers run either directly on the pool (reads) or inside a transaction
// (mutations + same-tx hash recompute).
type queryer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
