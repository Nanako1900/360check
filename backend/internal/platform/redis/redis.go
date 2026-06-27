// Package redis wraps go-redis/v9 for the three Redis workloads in C5: opaque
// refresh tokens, the casbin redis-watcher, and the asynq task queue.
package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

// Client wraps a go-redis client.
type Client struct {
	*goredis.Client
}

// New constructs a go-redis client. It does not verify connectivity; callers
// should Ping at startup to fail fast.
func New(addr, password string, db int) *Client {
	return &Client{Client: goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})}
}

// Ping verifies Redis connectivity (used by /healthz and /readyz).
func (c *Client) Ping(ctx context.Context) error {
	if err := c.Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}
