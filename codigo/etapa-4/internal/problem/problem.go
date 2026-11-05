// Package problem responde erros no formato Problem Details (RFC 9457). A
// causa interna vai para o log, nunca para o cliente.
package problem

import "github.com/gin-gonic/gin"

type Code string

const (
	FileTooLarge Code = "file_too_large"
	InvalidSTL   Code = "invalid_stl"
)

type Error struct{}

func New(code Code, cause error) *Error { return nil }

func (e *Error) Error() string { return "" }

func Abort(c *gin.Context, err error) {}
