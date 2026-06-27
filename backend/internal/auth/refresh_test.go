package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T, ttl time.Duration) (*RefreshStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewRefreshStore(rdb, ttl), mr
}

func TestRefresh_CreateAndRotate(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t, time.Hour)

	tok, err := store.Create(ctx, 42)
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	newTok, uid, err := store.Rotate(ctx, tok)
	require.NoError(t, err)
	assert.Equal(t, int64(42), uid)
	assert.NotEqual(t, tok, newTok)

	// Old token is single-use: a second rotate fails.
	_, _, err = store.Rotate(ctx, tok)
	require.ErrorIs(t, err, ErrRefreshNotFound)

	// New token still works.
	_, uid2, err := store.Rotate(ctx, newTok)
	require.NoError(t, err)
	assert.Equal(t, int64(42), uid2)
}

func TestRefresh_Revoke(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t, time.Hour)

	tok, err := store.Create(ctx, 1)
	require.NoError(t, err)
	require.NoError(t, store.Revoke(ctx, tok))

	_, _, err = store.Rotate(ctx, tok)
	require.ErrorIs(t, err, ErrRefreshNotFound)

	// Revoking empty / unknown is a no-op.
	require.NoError(t, store.Revoke(ctx, ""))
	require.NoError(t, store.Revoke(ctx, "nonexistent"))
}

func TestRefresh_Expiry(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t, time.Minute)

	tok, err := store.Create(ctx, 9)
	require.NoError(t, err)

	mr.FastForward(2 * time.Minute) // expire it
	_, _, err = store.Rotate(ctx, tok)
	require.ErrorIs(t, err, ErrRefreshNotFound)
}

func TestRefresh_ConsumeUnknown(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t, time.Hour)
	_, err := store.consume(ctx, "")
	require.ErrorIs(t, err, ErrRefreshNotFound)
}
