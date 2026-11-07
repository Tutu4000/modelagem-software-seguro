// Package problem answers errors in the Problem Details format (RFC 9457). The
// internal cause goes to the log, never to the client.
package problem

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Code string

const (
	Unauthenticated  Code = "unauthenticated"
	Forbidden        Code = "forbidden"
	FileTooLarge     Code = "file_too_large"
	InvalidSTL       Code = "invalid_stl"
	TooManyTriangles Code = "too_many_triangles"
	Internal         Code = "internal"
)

var catalog = map[Code]struct {
	status int
	title  string
}{
	Unauthenticated:  {http.StatusUnauthorized, "Authentication required"},
	Forbidden:        {http.StatusForbidden, "Access denied"},
	FileTooLarge:     {http.StatusRequestEntityTooLarge, "File too large"},
	InvalidSTL:       {http.StatusUnprocessableEntity, "Invalid STL"},
	TooManyTriangles: {http.StatusUnprocessableEntity, "STL with too many triangles"},
	Internal:         {http.StatusInternalServerError, "Internal error"},
}

type Error struct {
	Code  Code
	Cause error
}

func New(code Code, cause error) *Error {
	return &Error{Code: code, Cause: cause}
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return string(e.Code)
	}
	return string(e.Code) + ": " + e.Cause.Error()
}

func (e *Error) Unwrap() error { return e.Cause }

type details struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Code   Code   `json:"code"`
}

// Abort keeps the full error in c.Errors, for the log, and answers only the
// code. An error without a known Code becomes a 500.
func Abort(c *gin.Context, err error) {
	if err != nil {
		_ = c.Error(err)
	}
	code := Internal
	var pe *Error
	if errors.As(err, &pe) {
		if _, ok := catalog[pe.Code]; ok {
			code = pe.Code
		}
	}
	entry := catalog[code]
	c.Header("Content-Type", "application/problem+json")
	c.AbortWithStatusJSON(entry.status, details{
		Type:   "/errors/" + string(code),
		Title:  entry.title,
		Status: entry.status,
		Code:   code,
	})
}
