package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
)

// TaskDeriveMediaTiers is the asynq task type that derives the web + thumb tiers
// from a CONFIRMED original (docs/00 D4, "derive-media-tiers").
const TaskDeriveMediaTiers = "media:derive-tiers"

// derivePayload is the JSON payload of a TaskDeriveMediaTiers task: the original
// media_assets row to derive from.
type derivePayload struct {
	OriginalID int64 `json:"original_id"`
}

// NewDeriveTask builds an enqueue-ready task for the original id. The handler
// reads the original, generates web + thumb siblings sharing its media_group, and
// uploads them to COS.
func NewDeriveTask(originalID int64) (*asynq.Task, error) {
	b, err := json.Marshal(derivePayload{OriginalID: originalID})
	if err != nil {
		return nil, fmt.Errorf("media: marshal derive payload: %w", err)
	}
	return asynq.NewTask(TaskDeriveMediaTiers, b), nil
}

// derived describes one tier to produce from the original.
type derived struct {
	tier      oapi.MediaTier
	maxWidth  int // longest-edge target; original is downscaled to fit
	bucketFor func(Config) string
}

// deriveTiers is the fixed set of siblings: web (~1920px) and thumb (~320px).
var deriveTiers = []derived{
	{tier: oapi.Web, maxWidth: 1920, bucketFor: func(c Config) string { return c.BucketWeb }},
	{tier: oapi.Thumb, maxWidth: 320, bucketFor: func(c Config) string { return c.BucketThumb }},
}

// RegisterWorkers mounts the media task handlers onto the shared asynq ServeMux.
// The handlers close over their own Service built from pool/cos/cfg so cmd/worker
// only needs to pass the platform dependencies.
func RegisterWorkers(mux *asynq.ServeMux, pool *db.Pool, cosClient cos.Client, cfg Config) {
	w := &Worker{pool: pool, cos: cosClient, cfg: cfg.withDefaults()}
	mux.HandleFunc(TaskDeriveMediaTiers, w.HandleDeriveTiers)
}

// Worker holds the dependencies for the media task handlers.
type Worker struct {
	pool *db.Pool
	cos  cos.Client
	cfg  Config
}

// HandleDeriveTiers derives the web + thumb siblings from a CONFIRMED original.
// It is idempotent: siblings are inserted ON CONFLICT (bucket,key) DO NOTHING, so
// a retried task neither duplicates rows nor re-derives needlessly. Decode/resize
// failures are non-fatal — the original bytes are copied verbatim so the sibling
// row still exists and is signable (the DoD), per the task brief.
func (w *Worker) HandleDeriveTiers(ctx context.Context, t *asynq.Task) error {
	var p derivePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		// A malformed payload can never succeed on retry — drop it.
		return fmt.Errorf("media: derive: bad payload: %w: %w", err, asynq.SkipRetry)
	}

	orig, err := w.loadOriginal(ctx, p.OriginalID)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotConfirmed) || errors.Is(err, ErrNotOriginal) {
			// Source is gone or not eligible — nothing to retry.
			return fmt.Errorf("media: derive load: %w: %w", err, asynq.SkipRetry)
		}
		return fmt.Errorf("media: derive load: %w", err)
	}

	// Ensure the original carries a media_group so siblings can share it. Under
	// concurrent derive retries two runs could both observe media_group=NULL and
	// mint different UUIDs; the losing run would then tag its siblings with an
	// orphan group (ON CONFLICT(bucket,key) dedups rows, NOT the group). ensureGroup
	// sets the group atomically (COALESCE keeps any value a concurrent winner already
	// wrote) and RETURNS the EFFECTIVE group, so every run adopts the same one.
	group := orig.group
	if group == uuid.Nil {
		effective, err := w.ensureGroup(ctx, orig.id, uuid.New())
		if err != nil {
			return fmt.Errorf("media: derive set group: %w", err)
		}
		group = effective
	}

	// Download the original bytes once.
	rc, err := w.cos.GetObject(ctx, orig.bucket, orig.key)
	if err != nil {
		return fmt.Errorf("media: derive get original: %w", err)
	}
	srcBytes, err := readAllClose(rc)
	if err != nil {
		return fmt.Errorf("media: derive read original: %w", err)
	}

	for _, d := range deriveTiers {
		bucket := d.bucketFor(w.cfg)
		key := deriveKey(orig.key, d.tier)

		// Resize; on any failure fall back to the original bytes so the sibling
		// still exists and is signable.
		data, contentType := resizeJPEGOrCopy(srcBytes, d.maxWidth, orig.contentType)

		if err := w.cos.PutObject(ctx, bucket, key, contentType, bytesReader(data)); err != nil {
			return fmt.Errorf("media: derive put %s: %w", d.tier, err)
		}
		if err := w.insertSibling(ctx, orig, group, d.tier, bucket, key, contentType, int64(len(data))); err != nil {
			return fmt.Errorf("media: derive insert %s: %w", d.tier, err)
		}
	}
	return nil
}

