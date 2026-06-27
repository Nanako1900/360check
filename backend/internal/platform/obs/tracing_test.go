package obs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupTracing_NoEndpointIsNoop(t *testing.T) {
	shutdown, err := SetupTracing(context.Background(), "c5-api", "test", "")
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	// The no-op shutdown is always safe to defer.
	assert.NoError(t, shutdown(context.Background()))
}

func TestSetupTracing_WithEndpoint_BuildsProviderAndShutsDown(t *testing.T) {
	// otlptracegrpc.New is lazy (no dial until first export), so an unreachable
	// endpoint still constructs a real provider — this exercises the full setup +
	// shutdown path without a live collector.
	shutdown, err := SetupTracing(context.Background(), "c5-worker", "test", "127.0.0.1:4317")
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	assert.NoError(t, shutdown(context.Background()))
}
