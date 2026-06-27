// Package asynqx wraps hibiken/asynq with C5 defaults: a Redis option built from
// config, an enqueue Client (used by the media + export domains), and a Server
// for cmd/worker. Task type names are owned by their domains; the worker
// assembles each domain's RegisterWorkers(mux) into one asynq.ServeMux.
package asynqx

import (
	"time"

	"github.com/hibiken/asynq"
)

// shutdownTimeout bounds how long the server waits for in-flight tasks (notably
// long-running Excel exports) to drain on SIGTERM before forced termination. It
// sits comfortably under the worker pod's terminationGracePeriodSeconds.
const shutdownTimeout = 30 * time.Second

// RedisOpt builds the asynq Redis connection options from config values.
func RedisOpt(addr, password string, db int) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: addr, Password: password, DB: db}
}

// NewClient builds an asynq enqueue client. Callers must Close it.
func NewClient(addr, password string, db int) *asynq.Client {
	return asynq.NewClient(RedisOpt(addr, password, db))
}

// NewServer builds an asynq processing server (used by cmd/worker). concurrency
// <= 0 defaults to 10.
func NewServer(addr, password string, db, concurrency int) *asynq.Server {
	if concurrency <= 0 {
		concurrency = 10
	}
	return asynq.NewServer(RedisOpt(addr, password, db), asynq.Config{
		Concurrency:     concurrency,
		ShutdownTimeout: shutdownTimeout,
	})
}

// NewScheduler builds an asynq periodic scheduler (used for the UPLOADING reaper).
func NewScheduler(addr, password string, db int) *asynq.Scheduler {
	return asynq.NewScheduler(RedisOpt(addr, password, db), nil)
}
