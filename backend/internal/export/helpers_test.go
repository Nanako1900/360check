package export

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

func TestClampProgress(t *testing.T) {
	assert.Equal(t, 0, clampProgress(-5))
	assert.Equal(t, 0, clampProgress(0))
	assert.Equal(t, 50, clampProgress(50))
	assert.Equal(t, 100, clampProgress(100))
	assert.Equal(t, 100, clampProgress(250))
}

func TestPercent(t *testing.T) {
	assert.Equal(t, 100, percent(0, 0), "no rows = complete")
	assert.Equal(t, 0, percent(0, 10))
	assert.Equal(t, 50, percent(5, 10))
	assert.Equal(t, 100, percent(10, 10))
	assert.Equal(t, 100, percent(20, 10), "clamped over 100")
}

func TestMetersToKm(t *testing.T) {
	assert.InDelta(t, 1.025, metersToKm(1025.2), 1e-9)
	assert.InDelta(t, 0.0, metersToKm(0), 1e-9)
	assert.InDelta(t, 152.341, metersToKm(152340.5), 1e-9)
}

func TestShanghaiRendering(t *testing.T) {
	// 2026-06-26 00:00:00 UTC == 2026-06-26 08:00:00 Asia/Shanghai (UTC+8).
	utc := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "2026-06-26 08:00:00", shanghaiStringV(utc))
	assert.Equal(t, "2026-06-26 08:00:00", shanghaiString(&utc))

	assert.Equal(t, "", shanghaiString(nil))
	assert.Equal(t, "", shanghaiStringV(time.Time{}))
}

func TestExportParams_StatusDecoding(t *testing.T) {
	// PROBLEM_LIST: status is a numeric dict_item id.
	var pl exportParams
	require.NoError(t, json.Unmarshal([]byte(`{"status": 7}`), &pl))
	id := pl.statusAsID()
	require.NotNil(t, id)
	assert.EqualValues(t, 7, *id)
	assert.Nil(t, pl.statusAsString(), "numeric status is not a string")

	// INSPECTION_RECORDS: status is an enum string.
	var ir exportParams
	require.NoError(t, json.Unmarshal([]byte(`{"status": "FINISHED"}`), &ir))
	s := ir.statusAsString()
	require.NotNil(t, s)
	assert.Equal(t, "FINISHED", *s)
	assert.Nil(t, ir.statusAsID(), "string status is not an id")

	// Absent status decodes to nil both ways.
	var empty exportParams
	require.NoError(t, json.Unmarshal([]byte(`{}`), &empty))
	assert.Nil(t, empty.statusAsID())
	assert.Nil(t, empty.statusAsString())
}

func TestExportParams_TimeBound(t *testing.T) {
	s := "2026-06-26T00:00:00Z"
	got := timeBound(&s)
	require.NotNil(t, got)
	assert.True(t, got.Equal(time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)))

	assert.Nil(t, timeBound(nil))
	bad := "not-a-time"
	assert.Nil(t, timeBound(&bad))
	blank := ""
	assert.Nil(t, timeBound(&blank))
}

func TestFormatBuckets(t *testing.T) {
	assert.Equal(t, "", formatBuckets(nil))
	assert.Equal(t, "", formatBuckets([]oapi.CountBucket{}))

	got := formatBuckets([]oapi.CountBucket{
		{Label: "路面", Count: 42},
		{Label: "设施", Count: 8},
	})
	assert.Equal(t, "路面×42; 设施×8", got)
}

func TestValidType(t *testing.T) {
	assert.True(t, validType(oapi.INSPECTIONRECORDS))
	assert.True(t, validType(oapi.PROBLEMLIST))
	assert.True(t, validType(oapi.PROJECTSTATS))
	assert.False(t, validType(oapi.ExportType("NOPE")))
	assert.False(t, validType(oapi.ExportType("")))
}

func TestResultKey(t *testing.T) {
	j := &jobRow{Type: oapi.PROBLEMLIST, JobUUID: "abc-123"}
	assert.Equal(t, "exports/PROBLEM_LIST/abc-123.xlsx", resultKey(j))
}
