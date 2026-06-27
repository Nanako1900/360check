// Package httpx provides the single response envelope, the canonical error-code
// catalog and pagination helpers shared by every C5 HTTP handler.
package httpx

import (
	"fmt"
	"net/http"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// Canonical error codes — re-exported from the generated OpenAPI model so the
// backend, web and app share one literal set. Note: UNAUTHENTICATED (never
// UNAUTHORIZED), and TOKEN_EXPIRED is distinct so clients can single-flight a
// refresh. See §4.2.1 of the API contract.
const (
	CodeValidationFailed   = oapi.VALIDATIONFAILED
	CodeUnauthenticated    = oapi.UNAUTHENTICATED
	CodeTokenExpired       = oapi.TOKENEXPIRED
	CodeForbidden          = oapi.FORBIDDEN
	CodeNotFound           = oapi.NOTFOUND
	CodeConflict           = oapi.CONFLICT
	CodeMediaVerifyFailed  = oapi.MEDIAVERIFYFAILED
	CodeDictVersionRetired = oapi.DICTVERSIONRETIRED
	CodeRateLimited        = oapi.RATELIMITED
	CodeInternal           = oapi.INTERNAL
)

// statusByCode is the single canonical error-code → HTTP status mapping (§4.2.1).
var statusByCode = map[oapi.ErrorCode]int{
	oapi.VALIDATIONFAILED:   http.StatusUnprocessableEntity, // 422
	oapi.UNAUTHENTICATED:    http.StatusUnauthorized,        // 401
	oapi.TOKENEXPIRED:       http.StatusUnauthorized,        // 401
	oapi.FORBIDDEN:          http.StatusForbidden,           // 403
	oapi.NOTFOUND:           http.StatusNotFound,            // 404
	oapi.CONFLICT:           http.StatusConflict,            // 409
	oapi.MEDIAVERIFYFAILED:  http.StatusConflict,            // 409
	oapi.DICTVERSIONRETIRED: http.StatusConflict,            // 409
	oapi.RATELIMITED:        http.StatusTooManyRequests,     // 429
	oapi.INTERNAL:           http.StatusInternalServerError, // 500
}

// StatusFor returns the HTTP status mapped from a canonical error code; an
// unknown code maps to 500 so a misconfiguration never leaks as a 2xx.
func StatusFor(code oapi.ErrorCode) int {
	if s, ok := statusByCode[code]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// APIError is a structured, envelope-ready error carrying a canonical code,
// a client-safe message and optional field-level validation details.
type APIError struct {
	Code    oapi.ErrorCode
	Message string
	Details []oapi.ErrorDetail
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Status returns the HTTP status mapped from the error's canonical code.
func (e *APIError) Status() int { return StatusFor(e.Code) }

// NewError builds an APIError with a canonical code and a client-safe message.
func NewError(code oapi.ErrorCode, message string) *APIError {
	return &APIError{Code: code, Message: message}
}

// WithDetails attaches field-level validation details to a freshly built error.
func (e *APIError) WithDetails(details ...oapi.ErrorDetail) *APIError {
	e.Details = append(e.Details, details...)
	return e
}

// Convenience constructors for the common codes. Messages must stay client-safe
// (no stack traces, SQL or internal identifiers).
func ErrValidation(msg string) *APIError        { return NewError(oapi.VALIDATIONFAILED, msg) }
func ErrUnauthenticated(msg string) *APIError   { return NewError(oapi.UNAUTHENTICATED, msg) }
func ErrTokenExpired(msg string) *APIError      { return NewError(oapi.TOKENEXPIRED, msg) }
func ErrForbidden(msg string) *APIError         { return NewError(oapi.FORBIDDEN, msg) }
func ErrNotFound(msg string) *APIError          { return NewError(oapi.NOTFOUND, msg) }
func ErrConflict(msg string) *APIError          { return NewError(oapi.CONFLICT, msg) }
func ErrMediaVerifyFailed(msg string) *APIError { return NewError(oapi.MEDIAVERIFYFAILED, msg) }
func ErrRateLimited(msg string) *APIError       { return NewError(oapi.RATELIMITED, msg) }
func ErrInternal(msg string) *APIError          { return NewError(oapi.INTERNAL, msg) }
