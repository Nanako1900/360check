// Package sts defines the STS credential-issuing surface used by the media (P5)
// domain. The interface is the stable boundary so the media service can be tested
// with the in-package Mock; the real Tencent qcloud-cos-sts-sdk implementation
// lives in a sibling file (tencent.go) added by the media domain.
//
// The real Issuer MUST scope credentials to the per-upload prefix and only the 6
// multipart actions (InitiateMultipartUpload, UploadPart, CompleteMultipartUpload,
// AbortMultipartUpload, ListMultipartUploads, ListParts) with a short TTL.
package sts

import (
	"context"
	"time"
)

// Credentials are temporary, scoped STS credentials for a single upload.
type Credentials struct {
	TmpSecretID  string
	TmpSecretKey string
	SessionToken string
	ExpiredTime  int64 // unix seconds (absolute expiry)
}

// Issuer issues STS credentials scoped to a COS bucket/region/prefix.
type Issuer interface {
	IssueScoped(ctx context.Context, bucket, region, prefix string, ttl time.Duration) (*Credentials, error)
}

// Mock is a test Issuer returning deterministic fake credentials.
type Mock struct{ Err error }

// NewMock builds an STS mock.
func NewMock() *Mock { return &Mock{} }

// IssueScoped implements Issuer with fake credentials embedding the prefix.
func (m *Mock) IssueScoped(_ context.Context, _, _, prefix string, ttl time.Duration) (*Credentials, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return &Credentials{
		TmpSecretID:  "mock-tmp-id",
		TmpSecretKey: "mock-tmp-key",
		SessionToken: "mock-session-" + prefix,
		ExpiredTime:  time.Now().Add(ttl).Unix(),
	}, nil
}
