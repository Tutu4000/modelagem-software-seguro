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
	RateLimited      Code = "rate_limited"
	Unavailable      Code = "unavailable"
	InvalidRequest   Code = "invalid_request"
	// MFARequired: the token does not prove a second factor (panel, D02).
	MFARequired Code = "mfa_required"
	// StepUpRequired: a critical operation needs a recent login (E6.4).
	StepUpRequired Code = "step_up_required"
	// JustificationRequired: a critical operation needs a written reason.
	JustificationRequired Code = "justification_required"
	IdPUnavailable        Code = "idp_unavailable"
	// Conflict: the resource is not in the status the operation needs.
	Conflict     Code = "conflict"
	QuoteExpired Code = "quote_expired"
	// PartnerUnavailable: the gateway or the carrier did not answer (D03).
	PartnerUnavailable Code = "partner_unavailable"
	// InvalidSignature: a webhook without a valid signature (C14, C15).
	InvalidSignature Code = "invalid_signature"
	Internal         Code = "internal"
)

var catalog = map[Code]struct {
	status int
	title  string
}{
	Unauthenticated:       {http.StatusUnauthorized, "Authentication required"},
	Forbidden:             {http.StatusForbidden, "Access denied"},
	FileTooLarge:          {http.StatusRequestEntityTooLarge, "File too large"},
	InvalidSTL:            {http.StatusUnprocessableEntity, "Invalid STL"},
	TooManyTriangles:      {http.StatusUnprocessableEntity, "STL with too many triangles"},
	RateLimited:           {http.StatusTooManyRequests, "Too many requests"},
	Unavailable:           {http.StatusServiceUnavailable, "Service unavailable"},
	InvalidRequest:        {http.StatusBadRequest, "Invalid request"},
	MFARequired:           {http.StatusUnauthorized, "Multi-factor authentication required"},
	StepUpRequired:        {http.StatusUnauthorized, "Recent authentication required"},
	JustificationRequired: {http.StatusUnprocessableEntity, "Justification required"},
	IdPUnavailable:        {http.StatusBadGateway, "Identity provider unavailable"},
	Conflict:              {http.StatusConflict, "Conflict with the current status"},
	QuoteExpired:          {http.StatusConflict, "Quote expired"},
	PartnerUnavailable:    {http.StatusBadGateway, "Partner unavailable"},
	InvalidSignature:      {http.StatusBadRequest, "Invalid signature"},
	Internal:              {http.StatusInternalServerError, "Internal error"},
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
