package dict

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// Service holds the dict/config business logic: input validation, the dict_item
// mutation flow (mutate + same-tx content_hash recompute), and the app_config
// version-swap. The ETag returned to clients is always the STORED content_hash.
type Service struct {
	repo *repo
}

// NewService wires the dict service onto a pgx pool.
func NewService(pool *db.Pool) *Service { return &Service{repo: newRepo(pool)} }

// --- dict_type --------------------------------------------------------------

// ListDictTypes returns all dict_types matching the optional scope/is_active
// filter, ordered by code.
func (s *Service) ListDictTypes(ctx context.Context, scope *string, isActive *bool) ([]oapi.DictType, error) {
	return s.repo.listDictTypes(ctx, listDictTypesFilter{scope: scope, isActive: isActive})
}

// CreateDictType validates and inserts a dict_type. It enforces a non-empty
// code/name and a recognized scope.
func (s *Service) CreateDictType(ctx context.Context, in oapi.DictTypeCreate, actorID int64) (*oapi.DictType, error) {
	if in.Code == "" {
		return nil, newValidation("code is required", "code", "REQUIRED", "code must not be empty")
	}
	if in.Name == "" {
		return nil, newValidation("name is required", "name", "REQUIRED", "name must not be empty")
	}
	if !isValidScope(in.Scope) {
		return nil, newValidation("invalid scope", "scope", "INVALID",
			"scope must be one of problem_type, problem_status, problem_category, project_field, capture_preset, misc")
	}
	return s.repo.createDictType(ctx, createDictTypeArgs{
		code:        in.Code,
		name:        in.Name,
		scope:       string(in.Scope),
		description: strOrEmpty(in.Description),
		createdBy:   actorPtr(actorID),
	})
}

// UpdateDictType validates and applies a partial dict_type update.
func (s *Service) UpdateDictType(ctx context.Context, id int64, in oapi.DictTypeUpdate, actorID int64) (*oapi.DictType, error) {
	if in.Name != nil && *in.Name == "" {
		return nil, newValidation("name must not be empty", "name", "INVALID", "name must not be empty")
	}
	return s.repo.updateDictType(ctx, updateDictTypeArgs{
		id:          id,
		name:        in.Name,
		description: in.Description,
		isActive:    in.IsActive,
		updatedBy:   actorPtr(actorID),
	})
}

// DeleteDictType soft-deletes a dict_type (and CASCADE-affects items only via the
// schema; here we only mark the type deleted). Missing/deleted -> ErrNotFound.
func (s *Service) DeleteDictType(ctx context.Context, id, actorID int64) error {
	return s.repo.softDeleteDictType(ctx, id, actorPtr(actorID))
}

// --- dict_item pull-with-version --------------------------------------------

// ItemsPayload bundles a type's items with its stored version + content_hash;
// the handler emits the content_hash as the ETag. notModified is true when the
// caller's If-None-Match equals the stored hash, in which case Payload is the
// zero value and the handler returns 304.
type ItemsPayload struct {
	Payload     oapi.DictItemsPayload
	ETag        string
	NotModified bool
}

// GetItemsByTypeCode loads a dict_type by its machine code plus its items. When
// ifNoneMatch matches the stored content_hash, it returns NotModified=true and
// skips loading items (the handler answers 304). includeInactive defaults to
// true at the handler; retired items are part of the payload so clients can
// render historical references.
func (s *Service) GetItemsByTypeCode(ctx context.Context, code string, includeInactive bool, ifNoneMatch string) (*ItemsPayload, error) {
	dt, err := s.repo.getDictTypeByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	// ETag is the STORED hash; never recomputed on read.
	etag := dt.ContentHash
	if ifNoneMatch != "" && etagMatches(ifNoneMatch, etag) {
		return &ItemsPayload{ETag: etag, NotModified: true}, nil
	}
	items, err := s.repo.listItemsByTypeID(ctx, s.repo.pool, dt.Id, includeInactive)
	if err != nil {
		return nil, err
	}
	return &ItemsPayload{
		ETag: etag,
		Payload: oapi.DictItemsPayload{
			Type:        *dt,
			Items:       items,
			ContentHash: dt.ContentHash,
			Version:     dt.Version,
		},
	}, nil
}

