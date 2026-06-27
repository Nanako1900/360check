// Package cosimpl is the real Tencent cos-go-sdk-v5 implementation of the
// cos.Client interface defined in internal/platform/cos. It is kept in its own
// package (rather than a sibling file inside cos) so the cos package stays free
// of the heavy SDK dependency for the many packages that only need the Mock.
//
// The backend uses one long-lived admin credential (config COSConfig
// SecretID/SecretKey) for HeadObject/PutObject/GetObject and for signing
// download URLs. The short-lived, prefix-scoped STS credentials handed to the
// APP for multipart upload are issued by internal/platform/stsimpl, NOT here.
package cosimpl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	tcos "github.com/tencentyun/cos-go-sdk-v5"

	"github.com/nnkglobal/c5-backend/internal/platform/cos"
)

// Client is the real COS client. It builds a per-bucket cos.Client lazily and
// caches them, because cos-go-sdk-v5 binds one http.Client (with the credential
// transport) to a single bucket URL. All buckets share the same region and
// admin credential.
type Client struct {
	secretID  string
	secretKey string
	region    string
	secure    bool
}

// Option configures the Client.
type Option func(*Client)

// WithInsecure forces http instead of https (only for local/dev COS gateways).
func WithInsecure() Option { return func(c *Client) { c.secure = false } }

// New builds a real COS client from the admin credential + region. region is the
// COS region code (e.g. "ap-guangzhou").
func New(secretID, secretKey, region string, opts ...Option) (*Client, error) {
	if secretID == "" || secretKey == "" {
		return nil, fmt.Errorf("cosimpl: secret id/key are required")
	}
	if region == "" {
		return nil, fmt.Errorf("cosimpl: region is required")
	}
	c := &Client{secretID: secretID, secretKey: secretKey, region: region, secure: true}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

var _ cos.Client = (*Client)(nil)

// bucketClient builds a cos-go-sdk client bound to one bucket, authenticated
// with the admin credential.
func (c *Client) bucketClient(bucket string) (*tcos.Client, error) {
	u, err := tcos.NewBucketURL(bucket, c.region, c.secure)
	if err != nil {
		return nil, fmt.Errorf("cosimpl: bucket url %q: %w", bucket, err)
	}
	hc := &http.Client{
		Transport: &tcos.AuthorizationTransport{
			SecretID:  c.secretID,
			SecretKey: c.secretKey,
		},
		Timeout: 60 * time.Second,
	}
	return tcos.NewClient(&tcos.BaseURL{BucketURL: u}, hc), nil
}

// HeadObject returns the object's ETag (quotes stripped) and size, or
// cos.ErrNotFound when the key is absent (HTTP 404).
func (c *Client) HeadObject(ctx context.Context, bucket, key string) (string, int64, error) {
	bc, err := c.bucketClient(bucket)
	if err != nil {
		return "", 0, err
	}
	resp, err := bc.Object.Head(ctx, key, nil)
	if err != nil {
		if tcos.IsNotFoundError(err) {
			return "", 0, cos.ErrNotFound
		}
		return "", 0, fmt.Errorf("cosimpl: head %q: %w", key, err)
	}
	etag := strings.Trim(resp.Header.Get("ETag"), `"`)
	var size int64
	if v := resp.Header.Get("Content-Length"); v != "" {
		size, _ = strconv.ParseInt(v, 10, 64)
	}
	return etag, size, nil
}

// PutObject uploads r under key. The reader is buffered to a *bytes.Reader so the
// SDK can compute Content-Length (it rejects unsized readers).
func (c *Client) PutObject(ctx context.Context, bucket, key, contentType string, r io.Reader) error {
	bc, err := c.bucketClient(bucket)
	if err != nil {
		return err
	}
	buf, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("cosimpl: read body for %q: %w", key, err)
	}
	opt := &tcos.ObjectPutOptions{
		ObjectPutHeaderOptions: &tcos.ObjectPutHeaderOptions{ContentType: contentType},
	}
	if _, err := bc.Object.Put(ctx, key, bytes.NewReader(buf), opt); err != nil {
		return fmt.Errorf("cosimpl: put %q: %w", key, err)
	}
	return nil
}

// GetObject downloads key. cos.ErrNotFound on a missing key.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	bc, err := c.bucketClient(bucket)
	if err != nil {
		return nil, err
	}
	resp, err := bc.Object.Get(ctx, key, nil)
	if err != nil {
		if tcos.IsNotFoundError(err) {
			return nil, cos.ErrNotFound
		}
		return nil, fmt.Errorf("cosimpl: get %q: %w", key, err)
	}
	return resp.Body, nil
}

// SignedURL returns a time-limited signed GET URL for key, signed with the admin
// credential. The APP/web fetch tiers through this URL (or the CDN domain in
// front of it).
func (c *Client) SignedURL(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	bc, err := c.bucketClient(bucket)
	if err != nil {
		return "", err
	}
	u, err := bc.Object.GetPresignedURL(ctx, http.MethodGet, key, c.secretID, c.secretKey, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("cosimpl: presign %q: %w", key, err)
	}
	return u.String(), nil
}
