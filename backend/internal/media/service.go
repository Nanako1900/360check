// Package media implements the P5 media domain: COS STS upload credentials,
// HeadObject-verified confirm, the capture state machine, the derive-media-tiers
// worker (original → web + thumb siblings sharing a media_group), and the
// UPLOADING reaper.
//
// The binary never travels through the JSON API. The flow is:
//
//  1. POST /media/upload-credentials — accepts ONLY tier=original (D4). Builds a
//     per-upload prefix, issues prefix-scoped STS creds, and inserts a
//     media_assets row in capture_state=UPLOADING (idempotent on client_uuid).
//  2. APP multipart-uploads the bytes straight to COS with the scoped creds.
//  3. POST /media/confirm — HeadObject-verifies the key (etag+size must match the
//     client report) BEFORE persisting; on match advances to CONFIRMED and sets
//     verified_at. Mismatch → MEDIA_VERIFY_FAILED (409), row stays UPLOADING.
//  4. The confirm of an original enqueues derive-media-tiers, which generates the
//     web + thumb siblings.
//
// Every statement is hand-written pgx against the shared pool (consistent with
// internal/inspection): sqlc is not used here.
package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
	"github.com/nnkglobal/c5-backend/internal/platform/db"
	"github.com/nnkglobal/c5-backend/internal/platform/sts"
)

// Config holds the COS/STS settings the media service needs. Bucket/region come
// from config.COSConfig; TTLs default if zero.
type Config struct {
	BucketOriginal string
	BucketWeb      string
	BucketThumb    string
	Region         string
	// STSTTL is the lifetime of issued upload credentials (default 15m).
	STSTTL time.Duration
	// SignedURLTTL is the lifetime of GET signed download URLs (default 1h).
	SignedURLTTL time.Duration
	// UploadingTTL is how long a row may sit in UPLOADING before the reaper
	// flags it as stale (default 24h).
	UploadingTTL time.Duration
}

// withDefaults returns a copy of cfg with zero TTLs filled in.
func (c Config) withDefaults() Config {
	if c.STSTTL <= 0 {
		c.STSTTL = 15 * time.Minute
	}
	if c.SignedURLTTL <= 0 {
		c.SignedURLTTL = time.Hour
	}
	if c.UploadingTTL <= 0 {
		c.UploadingTTL = 24 * time.Hour
	}
	return c
}

// bucketForTier returns the configured bucket for a tier.
func (c Config) bucketForTier(tier oapi.MediaTier) string {
	switch tier {
	case oapi.Web:
		return c.BucketWeb
	case oapi.Thumb:
		return c.BucketThumb
	default:
		return c.BucketOriginal
	}
}

// Service owns the media_assets table plus the COS/STS dependencies.
type Service struct {
	pool   *db.Pool
	cos    cos.Client
	issuer sts.Issuer
	cfg    Config
}

// NewService wires the media service. cos/issuer are the stable interfaces (real
// impls in cosimpl/stsimpl; mocks in tests).
func NewService(pool *db.Pool, cosClient cos.Client, issuer sts.Issuer, cfg Config) *Service {
	return &Service{pool: pool, cos: cosClient, issuer: issuer, cfg: cfg.withDefaults()}
}

// mediaCols is the canonical projection used by every read; the column ORDER is
// the contract scanMedia depends on.
const mediaCols = `id, client_uuid, owner_type, owner_id, tier, cos_bucket, cos_key,
	cos_region, content_type, byte_size, width, height, etag, capture_state,
	verified_at, media_group, meta, created_at, updated_at`

// scanMedia materializes one media_assets row from mediaCols into oapi.MediaAsset.
func scanMedia(row pgx.Row) (oapi.MediaAsset, error) {
	var (
		m          oapi.MediaAsset
		ownerType  string
		tier       string
		state      string
		clientUUID uuid.UUID
		group      *uuid.UUID
		metaJSON   []byte
		content    *string
	)
	if err := row.Scan(
		&m.Id, &clientUUID, &ownerType, &m.OwnerId, &tier, &m.CosBucket, &m.CosKey,
		&m.CosRegion, &content, &m.ByteSize, &m.Width, &m.Height, &m.Etag, &state,
		&m.VerifiedAt, &group, &metaJSON, &m.CreatedAt, &m.UpdatedAt,
	); err != nil {
		return oapi.MediaAsset{}, err
	}
	m.ClientUuid = clientUUID
	m.OwnerType = oapi.MediaOwnerType(ownerType)
	m.Tier = oapi.MediaTier(tier)
	m.CaptureState = oapi.CaptureState(state)
	m.ContentType = content
	if group != nil {
		g := *group
		m.MediaGroup = &g
	}
	if len(metaJSON) > 0 {
		var mm map[string]interface{}
		if err := json.Unmarshal(metaJSON, &mm); err == nil && len(mm) > 0 {
			m.Meta = &mm
		}
	}
	return m, nil
}

