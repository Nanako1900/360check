package httpx

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// Pagination defaults and bounds. page_size is clamped to MaxPageSize to keep
// list queries bounded.
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 200
)

// Page holds normalized pagination inputs derived from query params.
type Page struct {
	Page     int
	PageSize int
}

// Offset returns the SQL OFFSET for the page.
func (p Page) Offset() int { return (p.Page - 1) * p.PageSize }

// Limit returns the SQL LIMIT for the page.
func (p Page) Limit() int { return p.PageSize }

// ParsePage reads page/page_size query params, clamping to safe bounds.
func ParsePage(c *gin.Context) Page {
	page := atoiDefault(c.Query("page"), DefaultPage)
	if page < 1 {
		page = DefaultPage
	}
	size := atoiDefault(c.Query("page_size"), DefaultPageSize)
	if size < 1 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	return Page{Page: page, PageSize: size}
}

// MetaFor builds list pagination Meta from a total count and the current page.
func MetaFor(total int64, p Page) oapi.Meta {
	page := p.Page
	size := p.PageSize
	t := total
	return oapi.Meta{Total: &t, Page: &page, PageSize: &size}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
