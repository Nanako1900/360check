package httpx

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

func TestStatusFor(t *testing.T) {
	// The canonical error-code -> HTTP status mapping (§4.2.1). UNAUTHENTICATED
	// (not UNAUTHORIZED) and TOKEN_EXPIRED both map to 401 but stay distinct codes.
	cases := []struct {
		code oapi.ErrorCode
		want int
	}{
		{oapi.VALIDATIONFAILED, http.StatusUnprocessableEntity},
		{oapi.UNAUTHENTICATED, http.StatusUnauthorized},
		{oapi.TOKENEXPIRED, http.StatusUnauthorized},
		{oapi.FORBIDDEN, http.StatusForbidden},
		{oapi.NOTFOUND, http.StatusNotFound},
		{oapi.CONFLICT, http.StatusConflict},
		{oapi.MEDIAVERIFYFAILED, http.StatusConflict},
		{oapi.DICTVERSIONRETIRED, http.StatusConflict},
		{oapi.RATELIMITED, http.StatusTooManyRequests},
		{oapi.INTERNAL, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			assert.Equal(t, tc.want, StatusFor(tc.code))
		})
	}
}

func TestStatusFor_UnknownDefaultsTo500(t *testing.T) {
	assert.Equal(t, http.StatusInternalServerError, StatusFor(oapi.ErrorCode("NOT_A_REAL_CODE")))
}

func TestAPIError(t *testing.T) {
	err := NewError(oapi.VALIDATIONFAILED, "bad input").
		WithDetails(oapi.ErrorDetail{Field: "name", Code: "required", Message: "name is required"})

	assert.Equal(t, http.StatusUnprocessableEntity, err.Status())
	assert.Equal(t, "VALIDATION_FAILED: bad input", err.Error())
	assert.Len(t, err.Details, 1)
	assert.Equal(t, "name", err.Details[0].Field)
}

func TestConvenienceConstructors(t *testing.T) {
	assert.Equal(t, oapi.UNAUTHENTICATED, ErrUnauthenticated("x").Code)
	assert.Equal(t, oapi.TOKENEXPIRED, ErrTokenExpired("x").Code)
	assert.Equal(t, oapi.FORBIDDEN, ErrForbidden("x").Code)
	assert.Equal(t, oapi.NOTFOUND, ErrNotFound("x").Code)
	assert.Equal(t, oapi.CONFLICT, ErrConflict("x").Code)
	assert.Equal(t, oapi.MEDIAVERIFYFAILED, ErrMediaVerifyFailed("x").Code)
	assert.Equal(t, oapi.RATELIMITED, ErrRateLimited("x").Code)
	assert.Equal(t, oapi.INTERNAL, ErrInternal("x").Code)
	assert.Equal(t, oapi.VALIDATIONFAILED, ErrValidation("x").Code)
}