// CredentialsInput is the validated payload for IssueUploadCredentials.
type CredentialsInput struct {
	OwnerType   oapi.MediaOwnerType
	OwnerID     int64
	Tier        oapi.MediaTier
	ContentType string
	ByteSize    int64
	ClientUUID  uuid.UUID
}

// IssueUploadCredentials implements step 1. It enforces D4 (tier=original only),
// builds a per-upload prefix + key, issues prefix-scoped STS creds, and inserts
// the row in UPLOADING (idempotent on client_uuid). A replayed call with the same
// client_uuid returns fresh creds for the SAME existing key (the bytes can be
// re-uploaded), never a duplicate row.
func (s *Service) IssueUploadCredentials(ctx context.Context, in CredentialsInput, actorID int64) (*oapi.MediaUploadCredentials, error) {
	if in.Tier != oapi.Original {
		return nil, ErrTierNotOriginal
	}
	if !validOwnerType(in.OwnerType) {
		return nil, ErrInvalidOwnerType
	}

	bucket := s.cfg.bucketForTier(oapi.Original)
	contentType := in.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	// Resolve (or create) the row first so a replay reuses its key/prefix.
	key, err := s.upsertUploadingRow(ctx, in, bucket, contentType, actorID)
	if err != nil {
		return nil, err
	}
	prefix := prefixOf(key)

	creds, err := s.issuer.IssueScoped(ctx, bucket, s.cfg.Region, prefix, s.cfg.STSTTL)
	if err != nil {
		return nil, fmt.Errorf("media: issue sts: %w", err)
	}

	return &oapi.MediaUploadCredentials{
		Bucket: bucket,
		Region: s.cfg.Region,
		Key:    key,
		Prefix: prefix,
		Credentials: oapi.StsCredentials{
			TmpSecretId:  creds.TmpSecretID,
			TmpSecretKey: creds.TmpSecretKey,
			SessionToken: creds.SessionToken,
			ExpiredTime:  creds.ExpiredTime,
		},
	}, nil
}

