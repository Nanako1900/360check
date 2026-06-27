package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// Envelope is the single response wrapper for every endpoint:
//
//	{ "success": bool, "data": <T|null>, "error": <ErrorObject|null>, "meta": <Meta|null> }
//
// HTTP status always mirrors success (2xx success; 4xx/5xx error). The data,
// error and meta keys are always present (null when absent) to match the frozen
// contract and keep the generated TS/Kotlin clients strongly typed.
type Envelope struct {
	Success bool              `json:"success"`
	Data    any               `json:"data"`
	Error   *oapi.ErrorObject `json:"error"`
	Meta    *oapi.Meta        `json:"meta"`
}

// OK writes a 200 success envelope carrying data and no meta.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Success: true, Data: data})
}

// OKEmpty writes a 200 success envelope with null data (logout, delete, etc.).
func OKEmpty(c *gin.Context) {
	c.JSON(http.StatusOK, Envelope{Success: true})
}

// Created writes a 201 success envelope carrying the newly created resource.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Envelope{Success: true, Data: data})
}

// List writes a 200 success envelope carrying a page of data plus pagination meta.
func List(c *gin.Context, data any, meta oapi.Meta) {
	m := meta
	c.JSON(http.StatusOK, Envelope{Success: true, Data: data, Meta: &m})
}

// Fail writes an error envelope from an APIError, mirroring its HTTP status.
func Fail(c *gin.Context, err *APIError) {
	obj := &oapi.ErrorObject{Code: err.Code, Message: err.Message}
	if len(err.Details) > 0 {
		d := err.Details
		obj.Details = &d
	}
	c.JSON(err.Status(), Envelope{Success: false, Error: obj})
}

// FailAbort writes an error envelope and aborts the gin handler chain. Use from
// middleware so downstream handlers do not run after a rejection.
func FailAbort(c *gin.Context, err *APIError) {
	Fail(c, err)
	c.Abort()
}

// FailCode is a shorthand that builds and writes an error envelope.
func FailCode(c *gin.Context, code oapi.ErrorCode, message string, details ...oapi.ErrorDetail) {
	Fail(c, &APIError{Code: code, Message: message, Details: details})
}
