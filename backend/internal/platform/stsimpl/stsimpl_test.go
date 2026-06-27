package stsimpl

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/platform/sts"
)

// TestNew_Validation: the constructor rejects missing secret/appID.
func TestNew_Validation(t *testing.T) {
	_, err := New("", "sk", "1250000000")
	require.Error(t, err)
	_, err = New("id", "", "1250000000")
	require.Error(t, err)
	_, err = New("id", "sk", "")
	require.Error(t, err)

	i, err := New("id", "sk", "1250000000")
	require.NoError(t, err)
	require.NotNil(t, i)
	var _ sts.Issuer = i
}

// TestMultipartActions_Scope: the action set is EXACTLY the six multipart upload
// actions (docs/00 "STS 6 动作分片") — no PutObject, read, or wildcard leaks in.
func TestMultipartActions_Scope(t *testing.T) {
	want := map[string]bool{
		"name/cos:InitiateMultipartUpload": true,
		"name/cos:UploadPart":              true,
		"name/cos:CompleteMultipartUpload": true,
		"name/cos:AbortMultipartUpload":    true,
		"name/cos:ListMultipartUploads":    true,
		"name/cos:ListParts":               true,
	}
	assert.Len(t, multipartActions, 6)
	assert.Len(t, multipartActions, len(want))
	for _, a := range multipartActions {
		assert.Truef(t, want[a], "unexpected action %q in scoped policy", a)
		assert.NotContains(t, a, "Get", "no read action may be granted")
		assert.NotContains(t, a, "*", "no wildcard action may be granted")
	}
}

// TestLive_IssueScoped is a smoke test against the real STS API, skipped unless
// CAM creds are present.
func TestLive_IssueScoped(t *testing.T) {
	id := os.Getenv("C5_COS_SECRET_ID")
	sk := os.Getenv("C5_COS_SECRET_KEY")
	appID := os.Getenv("C5_COS_APPID")
	region := os.Getenv("C5_COS_REGION")
	bucket := os.Getenv("C5_COS_BUCKET_ORIGINAL")
	if id == "" || sk == "" || appID == "" || region == "" || bucket == "" {
		t.Skip("STS live creds not set; skipping network smoke test")
	}
	i, err := New(id, sk, appID)
	require.NoError(t, err)
	creds, err := i.IssueScoped(context.Background(), bucket, region, "media/test/1/uuid/", 15*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, creds.TmpSecretID)
	assert.NotEmpty(t, creds.TmpSecretKey)
	assert.NotEmpty(t, creds.SessionToken)
	assert.Positive(t, creds.ExpiredTime)
}
