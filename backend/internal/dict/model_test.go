package dict

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHashItems_Deterministic verifies the item-set hash is stable for a fixed
// (id-ordered) set and changes only when a field that affects the payload changes.
func TestHashItems_Deterministic(t *testing.T) {
	base := []hashItemRow{
		{id: 1, code: "OPEN", label: "待处理", color: "#9CA3AF", sortOrder: 10, isActive: true},
		{id: 2, code: "CLOSED", label: "已关闭", color: "#6B7280", sortOrder: 40, isActive: true},
	}

	h1 := hashItems(base)
	h2 := hashItems(base)
	assert.Equal(t, h1, h2, "same input must hash identically")
	assert.NotEmpty(t, h1)
	assert.Len(t, h1, 64, "sha256 hex is 64 chars")
}

// TestHashItems_OrderIndependentForSortedSet verifies that re-building the same
// logical set (the SQL always orders by id) yields the same hash regardless of
// how the rows were appended in Go — because the caller sorts by id.
func TestHashItems_StableAcrossEqualSlices(t *testing.T) {
	a := []hashItemRow{
		{id: 1, code: "A", label: "a", color: "", sortOrder: 1, isActive: true},
		{id: 2, code: "B", label: "b", color: "#fff", sortOrder: 2, isActive: false},
	}
	b := []hashItemRow{
		{id: 1, code: "A", label: "a", color: "", sortOrder: 1, isActive: true},
		{id: 2, code: "B", label: "b", color: "#fff", sortOrder: 2, isActive: false},
	}
	assert.Equal(t, hashItems(a), hashItems(b))
}

// TestHashItems_ChangesOnEveryField asserts a different value in any serialized
// field (including is_active = retire) changes the hash.
func TestHashItems_ChangesOnEveryField(t *testing.T) {
	base := []hashItemRow{
		{id: 1, code: "OPEN", label: "待处理", color: "#9CA3AF", sortOrder: 10, isActive: true},
	}
	baseHash := hashItems(base)

	cases := map[string][]hashItemRow{
		"label changed":      {{id: 1, code: "OPEN", label: "处理中", color: "#9CA3AF", sortOrder: 10, isActive: true}},
		"color changed":      {{id: 1, code: "OPEN", label: "待处理", color: "#000000", sortOrder: 10, isActive: true}},
		"sort_order changed": {{id: 1, code: "OPEN", label: "待处理", color: "#9CA3AF", sortOrder: 99, isActive: true}},
		"code changed":       {{id: 1, code: "NEW", label: "待处理", color: "#9CA3AF", sortOrder: 10, isActive: true}},
		"retired (is_active false)": {
			{id: 1, code: "OPEN", label: "待处理", color: "#9CA3AF", sortOrder: 10, isActive: false},
		},
		"item added": {
			{id: 1, code: "OPEN", label: "待处理", color: "#9CA3AF", sortOrder: 10, isActive: true},
			{id: 2, code: "CLOSED", label: "已关闭", color: "#6B7280", sortOrder: 40, isActive: true},
		},
	}
	for name, mutated := range cases {
		t.Run(name, func(t *testing.T) {
			assert.NotEqual(t, baseHash, hashItems(mutated), "%s must change the hash", name)
		})
	}
}

// TestHashItems_NilColorEqualsEmpty confirms a NULL color (empty string) is a
// stable distinct value, not a source of nondeterminism.
func TestHashItems_NilColorEqualsEmpty(t *testing.T) {
	withEmpty := []hashItemRow{{id: 1, code: "X", label: "x", color: "", sortOrder: 0, isActive: true}}
	again := []hashItemRow{{id: 1, code: "X", label: "x", color: "", sortOrder: 0, isActive: true}}
	assert.Equal(t, hashItems(withEmpty), hashItems(again))

	withColor := []hashItemRow{{id: 1, code: "X", label: "x", color: "#fff", sortOrder: 0, isActive: true}}
	assert.NotEqual(t, hashItems(withEmpty), hashItems(withColor))
}

// TestHashItems_EmptySet gives a stable hash for a type with no items.
func TestHashItems_EmptySet(t *testing.T) {
	assert.Equal(t, hashItems(nil), hashItems([]hashItemRow{}))
	assert.NotEmpty(t, hashItems(nil))
}

// TestHashValue_CanonicalKeyOrder verifies the config-value hash ignores the
// client's key ordering (Go's json.Marshal sorts map keys), so two logically
// equal values hash identically.
func TestHashValue_CanonicalKeyOrder(t *testing.T) {
	a := map[string]interface{}{"width": 4096, "height": 2048, "quality": 85}
	b := map[string]interface{}{"quality": 85, "height": 2048, "width": 4096}

	ha, err := hashValue(a)
	require.NoError(t, err)
	hb, err := hashValue(b)
	require.NoError(t, err)
	assert.Equal(t, ha, hb, "key order must not affect the value hash")
	assert.Len(t, ha, 64)
}

// TestHashValue_ChangesOnValueChange asserts a changed value changes the hash.
func TestHashValue_ChangesOnValueChange(t *testing.T) {
	a := map[string]interface{}{"mode": "NORMAL", "hdr": false}
	b := map[string]interface{}{"mode": "HDR", "hdr": true}
	ha, err := hashValue(a)
	require.NoError(t, err)
	hb, err := hashValue(b)
	require.NoError(t, err)
	assert.NotEqual(t, ha, hb)
}

// TestUnquoteETag covers strong, weak and bare ETag header forms.
func TestUnquoteETag(t *testing.T) {
	assert.Equal(t, "abc", unquoteETag(`"abc"`))
	assert.Equal(t, "abc", unquoteETag(`W/"abc"`))
	assert.Equal(t, "abc", unquoteETag(`abc`))
	assert.Equal(t, "", unquoteETag(``))
}

// TestEtagMatches verifies the If-None-Match comparison tolerates quoting.
func TestEtagMatches(t *testing.T) {
	assert.True(t, etagMatches(`"deadbeef"`, "deadbeef"))
	assert.True(t, etagMatches(`W/"deadbeef"`, "deadbeef"))
	assert.True(t, etagMatches(`deadbeef`, "deadbeef"))
	assert.False(t, etagMatches(`"other"`, "deadbeef"))
	assert.False(t, etagMatches(``, "deadbeef"))
}
