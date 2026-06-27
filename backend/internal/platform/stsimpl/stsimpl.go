// Package stsimpl is the real Tencent qcloud-cos-sts-sdk implementation of the
// sts.Issuer interface defined in internal/platform/sts. It lives in its own
// package so the sts package stays dependency-light for the many callers that
// only need the Mock.
//
// IssueScoped returns short-lived credentials scoped to exactly the per-upload
// prefix and only the six multipart-upload actions, so a leaked credential can
// neither read other objects nor write outside its prefix (docs/00 §STS flow).
package stsimpl

import (
	"context"
	"fmt"
	"time"

	tsts "github.com/tencentyun/qcloud-cos-sts-sdk/go"

	"github.com/nnkglobal/c5-backend/internal/platform/sts"
)

// multipartActions is the minimal COS action set the APP needs to perform a
// resumable multipart upload — nothing else. PutObject is intentionally excluded
// because the contract uploads via multipart only.
var multipartActions = []string{
	"name/cos:InitiateMultipartUpload",
	"name/cos:UploadPart",
	"name/cos:CompleteMultipartUpload",
	"name/cos:AbortMultipartUpload",
	"name/cos:ListMultipartUploads",
	"name/cos:ListParts",
}

// Issuer issues prefix-scoped STS credentials via the Tencent STS API. appID is
// the COS APPID (the numeric suffix of the bucket name), needed to build the
// fully-qualified COS resource ARN.
type Issuer struct {
	client *tsts.Client
	appID  string
}

// New builds a real STS issuer from the long-lived CAM secret + COS APPID.
func New(secretID, secretKey, appID string) (*Issuer, error) {
	if secretID == "" || secretKey == "" {
		return nil, fmt.Errorf("stsimpl: secret id/key are required")
	}
	if appID == "" {
		return nil, fmt.Errorf("stsimpl: appID is required")
	}
	return &Issuer{
		client: tsts.NewClient(secretID, secretKey, nil),
		appID:  appID,
	}, nil
}

var _ sts.Issuer = (*Issuer)(nil)

// IssueScoped requests credentials limited to bucket/region under prefix with the
// multipart actions and ttl. The resource ARN restricts writes to prefix/* only.
func (i *Issuer) IssueScoped(_ context.Context, bucket, region, prefix string, ttl time.Duration) (*sts.Credentials, error) {
	secs := int64(ttl / time.Second)
	if secs <= 0 {
		secs = int64((15 * time.Minute) / time.Second)
	}
	resource := fmt.Sprintf("qcs::cos:%s:uid/%s:%s/%s*", region, i.appID, bucket, prefix)

	opt := &tsts.CredentialOptions{
		DurationSeconds: secs,
		Region:          region,
		Policy: &tsts.CredentialPolicy{
			Version: "2.0",
			Statement: []tsts.CredentialPolicyStatement{{
				Effect:   "allow",
				Action:   multipartActions,
				Resource: []string{resource},
			}},
		},
	}

	res, err := i.client.GetCredential(opt)
	if err != nil {
		return nil, fmt.Errorf("stsimpl: get credential: %w", err)
	}
	if res == nil || res.Credentials == nil {
		return nil, fmt.Errorf("stsimpl: empty credential result")
	}
	return &sts.Credentials{
		TmpSecretID:  res.Credentials.TmpSecretID,
		TmpSecretKey: res.Credentials.TmpSecretKey,
		SessionToken: res.Credentials.SessionToken,
		ExpiredTime:  int64(res.ExpiredTime),
	}, nil
}
