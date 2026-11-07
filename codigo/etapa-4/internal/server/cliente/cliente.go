// Package cliente é o roteador da API pública. As rotas do painel não existem
// aqui.
package cliente

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/problem"
	"nuggetmachine/internal/server"
)

type Deps struct {
	Verifier *auth.Verifier
	Files    files.Repo
	Store    files.ObjectStore
	Queue    files.Enqueuer
	Audit    *audit.Logger
	Log      *slog.Logger // nil desliga o log de requisições
	// TrustedProxies são os IPs de quem pode informar a origem por
	// X-Forwarded-For. Vazio, a origem dos eventos de auditoria vem da conexão,
	// e não de um cabeçalho forjado. No compose, só o nginx (E2.3).
	TrustedProxies []string
}

func New(d Deps) (*gin.Engine, error) {
	r := gin.New()
	if err := r.SetTrustedProxies(d.TrustedProxies); err != nil {
		return nil, fmt.Errorf("proxies confiáveis: %w", err)
	}
	if d.Log != nil {
		r.Use(server.RequestLog(d.Log))
	}
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		problem.Abort(c, fmt.Errorf("panic: %v", recovered))
	}))

	h := &files.Handler{Repo: d.Files, Store: d.Store, Queue: d.Queue, Audit: d.Audit}
	api := r.Group("/api/v1", auth.RequireUser(d.Verifier))
	api.GET("/arquivos/:id/download", h.Download)
	api.POST("/arquivos", h.Upload)
	return r, nil
}
