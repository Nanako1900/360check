// Package obs provides cross-cutting observability helpers shared by the api and
// worker entrypoints: a dependency health/readiness HTTP surface (liveness,
// readiness, Prometheus metrics) that is transport-agnostic (net/http, no gin).
//
// The OTel tracer-provider/OTLP setup is added alongside this (tracing.go) during
// the P7 integration pass. This file intentionally depends only on the standard
// library + prometheus/client_golang so it builds and tests in isolation, ahead
// of the domain packages.
package obs

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ReadyCheck is one named readiness dependency (e.g. "postgres", "redis").
// Check must respect the context deadline and return nil when healthy.
type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// defaultReadyTimeout bounds the whole /readyz evaluation so a hung dependency
// cannot wedge the probe (which would otherwise stall the kubelet).
const defaultReadyTimeout = 3 * time.Second

// NewHealthMux builds an http.ServeMux exposing:
//
//	GET /healthz  liveness  — always 200 (the process is up); makes no dependency calls.
//	GET /readyz   readiness — runs every ReadyCheck; 200 when all pass, 503 otherwise.
//	GET /metrics  Prometheus exposition for reg (nil → the default gatherer).
//
// It is gin-agnostic: the api reuses ReadyzHandler via gin.WrapF and serves
// /metrics on its main port, while cmd/worker serves this whole mux on its
// dedicated metrics port (asynq exposes no HTTP of its own).
func NewHealthMux(reg prometheus.Gatherer, checks ...ReadyCheck) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", ReadyzHandler(checks...))
	if reg == nil {
		mux.Handle("/metrics", promhttp.Handler())
	} else {
		mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	}
	return mux
}

// ReadyzHandler returns a net/http handler that runs every check (the whole set
// bounded by defaultReadyTimeout) and reports 200 / 503 with a JSON body listing
// any failures. With no checks it always reports ready.
func ReadyzHandler(checks ...ReadyCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), defaultReadyTimeout)
		defer cancel()

		failed := map[string]string{}
		for _, c := range checks {
			if err := c.Check(ctx); err != nil {
				failed[c.Name] = err.Error()
			}
		}
		if len(failed) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "unready",
				"failed": failed,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

// NewHealthServer wraps NewHealthMux in an *http.Server bound to addr, for
// cmd/worker. The caller runs ListenAndServe and Shutdown alongside the asynq
// processor.
func NewHealthServer(addr string, reg prometheus.Gatherer, checks ...ReadyCheck) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           NewHealthMux(reg, checks...),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