// --- dict_item mutations (mutate + same-tx hash recompute) -------------------

// CreateItem validates and inserts a dict_item, then recomputes the owning
// type's content_hash + version in the SAME transaction so the ETag changes iff
// the item set changes.
func (s *Service) CreateItem(ctx context.Context, in oapi.DictItemCreate, actorID int64) (*oapi.DictItem, error) {
	if in.Code == "" {
		return nil, newValidation("code is required", "code", "REQUIRED", "code must not be empty")
	}
	if in.Label == "" {
		return nil, newValidation("label is required", "label", "REQUIRED", "label must not be empty")
	}
	if in.DictTypeId <= 0 {
		return nil, newValidation("dict_type_id is required", "dict_type_id", "REQUIRED", "dict_type_id must be a positive id")
	}
	extra, err := extraJSON(in.Extra)
	if err != nil {
		return nil, err
	}
	sortOrder := 0
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	var out *oapi.DictItem
	txErr := s.repo.withItemTx(ctx, func(tx pgx.Tx) error {
		item, cErr := s.repo.createItem(ctx, tx, createItemArgs{
			dictTypeID: in.DictTypeId,
			code:       in.Code,
			label:      in.Label,
			color:      in.Color,
			extra:      extra,
			sortOrder:  sortOrder,
			createdBy:  actorPtr(actorID),
		})
		if cErr != nil {
			return cErr
		}
		if _, _, hErr := s.repo.recomputeTypeHash(ctx, tx, in.DictTypeId); hErr != nil {
			return hErr
		}
		out = item
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return out, nil
}

// UpdateItem validates and applies a partial dict_item update (retire via
// is_active=false), then recomputes the owning type's content_hash + version in
// the SAME transaction.
func (s *Service) UpdateItem(ctx context.Context, id int64, in oapi.DictItemUpdate, actorID int64) (*oapi.DictItem, error) {
	if in.Label != nil && *in.Label == "" {
		return nil, newValidation("label must not be empty", "label", "INVALID", "label must not be empty")
	}
	var extra []byte
	if in.Extra != nil {
		b, err := extraJSON(in.Extra)
		if err != nil {
			return nil, err
		}
		extra = b
	}
	var out *oapi.DictItem
	txErr := s.repo.withItemTx(ctx, func(tx pgx.Tx) error {
		typeID, tErr := s.repo.getItemTypeID(ctx, tx, id)
		if tErr != nil {
			return tErr
		}
		// Color is nullable: the update model carries `Color *string` with no
		// "omitempty", so a present payload always intends to set it (possibly to
		// null). We always apply color from the request.
		item, uErr := s.repo.updateItem(ctx, tx, updateItemArgs{
			id:        id,
			label:     in.Label,
			setColor:  true,
			color:     in.Color,
			extra:     extra,
			sortOrder: in.SortOrder,
			isActive:  in.IsActive,
			updatedBy: actorPtr(actorID),
		})
		if uErr != nil {
			return uErr
		}
		if _, _, hErr := s.repo.recomputeTypeHash(ctx, tx, typeID); hErr != nil {
			return hErr
		}
		out = item
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return out, nil
}

// DeleteItem soft-deletes a dict_item when it is NOT referenced by any problem;
// a referenced item is retired (is_active=false) instead and never hard-deleted.
// Either way the owning type's content_hash + version is recomputed in the SAME
// transaction. The returned bool reports whether the item was retired (true)
// rather than soft-deleted (false).
func (s *Service) DeleteItem(ctx context.Context, id, actorID int64) (retired bool, err error) {
	txErr := s.repo.withItemTx(ctx, func(tx pgx.Tx) error {
		typeID, tErr := s.repo.getItemTypeID(ctx, tx, id)
		if tErr != nil {
			return tErr
		}
		referenced, rErr := s.repo.itemIsReferenced(ctx, tx, id)
		if rErr != nil {
			return rErr
		}
		if referenced {
			// Never hard-delete a referenced item: retire it instead.
			active := false
			if _, uErr := s.repo.updateItem(ctx, tx, updateItemArgs{
				id: id, isActive: &active, updatedBy: actorPtr(actorID),
			}); uErr != nil {
				return uErr
			}
			retired = true
		} else if dErr := s.repo.softDeleteItem(ctx, tx, id, actorPtr(actorID)); dErr != nil {
			return dErr
		}
		if _, _, hErr := s.repo.recomputeTypeHash(ctx, tx, typeID); hErr != nil {
			return hErr
		}
		return nil
	})
	if txErr != nil {
		return false, txErr
	}
	return retired, nil
}

// --- app_config -------------------------------------------------------------

// ConfigResult bundles the active config with its ETag (content_hash) and a
// notModified flag for the GET 304 path.
type ConfigResult struct {
	Config      *oapi.AppConfig
	ETag        string
	NotModified bool
}

// GetConfig returns the current active config for a key, honoring the
// If-None-Match ETag (304 when the stored content_hash matches).
func (s *Service) GetConfig(ctx context.Context, key, ifNoneMatch string) (*ConfigResult, error) {
	cfg, err := s.repo.getActiveConfig(ctx, key)
	if err != nil {
		return nil, err
	}
	etag := cfg.ContentHash
	if ifNoneMatch != "" && etagMatches(ifNoneMatch, etag) {
		return &ConfigResult{ETag: etag, NotModified: true}, nil
	}
	return &ConfigResult{Config: cfg, ETag: etag}, nil
}

// GetConfigHistory returns all versions of a key, newest first. An unknown key
// (no rows) -> ErrNotFound so the handler answers 404.
func (s *Service) GetConfigHistory(ctx context.Context, key string) ([]oapi.AppConfig, error) {
	history, err := s.repo.listConfigHistory(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, ErrNotFound
	}
	return history, nil
}

// PutConfig writes a new active version of a key in a transaction: the current
// active row is retired and a new version+1 row is inserted with a recomputed
// content_hash over the value's canonical JSON form. The key is taken from the
// path; in.Value is required.
func (s *Service) PutConfig(ctx context.Context, key string, in oapi.AppConfigUpdate, actorID int64) (*oapi.AppConfig, error) {
	if key == "" {
		return nil, newValidation("key is required", "key", "REQUIRED", "config key must not be empty")
	}
	if in.Value == nil {
		return nil, newValidation("value is required", "value", "REQUIRED", "config value must not be null")
	}
	hash, err := hashValue(in.Value)
	if err != nil {
		return nil, fmt.Errorf("dict: hash config value: %w", err)
	}
	valueJSON, err := marshalValue(in.Value)
	if err != nil {
		return nil, err
	}
	return s.repo.swapConfigVersion(ctx, swapConfigArgs{
		key:           key,
		valueJSON:     valueJSON,
		contentHash:   hash,
		description:   in.Description,
		effectiveFrom: timeToPg(in.EffectiveFrom),
		updatedBy:     actorPtr(actorID),
	})
}

// --- helpers ----------------------------------------------------------------

// etagMatches compares a client If-None-Match header value against a stored
// content_hash, tolerating the optional surrounding quotes the HTTP ETag syntax
// uses (the handler sets `ETag: "<hash>"`).
func etagMatches(ifNoneMatch, stored string) bool {
	return unquoteETag(ifNoneMatch) == stored
}

// unquoteETag strips a single pair of surrounding double quotes and an optional
// weak-validator "W/" prefix from an ETag value.
func unquoteETag(v string) string {
	if len(v) >= 2 && v[0] == 'W' && v[1] == '/' {
		v = v[2:]
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	return v
}
