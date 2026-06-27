package export

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/nnkglobal/c5-backend/internal/platform/cos"
)

// eventSink is the minimal write+flush surface the SSE loop needs. The gin
// handler adapts its ResponseWriter to this; tests pass a buffer-backed sink so
// the emitted event sequence can be asserted without a real HTTP connection.
type eventSink interface {
	io.Writer
	Flush()
}

// sseSink adapts a gin ResponseWriter (io.Writer) + http.Flusher to eventSink.
type sseSink struct {
	w io.Writer
	f interface{ Flush() }
}

func (s sseSink) Write(p []byte) (int, error) { return s.w.Write(p) }
func (s sseSink) Flush()                      { s.f.Flush() }

// streamJobEvents drives the SSE loop: it re-reads the job every interval, emits a
// `progress` event whenever progress/processed/status changes (always at least the
// first read), and emits a terminal `done` event when the job reaches a terminal
// state — then returns. A heartbeat comment line is sent each tick so intermediaries
// keep the connection open even when nothing changed. The loop also returns if the
// client disconnects (ctx cancelled).
//
// reader is the job source (the Service in production, a fake in tests); signer
// produces the result_url for the done event on SUCCEEDED.
func streamJobEvents(ctx context.Context, sink eventSink, reader jobReader, signer cos.Client, jobUUID string, interval time.Duration) {
	// Emit an initial state immediately so a client that connects after the job
	// already finished still receives a terminal event without waiting a full tick.
	last := emitInitialAndMaybeDone(ctx, sink, reader, signer, jobUUID)
	if last == nil {
		return // already terminal (or unreadable): emitInitialAndMaybeDone handled it
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			job, err := reader.GetByUUID(ctx, jobUUID)
			if err != nil {
				// The job vanished mid-stream: emit a synthetic failure done event.
				writeDone(sink, doneEvent{Status: "FAILED", Error: strptr("job not found")})
				return
			}
			if progressChanged(last, job) {
				writeProgress(sink, progressEvent{
					Progress:      job.Progress,
					ProcessedRows: job.ProcessedRows,
					TotalRows:     job.TotalRows,
					Status:        job.Status,
				})
				last = job
			} else {
				writeHeartbeat(sink)
			}
			if isTerminal(job.Status) {
				writeDone(sink, doneEvent{
					Status:    job.Status,
					ResultURL: signedResultURL(ctx, job, signer),
					Error:     job.Error,
				})
				return
			}
		}
	}
}

// emitInitialAndMaybeDone reads the job once, emits a progress event, and — if the
// job is already terminal — emits the done event and returns nil (caller stops).
// Otherwise it returns the job as the baseline for change detection.
func emitInitialAndMaybeDone(ctx context.Context, sink eventSink, reader jobReader, signer cos.Client, jobUUID string) *jobRow {
	job, err := reader.GetByUUID(ctx, jobUUID)
	if err != nil {
		writeDone(sink, doneEvent{Status: "FAILED", Error: strptr("job not found")})
		return nil
	}
	writeProgress(sink, progressEvent{
		Progress:      job.Progress,
		ProcessedRows: job.ProcessedRows,
		TotalRows:     job.TotalRows,
		Status:        job.Status,
	})
	if isTerminal(job.Status) {
		writeDone(sink, doneEvent{
			Status:    job.Status,
			ResultURL: signedResultURL(ctx, job, signer),
			Error:     job.Error,
		})
		return nil
	}
	return job
}

// progressChanged reports whether anything client-visible moved since last emit.
func progressChanged(last, cur *jobRow) bool {
	if last == nil {
		return true
	}
	return last.Progress != cur.Progress ||
		last.ProcessedRows != cur.ProcessedRows ||
		last.Status != cur.Status
}

// writeProgress emits a named `progress` SSE event with a JSON data line.
func writeProgress(sink eventSink, ev progressEvent) {
	writeEvent(sink, "progress", ev)
}

// writeDone emits the terminal `done` SSE event and flushes.
func writeDone(sink eventSink, ev doneEvent) {
	writeEvent(sink, "done", ev)
}

// writeEvent serializes ev as JSON and writes a standard SSE frame
// (`event: <name>\n` + `data: <json>\n\n`), then flushes.
func writeEvent(sink eventSink, name string, ev any) {
	data, err := json.Marshal(ev)
	if err != nil {
		data = []byte("{}")
	}
	_, _ = fmt.Fprintf(sink, "event: %s\ndata: %s\n\n", name, data)
	sink.Flush()
}

// writeHeartbeat emits an SSE comment line (ignored by clients) to keep the
// connection and any intermediary buffers alive between progress changes.
func writeHeartbeat(sink eventSink) {
	_, _ = io.WriteString(sink, ": heartbeat\n\n")
	sink.Flush()
}

func strptr(s string) *string { return &s }
