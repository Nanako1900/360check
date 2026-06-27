package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func pageFromQuery(t *testing.T, query string) Page {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return ParsePage(c)
}

func TestParsePage(t *testing.T) {
	cases := []struct {
		name         string
		query        string
		wantPage     int
		wantPageSize int
	}{
		{"defaults", "", DefaultPage, DefaultPageSize},
		{"explicit", "page=3&page_size=50", 3, 50},
		{"zero clamps to defaults", "page=0&page_size=0", DefaultPage, DefaultPageSize},
		{"negative clamps to defaults", "page=-5&page_size=-1", DefaultPage, DefaultPageSize},
		{"oversize clamps to max", "page=1&page_size=10000", 1, MaxPageSize},
		{"garbage falls back to defaults", "page=abc&page_size=xyz", DefaultPage, DefaultPageSize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := pageFromQuery(t, tc.query)
			assert.Equal(t, tc.wantPage, p.Page)
			assert.Equal(t, tc.wantPageSize, p.PageSize)
		})
	}
}

func TestPageOffsetLimit(t *testing.T) {
	p := Page{Page: 3, PageSize: 20}
	assert.Equal(t, 40, p.Offset())
	assert.Equal(t, 20, p.Limit())

	first := Page{Page: 1, PageSize: 20}
	assert.Equal(t, 0, first.Offset())
}

func TestMetaFor(t *testing.T) {
	m := MetaFor(123, Page{Page: 2, PageSize: 25})
	assert.EqualValues(t, 123, *m.Total)
	assert.Equal(t, 2, *m.Page)
	assert.Equal(t, 25, *m.PageSize)
}