// originalRow is the minimal projection of an original needed to derive siblings.
type originalRow struct {
	id          int64
	ownerType   string
	ownerID     int64
	bucket      string
	key         string
	region      string
	contentType string
	group       uuid.UUID
	createdBy   *int64
}

// loadOriginal reads the original row and enforces it is the original tier and
// CONFIRMED before derivation proceeds.
func (w *Worker) loadOriginal(ctx context.Context, id int64) (originalRow, error) {
	var (
		o     originalRow
		tier  string
		state string
		group *uuid.UUID
	)
	err := w.pool.QueryRow(ctx, `
		SELECT id, owner_type, owner_id, cos_bucket, cos_key, cos_region,
		       content_type, tier, capture_state, media_group, created_by
		FROM media_assets
		WHERE id = $1 AND deleted_at IS NULL`, id).Scan(
		&o.id, &o.ownerType, &o.ownerID, &o.bucket, &o.key, &o.region,
		&o.contentType, &tier, &state, &group, &o.createdBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return originalRow{}, ErrNotFound
	}
	if err != nil {
		return originalRow{}, fmt.Errorf("media: load original: %w", err)
	}
	if oapi.MediaTier(tier) != oapi.Original {
		return originalRow{}, ErrNotOriginal
	}
	if oapi.CaptureState(state) != oapi.CONFIRMED {
		return originalRow{}, ErrNotConfirmed
	}
	if group != nil {
		o.group = *group
	}
	return o, nil
}

// ensureGroup atomically assigns proposed as the original's media_group only if it
// has none yet, and returns the EFFECTIVE group. COALESCE makes the write a no-op
// when a concurrent derive already set a group, and RETURNING hands back whatever
// value now stands — so a losing run adopts the winner's group instead of an orphan
// UUID. A single UPDATE...RETURNING is atomic under the row lock, closing the
// check-then-act race the previous read-then-conditional-write had.
func (w *Worker) ensureGroup(ctx context.Context, id int64, proposed uuid.UUID) (uuid.UUID, error) {
	var effective uuid.UUID
	err := w.pool.QueryRow(ctx,
		`UPDATE media_assets SET media_group = COALESCE(media_group, $2)
		 WHERE id = $1
		 RETURNING media_group`,
		id, proposed).Scan(&effective)
	if err != nil {
		return uuid.Nil, err
	}
	return effective, nil
}

// insertSibling writes one derived sibling row already CONFIRMED+verified_at,
// sharing the original's media_group/owner. Idempotent on (bucket,key).
func (w *Worker) insertSibling(ctx context.Context, orig originalRow, group uuid.UUID, tier oapi.MediaTier, bucket, key, contentType string, size int64) error {
	_, err := w.pool.Exec(ctx, `
		INSERT INTO media_assets
			(owner_type, owner_id, tier, cos_bucket, cos_key, cos_region, content_type,
			 byte_size, capture_state, verified_at, media_group, created_by, updated_by)
		VALUES ($1::media_owner_type, $2, $3::media_tier, $4, $5, $6, $7, $8,
			'CONFIRMED', now(), $9, $10, $10)
		ON CONFLICT (cos_bucket, cos_key) DO NOTHING`,
		orig.ownerType, orig.ownerID, string(tier), bucket, key, orig.region, contentType,
		size, group, orig.createdBy)
	return err
}
