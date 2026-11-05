// Package cliente é o roteador da API pública. As rotas do painel não existem
// aqui.
package cliente

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"nuggetmachine/internal/audit"
	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/files"
	"nuggetmachine/internal/problem"
)

type Deps struct {
	Verifier *auth.Verifier
	Files    files.Repo
	Store    files.ObjectStore
	Queue    files.Enqueuer
	Audit    *audit.Logger
}

func New(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		problem.Abort(c, fmt.Errorf("panic: %v", recovered))
	}))
	// Nenhum proxy é confiável até o nginx (E2.3): a origem dos eventos de
	// auditoria vem da conexão, e não de um X-Forwarded-For forjado.
	_ = r.SetTrustedProxies(nil)

	h := &files.Handler{Repo: d.Files, Store: d.Store, Queue: d.Queue, Audit: d.Audit}
	api := r.Group("/api/v1", auth.RequireUser(d.Verifier))
	api.GET("/arquivos/:id/download", h.Download)
	api.POST("/arquivos", h.Upload)
	return r
}
