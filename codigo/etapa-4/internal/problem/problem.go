// Package problem responde erros no formato Problem Details (RFC 9457). A
// causa interna vai para o log, nunca para o cliente.
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
	Unauthenticated:  {http.StatusUnauthorized, "Autenticação necessária"},
	Forbidden:        {http.StatusForbidden, "Acesso negado"},
	FileTooLarge:     {http.StatusRequestEntityTooLarge, "Arquivo grande demais"},
	InvalidSTL:       {http.StatusUnprocessableEntity, "STL inválido"},
	TooManyTriangles: {http.StatusUnprocessableEntity, "STL com triângulos demais"},
	Internal:         {http.StatusInternalServerError, "Erro interno"},
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

// Abort guarda o erro completo em c.Errors, para o log, e responde só o
// código. Um erro sem Code conhecido vira 500.
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
		Type:   "/erros/" + string(code),
		Title:  entry.title,
		Status: entry.status,
		Code:   code,
	})
}
