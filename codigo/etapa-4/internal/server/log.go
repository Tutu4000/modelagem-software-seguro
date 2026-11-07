// Package server tem os middlewares compartilhados pelos roteadores.
package server

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLog grava uma linha por requisição, com a causa interna dos erros que
// o problem.Abort escondeu do cliente. Nunca grava cabeçalhos, corpo nem query
// string, onde ficam tokens e assinaturas (C17, E7.3).
func RequestLog(l *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("route", c.FullPath()),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("latency", time.Since(start)),
			slog.String("origin", c.ClientIP()),
		}
		level := slog.LevelInfo
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.Any("errors", c.Errors.Errors()))
			if c.Writer.Status() >= 500 {
				level = slog.LevelError
			}
		}
		l.LogAttrs(c.Request.Context(), level, "request", attrs...)
	}
}
