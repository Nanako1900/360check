// Package cos defines the COS object-storage surface used by the media (P5) and
// export (P6) domains. The interface is the stable boundary so services can be
// tested with the in-package Mock; the real Tencent cos-go-sdk-v5 implementation
// lives in a sibling file (tencent.go) added by the media domain.
package cos

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// ErrNotFound is returned by HeadObject when the key does not exist.
var ErrNotFound = errors.New("cos: object not found")

// Client is the minimal COS surface C5 needs. Implementations must be safe for
// concurrent use.
type Client interface {
	// HeadObject returns the object's ETag and size, or ErrNotFound. Used by
	// /media/confirm to verify an upload before persisting the reference.
	HeadObject(ctx context.Context, bucket, key string) (etag string, size int64, err error)
	// PutObject uploads bytes (used by the derive-media worker for web/thumb and
	// by the export worker for the produced .xlsx).
	PutObject(ctx context.Context, bucket, key, contentType string, r io.Reader) error
	// GetObject downloads an object (used by derive-media to read the original).
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	// SignedURL returns a time-limited signed download URL (CDN/COS).
	SignedURL(ctx context.Context, bucket, key string, ttl time.Duration) (string, error)
}

// object is an in-memory stored object for the Mock.
type object struct {
	data        []byte
	etag        string
	contentType string
}

// Mock is an in-memory Client for tests. Seed objects with Put or SetObject.
type Mock struct {
	mu      sync.Mutex
	objects map[string]object // key = bucket + "/" + key
	// HeadErr, if set, is returned by HeadObject (to simulate verify failures).
	HeadErr error
}

// NewMock builds an empty in-memory COS mock.
func NewMock() *Mock { return &Mock{objects: map[string]object{}} }

func mkKey(bucket, key string) string { return bucket + "/" + key }

// SetObject seeds an object with an explicit etag/size for HeadObject verification.
func (m *Mock) SetObject(bucket, key, etag string, data []byte, contentType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[mkKey(bucket, key)] = object{data: data, etag: etag, contentType: contentType}
}

// HeadObject implements Client.
func (m *Mock) HeadObject(_ context.Context, bucket, key string) (string, int64, error) {
	if m.HeadErr != nil {
		return "", 0, m.HeadErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[mkKey(bucket, key)]
	if !ok {
		return "", 0, ErrNotFound
	}
	return o.etag, int64(len(o.data)), nil
}

// PutObject implements Client.
func (m *Mock) PutObject(_ context.Context, bucket, key, contentType string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[mkKey(bucket, key)] = object{data: b, etag: "mock-etag", contentType: contentType}
	return nil
}

// GetObject implements Client.
func (m *Mock) GetObject(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[mkKey(bucket, key)]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytesReader(o.data)), nil
}

// SignedURL implements Client.
func (m *Mock) SignedURL(_ context.Context, bucket, key string, _ time.Duration) (string, error) {
	return "https://mock-cdn.example.com/" + bucket + "/" + key + "?sign=mock", nil
}

func bytesReader(b []byte) io.Reader { return &sliceReader{b: b} }

type sliceReader struct {
	b []byte
	i int
}

func (s *sliceReader) Read(p []byte) (int, error) {
	if s.i >= len(s.b) {
		return 0, io.EOF
	}
	n := copy(p, s.b[s.i:])
	s.i += n
	return n, nil
}