// upsertUploadingRow inserts the original row in UPLOADING (idempotent on
// client_uuid) and returns its cos_key. On conflict the existing key is returned.
func (s *Service) upsertUploadingRow(ctx context.Context, in CredentialsInput, bucket, contentType string, actorID int64) (string, error) {
	// Deterministic per-upload key from the client_uuid so a replay maps to the
	// same prefix/key and the unique (bucket,key) index never collides.
	key := buildOriginalKey(in.OwnerType, in.OwnerID, in.ClientUUID)

	var gotKey string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO media_assets
			(client_uuid, owner_type, owner_id, tier, cos_bucket, cos_key, cos_region,
			 content_type, byte_size, capture_state, created_by, updated_by)
		VALUES ($1, $2::media_owner_type, $3, 'original', $4, $5, $6, $7, $8,
			'UPLOADING', $9, $9)
		ON CONFLICT (client_uuid) DO NOTHING
		RETURNING cos_key`,
		in.ClientUUID, string(in.OwnerType), in.OwnerID, bucket, key, s.cfg.Region,
		contentType, nullableSize(in.ByteSize), actorID,
	).Scan(&gotKey)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Conflict: row exists — return its key.
		if err := s.pool.QueryRow(ctx,
			`SELECT cos_key FROM media_assets WHERE client_uuid = $1 AND deleted_at IS NULL`,
			in.ClientUUID).Scan(&gotKey); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", ErrNotFound
			}
			return "", fmt.Errorf("media: reselect on conflict: %w", err)
		}
		return gotKey, nil
	case err != nil:
		return "", fmt.Errorf("media: insert uploading: %w", err)
	}
	return gotKey, nil
}

// ConfirmInput is the validated payload for Confirm.
type ConfirmInput struct {
	ClientUUID uuid.UUID
	Key        string
	Etag       string
	ByteSize   int64
	Width      *int
	Height     *int
}

// Confirm implements step 3. It loads the UPLOADING row, HeadObject-verifies the
// key (the COS etag and size MUST match the client's report), and only on a match
// advances the row to CONFIRMED with verified_at=now and the persisted
// etag/size/width/height. Any mismatch (missing key, wrong etag, wrong size) →
// ErrVerifyFailed and the row stays UPLOADING for client retry.
//
// Returns the confirmed asset plus whether a derive task should be enqueued
// (true only when an original transitions into CONFIRMED here).
func (s *Service) Confirm(ctx context.Context, in ConfirmInput, actorID int64) (*oapi.MediaAsset, bool, error) {
	var (
		out     oapi.MediaAsset
		enqueue bool
	)
	err := s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		var (
			bucket     string
			key        string
			state      string
			verifiedAt *time.Time // throwaway; CONFIRMED idempotency keys off state
		)
		err := tx.QueryRow(ctx, `
			SELECT cos_bucket, cos_key, capture_state, verified_at
			FROM media_assets
			WHERE client_uuid = $1 AND deleted_at IS NULL
			FOR UPDATE`, in.ClientUUID).Scan(&bucket, &key, &state, &verifiedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("media: confirm load: %w", err)
		}

		// Idempotent re-confirm: already CONFIRMED → return current row, no enqueue.
		if oapi.CaptureState(state) == oapi.CONFIRMED {
			m, err := scanMedia(tx.QueryRow(ctx,
				`SELECT `+mediaCols+` FROM media_assets WHERE client_uuid = $1 AND deleted_at IS NULL`, in.ClientUUID))
			if err != nil {
				return fmt.Errorf("media: confirm reselect: %w", err)
			}
			out = m
			return nil
		}

		// HeadObject-verify BEFORE persisting. The client reports a key but we trust
		// the row's stored key (the STS prefix scope); they must agree.
		if in.Key != "" && in.Key != key {
			return ErrVerifyFailed
		}
		etag, size, herr := s.cos.HeadObject(ctx, bucket, key)
		if herr != nil {
			if errors.Is(herr, cos.ErrNotFound) {
				return ErrVerifyFailed
			}
			return fmt.Errorf("media: headobject: %w", herr)
		}
		if !etagMatches(etag, in.Etag) || size != in.ByteSize {
			return ErrVerifyFailed
		}

		// Forward-only guard (defensive; DB row is UPLOADING here).
		if !canTransition(oapi.CaptureState(state), oapi.CONFIRMED) {
			return ErrVerifyFailed
		}

		m, err := scanMedia(tx.QueryRow(ctx, `
			UPDATE media_assets SET
				capture_state = 'CONFIRMED',
				verified_at   = now(),
				etag          = $2,
				byte_size     = $3,
				width         = COALESCE($4, width),
				height        = COALESCE($5, height),
				updated_by    = $6
			WHERE client_uuid = $1
			RETURNING `+mediaCols,
			in.ClientUUID, etag, in.ByteSize, in.Width, in.Height, actorID))
		if err != nil {
			return fmt.Errorf("media: confirm update: %w", err)
		}
		out = m
		enqueue = m.Tier == oapi.Original
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &out, enqueue, nil
}

// Get returns one non-deleted media asset with a freshly minted signed CDN URL
// for its tier populated on SignedUrl. Missing/deleted → ErrNotFound.
func (s *Service) Get(ctx context.Context, id int64) (*oapi.MediaAsset, error) {
	m, err := scanMedia(s.pool.QueryRow(ctx,
		`SELECT `+mediaCols+` FROM media_assets WHERE id = $1 AND deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("media: get: %w", err)
	}
	url, err := s.cos.SignedURL(ctx, m.CosBucket, m.CosKey, s.cfg.SignedURLTTL)
	if err != nil {
		return nil, fmt.Errorf("media: signed url: %w", err)
	}
	m.SignedUrl = &url
	return &m, nil
}

// validOwnerType reports whether t is a frozen media_owner_type literal.
func validOwnerType(t oapi.MediaOwnerType) bool {
	switch t {
	case oapi.MediaOwnerTypeProblem, oapi.MediaOwnerTypeInspection,
		oapi.MediaOwnerTypeProject, oapi.MediaOwnerTypeUser:
		return true
	default:
		return false
	}
}

// nullableSize returns nil for a non-positive byte size so the column stays NULL
// until confirm fills the verified size.
func nullableSize(n int64) *int64 {
	if n <= 0 {
		return nil
	}
	return &n
}
