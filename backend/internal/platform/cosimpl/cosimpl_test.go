package cosimpl

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/platform/cos"
)

// TestNew_Validation: the constructor rejects missing credentials/region.
func TestNew_Validation(t *testing.T) {
	_, err := New("", "sk", "ap-guangzhou")
	require.Error(t, err)
	_, err = New("id", "", "ap-guangzhou")
	require.Error(t, err)
	_, err = New("id", "sk", "")
	require.Error(t, err)

	c, err := New("id", "sk", "ap-guangzhou")
	require.NoError(t, err)
	require.NotNil(t, c)
	// It satisfies the stable cos.Client interface.
	var _ cos.Client = c
}

// TestBucketClient_BuildsURL: a per-bucket client is constructed for a valid
// bucket/region without performing any network call.
func TestBucketClient_BuildsURL(t *testing.T) {
	c, err := New("id", "sk", "ap-guangzhou")
	require.NoError(t, err)

	bc, err := c.bucketClient("c5-media-1250000000")
	require.NoError(t, err)
	require.NotNil(t, bc)
	require.NotNil(t, bc.BaseURL.BucketURL)
	assert.Contains(t, bc.BaseURL.BucketURL.Host, "c5-media-1250000000")
	assert.Contains(t, bc.BaseURL.BucketURL.Host, "ap-guangzhou")
}

// TestBucketClient_InvalidBucket: an invalid bucket name surfaces an error rather
// than panicking.
func TestBucketClient_InvalidBucket(t *testing.T) {
	c, err := New("id", "sk", "ap-guangzhou")
	require.NoError(t, err)
	_, err = c.bucketClient("nodash") // bucket must be name-appid
	require.Error(t, err)
}

// TestSignedURL_NoNetwork: presigning is a local HMAC operation, so it returns a
// URL without contacting COS.
func TestSignedURL_NoNetwork(t *testing.T) {
	c, err := New("id", "sk", "ap-guangzhou")
	require.NoError(t, err)
	u, err := c.SignedURL(context.Background(), "c5-media-1250000000", "media/x/1/uuid/original.jpg", time.Minute)
	require.NoError(t, err)
	assert.Contains(t, u, "c5-media-1250000000")
	assert.Contains(t, u, "media/x/1/uuid/original.jpg")
	assert.Contains(t, u, "q-signature=", "COS v5 presign query carries q-signature")
}

// TestLive_HeadObject is a smoke test against real COS, skipped unless COS creds
// are present in the environment (never run in CI without secrets).
func TestLive_HeadObject(t *testing.T) {
	id := os.Getenv("C5_COS_SECRET_ID")
	sk := os.Getenv("C5_COS_SECRET_KEY")
	region := os.Getenv("C5_COS_REGION")
	bucket := os.Getenv("C5_COS_BUCKET_ORIGINAL")
	key := os.Getenv("C5_COS_SMOKE_KEY")
	if id == "" || sk == "" || region == "" || bucket == "" || key == "" {
		t.Skip("COS live creds not set; skipping network smoke test")
	}
	c, err := New(id, sk, region)
	require.NoError(t, err)
	etag, size, err := c.HeadObject(context.Background(), bucket, key)
	require.NoError(t, err)
	assert.NotEmpty(t, etag)
	assert.Positive(t, size)
}
