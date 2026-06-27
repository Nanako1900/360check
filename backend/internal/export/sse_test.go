package export

import (
	"bufio"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/platform/cos"
)

// bufSink is a buffer-backed eventSink that records every byte written so the SSE
// frame sequence can be asserted. Flush is a no-op (nothing to flush in a buffer).
type bufSink struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *bufSink) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.WriteString(string(p))
}
func (b *bufSink) Flush() {}
func (b *bufSink) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// fakeJobReader advances a job through a scripted sequence of states on each read.
// The last state repeats once exhausted.
type fakeJobReader struct {
	mu     sync.Mutex
	states []*jobRow
	idx    int
}

func (f *fakeJobReader) GetByUUID(_ context.Context, _ string) (*jobRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.states[f.idx]
	if f.idx < len(f.states)-1 {
		f.idx++
	}
	return st, nil
}

func job(status oapi.JobStatus, progress, processed int, total int) *jobRow {
	t := total
	return &jobRow{
		JobUUID:       "11111111-1111-1111-1111-111111111111",
		Type:          oapi.PROBLEMLIST,
		Status:        status,
		Progress:      progress,
		ProcessedRows: processed,
		TotalRows:     &t,
	}
}

// parseEvents splits an SSE stream into (eventName, dataLine) pairs, ignoring
// heartbeat comment lines (lines starting with ':').
func parseEvents(raw string) []struct{ Event, Data string } {
	var out []struct{ Event, Data string }
	sc := bufio.NewScanner(strings.NewReader(raw))
	var cur struct{ Event, Data string }
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			cur.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			cur.Data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "":
			if cur.Event != "" {
				out = append(out, cur)
				cur = struct{ Event, Data string }{}
			}
		}
	}
	return out
}

func TestStreamJobEvents_ProgressThenDone(t *testing.T) {
	reader := &fakeJobReader{states: []*jobRow{
		job(oapi.RUNNING, 0, 0, 10),
		job(oapi.RUNNING, 50, 5, 10),
		job(oapi.SUCCEEDED, 100, 10, 10),
	}}
	reader.states[2].ResultBucket = strptr("c5-exports")
	reader.states[2].ResultCosKey = strptr("exports/PROBLEM_LIST/x.xlsx")

	sink := &bufSink{}
	signer := cos.NewMock()

	// Tiny interval so the loop advances quickly through the scripted states.
	streamJobEvents(context.Background(), sink, reader, signer,
		"11111111-1111-1111-1111-111111111111", 2*time.Millisecond)

	evts := parseEvents(sink.String())
	require.NotEmpty(t, evts, "expected at least one event")

	// First event is a progress (initial state emitted immediately).
	assert.Equal(t, "progress", evts[0].Event)
	assert.Contains(t, evts[0].Data, `"status":"RUNNING"`)

	// Last event is the terminal done with SUCCEEDED + a result_url.
	last := evts[len(evts)-1]
	assert.Equal(t, "done", last.Event)
	assert.Contains(t, last.Data, `"status":"SUCCEEDED"`)
	assert.Contains(t, last.Data, `"result_url"`)
	assert.Contains(t, last.Data, "mock-cdn.example.com")

	// A 50% progress event appeared somewhere in the middle.
	var saw50 bool
	for _, e := range evts {
		if e.Event == "progress" && strings.Contains(e.Data, `"progress":50`) {
			saw50 = true
		}
	}
	assert.True(t, saw50, "expected a mid-stream 50%% progress event")
}

func TestStreamJobEvents_AlreadyTerminalEmitsDoneImmediately(t *testing.T) {
	j := job(oapi.FAILED, 30, 3, 10)
	j.Error = strptr("boom")
	reader := &fakeJobReader{states: []*jobRow{j}}
	sink := &bufSink{}

	streamJobEvents(context.Background(), sink, reader, cos.NewMock(),
		"11111111-1111-1111-1111-111111111111", time.Hour) // interval never fires

	evts := parseEvents(sink.String())
	require.Len(t, evts, 2, "an initial progress + terminal done")
	assert.Equal(t, "progress", evts[0].Event)
	assert.Equal(t, "done", evts[1].Event)
	assert.Contains(t, evts[1].Data, `"status":"FAILED"`)
	assert.Contains(t, evts[1].Data, `"error":"boom"`)
}

func TestStreamJobEvents_ContextCancelStops(t *testing.T) {
	// Job never reaches terminal; cancelling ctx must stop the loop promptly.
	reader := &fakeJobReader{states: []*jobRow{job(oapi.RUNNING, 10, 1, 10)}}
	sink := &bufSink{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		streamJobEvents(ctx, sink, reader, cos.NewMock(),
			"11111111-1111-1111-1111-111111111111", 5*time.Millisecond)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("streamJobEvents did not return after context cancel")
	}
	// At least the initial progress event was emitted; no terminal done.
	evts := parseEvents(sink.String())
	require.NotEmpty(t, evts)
	assert.Equal(t, "progress", evts[0].Event)
	for _, e := range evts {
		assert.NotEqual(t, "done", e.Event, "no terminal done for a cancelled non-terminal job")
	}
}

func TestProgressChanged(t *testing.T) {
	base := job(oapi.RUNNING, 10, 1, 10)
	assert.True(t, progressChanged(nil, base), "nil last always changes")
	assert.False(t, progressChanged(base, job(oapi.RUNNING, 10, 1, 10)), "identical = no change")
	assert.True(t, progressChanged(base, job(oapi.RUNNING, 20, 1, 10)), "progress moved")
	assert.True(t, progressChanged(base, job(oapi.RUNNING, 10, 2, 10)), "processed moved")
	assert.True(t, progressChanged(base, job(oapi.SUCCEEDED, 10, 1, 10)), "status moved")
}
