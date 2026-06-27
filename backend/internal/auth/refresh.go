// Package auth implements login/refresh/logout/me/change-password: short-lived
// JWT access tokens (internal/platform/jwt) paired with opaque, revocable refresh
// tokens stored in Redis. Refresh rotates on every use (old token single-use).
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// ErrRefreshNotFound is returned when a refresh token is unknown, already used
// (rotated), or expired.
var ErrRefreshNotFound = errors.New("refresh token not found or expired")

const refreshKeyPrefix = "refresh:"

// RefreshStore manages opaque, revocable refresh tokens in Redis.
type RefreshStore struct {
	rdb goredis.Cmdable
	ttl time.Duration
}

// NewRefreshStore builds a RefreshStore with the given token TTL.
func NewRefreshStore(rdb goredis.Cmdable, ttl time.Duration) *RefreshStore {
	return &RefreshStore{rdb: rdb, ttl: ttl}
}

// Create issues a new opaque refresh token bound to userID.
func (s *RefreshStore) Create(ctx context.Context, userID int64) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, refreshKeyPrefix+token, userID, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("store refresh token: %w", err)
	}
	return token, nil
}

// Rotate atomically consumes oldToken (single-use) and issues a fresh token for
// the same user. Reusing a rotated/old token returns ErrRefreshNotFound.
func (s *RefreshStore) Rotate(ctx context.Context, oldToken string) (newToken string, userID int64, err error) {
	uid, err := s.consume(ctx, oldToken)
	if err != nil {
		return "", 0, err
	}
	nt, err := s.Create(ctx, uid)
	if err != nil {
		return "", 0, err
	}
	return nt, uid, nil
}

// Revoke deletes a refresh token (logout). A missing/empty token is not an error.
func (s *RefreshStore) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.rdb.Del(ctx, refreshKeyPrefix+token).Err(); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

// consume atomically reads and deletes a token (GETDEL), so a token can never be
// used twice even under concurrent refresh.
func (s *RefreshStore) consume(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, ErrRefreshNotFound
	}
	res, err := s.rdb.GetDel(ctx, refreshKeyPrefix+token).Result()
	if errors.Is(err, goredis.Nil) {
		return 0, ErrRefreshNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("consume refresh token: %w", err)
	}
	uid, err := strconv.ParseInt(res, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("corrupt refresh token payload: %w", err)
	}
	return uid, nil
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
